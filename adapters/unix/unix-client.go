// Package unix provides a Unix Domain Socket transport for Socket.IO adapters.
// Messages use a 4-byte big-endian length prefix over SOCK_STREAM connections.
package unix

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	// maxMessageSize is the maximum allowed message size (10 MiB).
	maxMessageSize = 10 << 20

	defaultUnixDialTimeout  = 5 * time.Second
	defaultUnixWriteTimeout = 5 * time.Second
)

var (
	unixClientLog = log.NewLog("socket.io-unix")

	// ErrUnixSocketPathRequired is returned when no usable Unix socket base path is provided.
	ErrUnixSocketPathRequired = errors.New("unix: socket path is required")
	// ErrUnixClientClosed is returned when an operation starts after client shutdown.
	ErrUnixClientClosed = errors.New("unix: client is closed")
	// ErrUnixListenerNotStarted is returned when ReadMessage is called before Listen.
	ErrUnixListenerNotStarted = errors.New("unix: listener not started")
	// ErrUnixClientAlreadyListening is returned when Listen is called with another path.
	ErrUnixClientAlreadyListening = errors.New("unix: client is already listening")

	errUnixPeerRemoved = errors.New("unix: peer was removed")
)

// peerConn serializes framed writes while allowing Close to interrupt a blocked
// write by closing the connection independently.
type peerConn struct {
	sendMu sync.Mutex
	connMu sync.Mutex
	conn   net.Conn
}

func (p *peerConn) connection() net.Conn {
	p.connMu.Lock()
	defer p.connMu.Unlock()
	return p.conn
}

func (p *peerConn) setConn(conn net.Conn) {
	p.connMu.Lock()
	p.conn = conn
	p.connMu.Unlock()
}

func (p *peerConn) close() {
	p.connMu.Lock()
	conn := p.conn
	p.conn = nil
	p.connMu.Unlock()
	if conn != nil {
		_ = conn.Close()
	}
}

// UnixClientOptions configures bounded peer I/O. Non-positive values use the
// defaults returned by DefaultUnixClientOptions.
type UnixClientOptions struct {
	// DialTimeout bounds each peer connection attempt.
	DialTimeout time.Duration
	// WriteTimeout bounds each complete framed write.
	WriteTimeout time.Duration
}

// DefaultUnixClientOptions returns the default peer I/O timeouts.
func DefaultUnixClientOptions() *UnixClientOptions {
	return &UnixClientOptions{
		DialTimeout:  defaultUnixDialTimeout,
		WriteTimeout: defaultUnixWriteTimeout,
	}
}

// UnixClient owns a listener and pooled outgoing Unix stream connections.
// The zero value is not usable; create clients with NewUnixClient.
type UnixClient struct {
	types.EventEmitter

	socketPath   string
	ctx          context.Context
	cancel       context.CancelFunc
	dialTimeout  time.Duration
	writeTimeout time.Duration

	mu           sync.Mutex
	listener     net.Listener
	listenerPath string
	peers        map[string]*peerConn
	activeConns  map[net.Conn]struct{}
	closeOnce    sync.Once
	closeErr     error
	wg           sync.WaitGroup

	msgCh chan []byte
}

func (c *UnixClient) onError(...any) {
	if c.ListenerCount("error") == 1 {
		unixClientLog.Warning("missing 'error' handler on this Unix client")
	}
}

