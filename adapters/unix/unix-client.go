// Package unix provides a Unix Domain Socket client wrapper for Socket.IO Unix adapter.
// This package offers a unified interface for Unix Domain Socket operations with event handling
// support using connection-oriented stream sockets (SOCK_STREAM) for pub/sub communication.
//
// Messages are framed with a 4-byte big-endian length prefix to ensure reliable delivery
// over the byte-stream transport. Connections to peers are pooled and reused for performance.
package unix

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// maxMessageSize is the maximum allowed message size (10 MB).
// This prevents malicious or corrupted length headers from causing excessive memory allocation.
const maxMessageSize = 10 << 20

// peerConn wraps a persistent connection to a peer with its own mutex
// to ensure atomic framed writes when multiple goroutines send concurrently.
type peerConn struct {
	mu   sync.Mutex
	conn net.Conn
}

// receivedMessage holds a complete message received from a stream connection.
type receivedMessage struct {
	data []byte
	addr net.Addr
}

// UnixClient wraps Unix Domain Socket stream connections and provides context management
// and event emitting capabilities for the Socket.IO Unix adapter.
//
// The client uses connection-oriented Unix Domain Sockets (SOCK_STREAM) for pub/sub messaging.
// Each message is framed with a 4-byte big-endian length prefix for reliable delivery.
// Outgoing connections are pooled per peer for performance; incoming connections are
// accepted in background goroutines and delivered through an internal message queue.
//
// The client supports error event emission, which allows higher-level components
// to handle Unix socket-related errors gracefully.
type UnixClient struct {
	types.EventEmitter

	// SocketPath is the base path of the Unix Domain Socket used for communication.
	SocketPath string

	// Context controls the lifecycle of the client.
	// When canceled, all operations will be terminated.
	Context context.Context

	mu           sync.Mutex
	listener     net.Listener
	listenerPath string

	// Connection pool for outgoing connections to peers.
	peersMu sync.Mutex
	peers   map[string]*peerConn

	// Internal message channel for received messages.
	msgCh chan *receivedMessage

	// Tracking accepted connections for cleanup.
	activeConns   map[net.Conn]struct{}
	activeConnsMu sync.Mutex

	// Shutdown control.
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewUnixClient creates a new UnixClient with the given context and socket path.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of Unix socket operations.
//     When canceled, all operations will be terminated.
//   - socketPath: The path to the shared Unix Domain Socket used for broadcasting messages.
//
// Returns:
//   - A pointer to the initialized UnixClient instance.
//
// Example:
//
//	client := NewUnixClient(context.Background(), "/tmp/socket.io.sock")
func NewUnixClient(ctx context.Context, socketPath string) *UnixClient {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx, cancel := context.WithCancel(ctx)

	return &UnixClient{
		EventEmitter: types.NewEventEmitter(),
		SocketPath:   socketPath,
		Context:      ctx,
		cancel:       cancel,
		peers:        make(map[string]*peerConn),
		activeConns:  make(map[net.Conn]struct{}),
		msgCh:        make(chan *receivedMessage, 256),
	}
}

// Listen starts accepting stream connections on the given Unix socket path.
// Incoming connections are handled in background goroutines, with messages
// delivered through ReadMessage via an internal queue.
//
// Parameters:
//   - listenerPath: The unique path for this listener's Unix Domain Socket.
func (c *UnixClient) Listen(listenerPath string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.listener != nil {
		return nil
	}

	c.listenerPath = listenerPath

	listener, err := net.Listen("unix", listenerPath)
	if err != nil {
		return fmt.Errorf("failed to listen on Unix socket %q: %w", listenerPath, err)
	}

	c.listener = listener

	c.wg.Add(1)
	go c.acceptLoop()

	return nil
}

// acceptLoop continuously accepts new stream connections on the listener.
func (c *UnixClient) acceptLoop() {
	defer c.wg.Done()

	for {
		conn, err := c.listener.Accept()
		if err != nil {
			select {
			case <-c.Context.Done():
				return
			default:
				return // listener was closed
			}
		}

		c.trackConn(conn)
		c.wg.Add(1)
		go c.handleConn(conn)
	}
}

// trackConn adds a connection to the active set for cleanup on Close.
func (c *UnixClient) trackConn(conn net.Conn) {
	c.activeConnsMu.Lock()
	c.activeConns[conn] = struct{}{}
	c.activeConnsMu.Unlock()
}

// untrackConn removes a connection from the active set.
func (c *UnixClient) untrackConn(conn net.Conn) {
	c.activeConnsMu.Lock()
	delete(c.activeConns, conn)
	c.activeConnsMu.Unlock()
}

// handleConn reads length-prefixed messages from an accepted stream connection.
// Each message is framed as [4-byte big-endian length][payload].
func (c *UnixClient) handleConn(conn net.Conn) {
	defer c.wg.Done()
	defer func() { _ = conn.Close() }()
	defer c.untrackConn(conn)

	addr := conn.RemoteAddr()

	for {
		// Read 4-byte length header.
		var header [4]byte
		if _, err := io.ReadFull(conn, header[:]); err != nil {
			return // connection closed or broken
		}

		msgLen := binary.BigEndian.Uint32(header[:])
		if msgLen == 0 || msgLen > maxMessageSize {
			return // invalid or oversized message
		}

		data := make([]byte, msgLen)
		if _, err := io.ReadFull(conn, data); err != nil {
			return // incomplete read
		}

		select {
		case c.msgCh <- &receivedMessage{data: data, addr: addr}:
		case <-c.Context.Done():
			return
		}
	}
}

// ReadMessage reads the next message from the internal message queue.
// This method blocks until a message is received or the context is canceled.
//
// Returns the received message bytes and the sender address, or an error.
func (c *UnixClient) ReadMessage(buf []byte) (int, net.Addr, error) {
	c.mu.Lock()
	listening := c.listener != nil
	c.mu.Unlock()

	if !listening {
		return 0, nil, fmt.Errorf("listener not started")
	}

	select {
	case msg := <-c.msgCh:
		if msg == nil {
			return 0, nil, fmt.Errorf("message channel closed")
		}
		n := copy(buf, msg.data)
		return n, msg.addr, nil
	case <-c.Context.Done():
		return 0, nil, c.Context.Err()
	}
}

// Send sends a length-prefixed message to the specified Unix socket path.
// Connections are pooled and reused across calls. If a send fails, the stale
// connection is discarded and the send is retried once with a fresh connection.
//
// Parameters:
//   - targetPath: The path of the target Unix Domain Socket.
//   - payload: The message payload bytes.
func (c *UnixClient) Send(targetPath string, payload []byte) error {
	if c.Context.Err() != nil {
		return c.Context.Err()
	}

	pc := c.getOrCreatePeer(targetPath)
	pc.mu.Lock()
	defer pc.mu.Unlock()

	// Try once; on failure the connection is reset, so retry with a fresh connection.
	if err := c.writeFrame(pc, targetPath, payload); err != nil {
		if err2 := c.writeFrame(pc, targetPath, payload); err2 != nil {
			return err2
		}
	}

	return nil
}

// getOrCreatePeer returns the peerConn for the given path, creating one if needed.
func (c *UnixClient) getOrCreatePeer(targetPath string) *peerConn {
	c.peersMu.Lock()
	defer c.peersMu.Unlock()

	if pc, ok := c.peers[targetPath]; ok {
		return pc
	}

	pc := &peerConn{}
	c.peers[targetPath] = pc
	return pc
}

// writeFrame connects (if needed) and writes a length-prefixed message to the peer.
// The frame format is [4-byte big-endian length][payload]. Uses net.Buffers for
// efficient scatter-gather I/O (writev). On write failure, the connection is closed
// and set to nil so the next call retries with a fresh connection.
func (c *UnixClient) writeFrame(pc *peerConn, targetPath string, payload []byte) error {
	if pc.conn == nil {
		conn, err := net.Dial("unix", targetPath)
		if err != nil {
			return fmt.Errorf("failed to dial Unix socket %q: %w", targetPath, err)
		}
		pc.conn = conn
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))

	bufs := net.Buffers{header[:], payload}
	if _, err := bufs.WriteTo(pc.conn); err != nil {
		_ = pc.conn.Close()
		pc.conn = nil
		return fmt.Errorf("failed to send to Unix socket %q: %w", targetPath, err)
	}

	return nil
}