// NewUnixClient creates a Unix transport whose lifecycle follows ctx. The
// option values are copied during construction; nil uses the bounded defaults.
func NewUnixClient(ctx context.Context, socketPath string, opts *UnixClientOptions) (*UnixClient, error) {
	if socketPath == "" {
		return nil, ErrUnixSocketPathRequired
	}
	if os.IsPathSeparator(socketPath[len(socketPath)-1]) {
		return nil, fmt.Errorf("%w: must not end with a path separator", ErrUnixSocketPathRequired)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	resolved := DefaultUnixClientOptions()
	if opts != nil {
		if opts.DialTimeout > 0 {
			resolved.DialTimeout = opts.DialTimeout
		}
		if opts.WriteTimeout > 0 {
			resolved.WriteTimeout = opts.WriteTimeout
		}
	}

	ctx, cancel := context.WithCancel(ctx)
	client := &UnixClient{
		EventEmitter: types.NewEventEmitter(),
		socketPath:   socketPath,
		ctx:          ctx,
		cancel:       cancel,
		dialTimeout:  resolved.DialTimeout,
		writeTimeout: resolved.WriteTimeout,
		peers:        make(map[string]*peerConn),
		activeConns:  make(map[net.Conn]struct{}),
		msgCh:        make(chan []byte, 256),
	}
	_ = client.On("error", client.onError)

	context.AfterFunc(ctx, func() {
		_ = client.Close()
	})

	return client, nil
}

// SocketPath returns the base path used for peer discovery.
func (c *UnixClient) SocketPath() string { return c.socketPath }

// Context returns the context controlling Unix socket operations.
func (c *UnixClient) Context() context.Context { return c.ctx }

// Listen starts accepting framed messages on listenerPath.
func (c *UnixClient) Listen(listenerPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.ctx.Err() != nil {
		return ErrUnixClientClosed
	}
	if c.listener != nil {
		if c.listenerPath == listenerPath {
			return nil
		}
		return fmt.Errorf("%w on %q", ErrUnixClientAlreadyListening, c.listenerPath)
	}

	listener, err := listenUnix(listenerPath)
	if err != nil {
		return fmt.Errorf("failed to listen on Unix socket %q: %w", listenerPath, err)
	}

	// Publish listener state only after listenUnix, including chmod, succeeds.
	c.listener = listener
	c.listenerPath = listenerPath
	c.wg.Add(1)
	go c.acceptLoop(listener)
	return nil
}

func (c *UnixClient) acceptLoop(listener net.Listener) {
	defer c.wg.Done()
	var retryDelay time.Duration

	for {
		conn, err := listener.Accept()
		if err != nil {
			c.mu.Lock()
			stopped := c.ctx.Err() != nil || c.listener != listener
			c.mu.Unlock()
			if stopped {
				return
			}

			go c.Emit("error", fmt.Errorf("failed to accept Unix socket connection: %w", err))
			if retryDelay == 0 {
				retryDelay = 5 * time.Millisecond
			} else {
				retryDelay = min(2*retryDelay, time.Second)
			}
			timer := time.NewTimer(retryDelay)
			select {
			case <-timer.C:
			case <-c.ctx.Done():
				timer.Stop()
				return
			}
			continue
		}
		retryDelay = 0

		c.mu.Lock()
		if c.listener != listener {
			c.mu.Unlock()
			_ = conn.Close()
			return
		}
		c.activeConns[conn] = struct{}{}
		c.wg.Add(1)
		c.mu.Unlock()

		go c.handleConn(conn)
	}
}

func (c *UnixClient) handleConn(conn net.Conn) {
	var frameErr error
	defer func() {
		_ = conn.Close()
		c.mu.Lock()
		delete(c.activeConns, conn)
		c.mu.Unlock()
		c.wg.Done()
		if frameErr != nil {
			c.Emit("error", frameErr)
		}
	}()

	for {
		data, err := readUnixMessage(conn)
		if err != nil {
			if errors.Is(err, ErrUnixMessageSize) {
				frameErr = fmt.Errorf("failed to read Unix socket frame: %w", err)
			}
			return
		}

		select {
		case c.msgCh <- data:
		case <-c.ctx.Done():
			return
		}
	}
}

// ReadMessage returns the next complete frame without copying it through a
// caller-sized buffer.
func (c *UnixClient) ReadMessage() ([]byte, error) {
	c.mu.Lock()
	closed := c.ctx.Err() != nil
	listening := c.listener != nil
	c.mu.Unlock()

	if closed {
		return nil, ErrUnixClientClosed
	}
	if !listening {
		return nil, ErrUnixListenerNotStarted
	}

	select {
	case data := <-c.msgCh:
		return data, nil
	case <-c.ctx.Done():
		return nil, ErrUnixClientClosed
	}
}

// Send writes one framed message to targetPath. A non-timeout write failure on
// an existing pooled connection is retried once with a new connection. Dial,
// first-use write, and write-timeout failures are returned directly.
func (c *UnixClient) Send(targetPath string, payload []byte) error {
	if err := validateUnixMessage(payload); err != nil {
		return err
	}

	for {
		pc, err := c.lockPeer(targetPath)
		if err != nil {
			return err
		}
		err = c.sendLocked(targetPath, payload, pc)
		pc.sendMu.Unlock()
		if !errors.Is(err, errUnixPeerRemoved) {
			return err
		}
	}
}

func (c *UnixClient) lockPeer(targetPath string) (*peerConn, error) {
	for {
		c.mu.Lock()
		if c.ctx.Err() != nil {
			c.mu.Unlock()
			return nil, ErrUnixClientClosed
		}
		pc := c.peers[targetPath]
		if pc == nil {
			pc = &peerConn{}
			c.peers[targetPath] = pc
		}
		c.mu.Unlock()

		pc.sendMu.Lock()
		c.mu.Lock()
		closed := c.ctx.Err() != nil
		current := c.peers[targetPath] == pc
		c.mu.Unlock()
		if closed {
			pc.sendMu.Unlock()
			return nil, ErrUnixClientClosed
		}
		if current {
			return pc, nil
		}
		pc.sendMu.Unlock()
	}
}

func (c *UnixClient) sendLocked(targetPath string, payload []byte, pc *peerConn) error {
	conn := pc.connection()
	pooled := conn != nil
	if !pooled {
		var err error
		conn, err = c.dialPeer(targetPath)
		if err != nil {
			c.removePeer(targetPath, pc)
			return err
		}
		pc.setConn(conn)
		if err := c.checkPeer(targetPath, pc); err != nil {
			pc.close()
			return err
		}
	}

	err := c.writeMessage(conn, payload)
	if err == nil {
		return nil
	}
	if !pooled || errors.Is(err, os.ErrDeadlineExceeded) {
		pc.close()
		c.removePeer(targetPath, pc)
		return fmt.Errorf("failed to send to Unix socket %q: %w", targetPath, err)
	}

	// Only a stale pooled connection gets one reconnect attempt.
	pc.close()
	if peerErr := c.checkPeer(targetPath, pc); peerErr != nil {
		c.removePeer(targetPath, pc)
		return peerErr
	}

	conn, err = c.dialPeer(targetPath)
	if err != nil {
		c.removePeer(targetPath, pc)
		return err
	}
	pc.setConn(conn)
	if peerErr := c.checkPeer(targetPath, pc); peerErr != nil {
		pc.close()
		return peerErr
	}
	err = c.writeMessage(conn, payload)
	if err != nil {
		pc.close()
		c.removePeer(targetPath, pc)
		return fmt.Errorf("failed to send to Unix socket %q: %w", targetPath, err)
	}
	return nil
}

// writeMessage bounds each complete frame write with a fresh deadline.
func (c *UnixClient) writeMessage(conn net.Conn, payload []byte) error {
	if err := conn.SetWriteDeadline(time.Now().Add(c.writeTimeout)); err != nil {
		return err
	}
	return writeUnixMessage(conn, payload)
}

func (c *UnixClient) dialPeer(targetPath string) (net.Conn, error) {
	conn, err := dialUnix(c.ctx, targetPath, c.dialTimeout)
	if err == nil {
		return conn, nil
	}
	if c.ctx.Err() != nil {
		return nil, ErrUnixClientClosed
	}
	return nil, fmt.Errorf("failed to dial Unix socket %q: %w", targetPath, err)
}

func (c *UnixClient) checkPeer(targetPath string, pc *peerConn) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ctx.Err() != nil {
		return ErrUnixClientClosed
	}
	if c.peers[targetPath] != pc {
		return errUnixPeerRemoved
	}
	return nil
}

func (c *UnixClient) removePeer(targetPath string, pc *peerConn) {
	c.mu.Lock()
	if c.peers[targetPath] == pc {
		delete(c.peers, targetPath)
	}
	c.mu.Unlock()
}

// Broadcast discovers listener sockets derived from SocketPath and sends the
// payload to each peer. Per-peer failures are emitted as "error" events; only
// discovery and input errors are returned.
func (c *UnixClient) Broadcast(payload []byte) error {
	if err := validateUnixMessage(payload); err != nil {
		return err
	}

	if c.ctx.Err() != nil {
		return ErrUnixClientClosed
	}

	directory := filepath.Dir(c.socketPath)
	prefix := filepath.Base(c.socketPath) + "."
	entries, err := os.ReadDir(directory)
	if err != nil {
		return fmt.Errorf("failed to scan Unix socket directory %q: %w", directory, err)
	}

	targets := make([]string, 0, len(entries))
	discovered := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if !matchesUnixPeerName(entry.Name(), prefix) {
			continue
		}
		isSocket, err := isUnixSocketEntry(entry)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				go c.Emit("error", fmt.Errorf("failed to inspect Unix socket %q: %w", entry.Name(), err))
			}
			continue
		}
		if !isSocket {
			continue
		}

		targetPath := filepath.Join(directory, entry.Name())
		discovered[targetPath] = struct{}{}
		targets = append(targets, targetPath)
	}

	c.prunePeers(directory, prefix, discovered)
	for _, targetPath := range targets {
		if err := c.Send(targetPath, payload); err != nil {
			go c.Emit("error", err)
		}
	}
	return nil
}