// ListenerPath returns the path of the listener socket.
func (c *UnixClient) ListenerPath() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.listenerPath
}

// Close releases all resources including the listener, accepted connections,
// pooled peer connections, and waits for background goroutines to exit.
func (c *UnixClient) Close() error {
	c.mu.Lock()

	var errs []error

	// Cancel context to signal all goroutines.
	if c.cancel != nil {
		c.cancel()
	}

	// Close listener to unblock Accept.
	if c.listener != nil {
		if err := c.listener.Close(); err != nil {
			errs = append(errs, err)
		}
		c.listener = nil
	}

	c.mu.Unlock()

	// Close all accepted connections to unblock ReadFull.
	c.activeConnsMu.Lock()
	for conn := range c.activeConns {
		_ = conn.Close()
		delete(c.activeConns, conn)
	}
	c.activeConnsMu.Unlock()

	// Close all pooled peer connections.
	c.peersMu.Lock()
	for path, pc := range c.peers {
		pc.mu.Lock()
		if pc.conn != nil {
			_ = pc.conn.Close()
			pc.conn = nil
		}
		pc.mu.Unlock()
		delete(c.peers, path)
	}
	c.peersMu.Unlock()

	// Wait for all background goroutines to exit.
	c.wg.Wait()

	if len(errs) > 0 {
		return fmt.Errorf("errors closing Unix client: %v", errs)
	}
	return nil
}