func matchesUnixPeerName(name, prefix string) bool {
	suffix, matchesPrefix := strings.CutPrefix(name, prefix)
	return matchesPrefix && suffix != "" && !strings.ContainsRune(suffix, '.')
}

func isUnixSocketEntry(entry os.DirEntry) (bool, error) {
	mode := entry.Type()
	if mode&os.ModeSymlink != 0 || mode.IsDir() {
		return false, nil
	}
	if mode&os.ModeSocket != 0 {
		return true, nil
	}
	info, err := entry.Info()
	if err != nil {
		return false, err
	}
	return info.Mode()&os.ModeSocket != 0, nil
}

func (c *UnixClient) prunePeers(directory, prefix string, discovered map[string]struct{}) {
	c.mu.Lock()
	for targetPath, pc := range c.peers {
		if filepath.Dir(targetPath) != directory || !matchesUnixPeerName(filepath.Base(targetPath), prefix) {
			continue
		}
		if _, ok := discovered[targetPath]; ok {
			continue
		}
		pc.close()
		delete(c.peers, targetPath)
	}
	c.mu.Unlock()
}

// Close releases the listener and all accepted and pooled connections. It is
// safe to call concurrently and waits for an in-progress Close to finish.
func (c *UnixClient) Close() error {
	c.closeOnce.Do(func() {
		c.closeErr = c.close()
	})
	return c.closeErr
}

func (c *UnixClient) close() error {
	c.mu.Lock()
	c.cancel()
	listener := c.listener
	c.listener = nil
	c.listenerPath = ""
	activeConns := make([]net.Conn, 0, len(c.activeConns))
	for conn := range c.activeConns {
		activeConns = append(activeConns, conn)
	}
	clear(c.activeConns)
	peers := make([]*peerConn, 0, len(c.peers))
	for _, pc := range c.peers {
		peers = append(peers, pc)
	}
	clear(c.peers)
	c.mu.Unlock()

	var closeErr error
	if listener != nil {
		if closeErr = listener.Close(); errors.Is(closeErr, net.ErrClosed) {
			closeErr = nil
		}
	}
	for _, conn := range activeConns {
		_ = conn.Close()
	}
	for _, pc := range peers {
		pc.close()
	}
	c.wg.Wait()

drainMessages:
	for {
		select {
		case <-c.msgCh:
		default:
			break drainMessages
		}
	}

	return closeErr
}
