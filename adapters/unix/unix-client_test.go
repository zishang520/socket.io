package unix

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewUnixClient(t *testing.T) {
	t.Run("with valid context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		client, err := NewUnixClient(ctx, "/tmp/test.sock", nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })

		if client.SocketPath() != "/tmp/test.sock" {
			t.Fatal("SocketPath mismatch")
		}
		cancel()
		<-client.Context().Done()
	})

	t.Run("with nil context defaults to background", func(t *testing.T) {
		client, err := NewUnixClient(nil, "/tmp/test.sock", nil) //nolint:staticcheck // Verify the nil-context fallback.
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })

		if client.Context() == nil {
			t.Fatal("expected non-nil context")
		}
	})

	t.Run("requires socket path", func(t *testing.T) {
		client, err := NewUnixClient(context.Background(), "", nil)
		if client != nil || !errors.Is(err, ErrUnixSocketPathRequired) {
			t.Fatalf("NewUnixClient() = (%v, %v), want (nil, ErrUnixSocketPathRequired)", client, err)
		}
	})

	t.Run("rejects a directory-like socket path", func(t *testing.T) {
		path := "socket.io" + string(os.PathSeparator)
		client, err := NewUnixClient(context.Background(), path, nil)
		if client != nil || !errors.Is(err, ErrUnixSocketPathRequired) {
			t.Fatalf("NewUnixClient(%q) = (%v, %v), want (nil, ErrUnixSocketPathRequired)", path, client, err)
		}
	})

	t.Run("uses copied bounded options", func(t *testing.T) {
		opts := &UnixClientOptions{
			DialTimeout:  150 * time.Millisecond,
			WriteTimeout: 250 * time.Millisecond,
		}
		client, err := NewUnixClient(context.Background(), "/tmp/test.sock", opts)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })

		opts.DialTimeout = time.Second
		opts.WriteTimeout = time.Second
		if client.dialTimeout != 150*time.Millisecond || client.writeTimeout != 250*time.Millisecond {
			t.Fatalf("client timeouts = (%s, %s), want (150ms, 250ms)", client.dialTimeout, client.writeTimeout)
		}
	})

	t.Run("non-positive options use bounded defaults", func(t *testing.T) {
		client, err := NewUnixClient(context.Background(), "/tmp/test.sock", &UnixClientOptions{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })
		if client.dialTimeout != defaultUnixDialTimeout || client.writeTimeout != defaultUnixWriteTimeout {
			t.Fatalf("client timeouts = (%s, %s), want defaults", client.dialTimeout, client.writeTimeout)
		}
	})
}

func TestUnixClientDefaultErrorHandler(t *testing.T) {
	client, err := NewUnixClient(context.Background(), "/tmp/test.sock", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	if got := client.ListenerCount("error"); got != 1 {
		t.Fatalf("default error listeners = %d, want 1", got)
	}

	previousOutput := unixClientLog.Writer()
	var output bytes.Buffer
	unixClientLog.SetOutput(&output)
	t.Cleanup(func() { unixClientLog.SetOutput(previousOutput) })

	client.Emit("error", errors.New("test error"))
	if !strings.Contains(output.String(), "missing 'error' handler on this Unix client") {
		t.Fatalf("default error output = %q", output.String())
	}

	var handled atomic.Int64
	if err := client.On("error", func(...any) { handled.Add(1) }); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	client.Emit("error", errors.New("handled error"))
	if handled.Load() != 1 {
		t.Fatalf("user error handler calls = %d, want 1", handled.Load())
	}
	if output.Len() != 0 {
		t.Fatalf("default error handler logged with a user listener: %q", output.String())
	}
}

func TestUnixMessageSizeBoundaries(t *testing.T) {
	directory := newTestUnixDirectory(t)
	basePath := filepath.Join(directory, "socket.io")
	listenerPath := basePath + ".receiver"
	receiver := newTestUnixClient(t, context.Background(), basePath)
	if err := receiver.Listen(listenerPath); err != nil {
		t.Fatal(err)
	}

	sender := newTestUnixClient(t, context.Background(), basePath)
	payload := bytes.Repeat([]byte("large-frame"), 8<<10)
	if len(payload) <= 64<<10 {
		t.Fatal("test payload must exceed the former fixed read buffer")
	}
	if err := sender.Send(listenerPath, payload); err != nil {
		t.Fatal(err)
	}

	got := readTestMessage(t, receiver)
	if !bytes.Equal(got, payload) {
		t.Fatalf("ReadMessage() returned %d bytes, want %d", len(got), len(payload))
	}

	for name, payload := range map[string][]byte{
		"empty":     nil,
		"oversized": make([]byte, maxMessageSize+1),
	} {
		t.Run("send "+name, func(t *testing.T) {
			if err := sender.Send("missing", payload); !errors.Is(err, ErrUnixMessageSize) {
				t.Fatalf("Send() error = %v, want ErrUnixMessageSize", err)
			}
		})
	}
	if err := validateUnixMessage(make([]byte, maxMessageSize)); err != nil {
		t.Fatalf("maximum-size message rejected: %v", err)
	}

	for name, size := range map[string]uint32{
		"empty frame":     0,
		"oversized frame": maxMessageSize + 1,
	} {
		t.Run("receive "+name, func(t *testing.T) {
			reader, writer := net.Pipe()
			defer func() { _ = reader.Close() }()
			defer func() { _ = writer.Close() }()

			var header [4]byte
			binary.BigEndian.PutUint32(header[:], size)
			go func() { _, _ = writer.Write(header[:]) }()
			if _, err := readUnixMessage(reader); !errors.Is(err, ErrUnixMessageSize) {
				t.Fatalf("readUnixMessage() error = %v, want ErrUnixMessageSize", err)
			}
		})
	}
}

func TestUnixClientListenState(t *testing.T) {
	directory := newTestUnixDirectory(t)
	client := newTestUnixClient(t, context.Background(), filepath.Join(directory, "socket.io"))

	badPath := filepath.Join(directory, "missing", "listener")
	if err := client.Listen(badPath); err == nil {
		t.Fatal("Listen() succeeded with a missing parent directory")
	}
	if _, err := client.ReadMessage(); !errors.Is(err, ErrUnixListenerNotStarted) {
		t.Fatalf("ReadMessage() error = %v, want ErrUnixListenerNotStarted", err)
	}

	listenerPath := filepath.Join(directory, "listener")
	if err := client.Listen(listenerPath); err != nil {
		t.Fatal(err)
	}
	if err := client.Listen(listenerPath); err != nil {
		t.Fatalf("Listen() should be idempotent for the same path: %v", err)
	}
	if err := client.Listen(filepath.Join(directory, "other")); !errors.Is(err, ErrUnixClientAlreadyListening) {
		t.Fatalf("Listen() error = %v, want ErrUnixClientAlreadyListening", err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(listenerPath)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o600 {
			t.Fatalf("listener permissions = %o, want 600", got)
		}
	}
}

func TestUnixClientReportsMalformedFrame(t *testing.T) {
	directory := newTestUnixDirectory(t)
	client := newTestUnixClient(t, context.Background(), filepath.Join(directory, "socket.io"))
	listenerPath := filepath.Join(directory, "listener")
	if err := client.Listen(listenerPath); err != nil {
		t.Fatal(err)
	}

	errCh := make(chan error, 1)
	if err := client.On("error", func(args ...any) {
		if err, ok := args[0].(error); ok {
			errCh <- err
		}
	}); err != nil {
		t.Fatal(err)
	}

	conn, err := dialUnix(context.Background(), listenerPath, defaultUnixDialTimeout)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.Write(make([]byte, 4)); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, ErrUnixMessageSize) {
			t.Fatalf("error event = %v, want ErrUnixMessageSize", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("malformed frame did not emit an error")
	}
}

func TestUnixClientRetriesAcceptErrors(t *testing.T) {
	client := newTestUnixClient(t, context.Background(), filepath.Join(newTestUnixDirectory(t), "socket.io"))
	server, peer := net.Pipe()
	defer func() { _ = peer.Close() }()
	listener := newRetryListener(server)

	errCh := make(chan error, 1)
	if err := client.On("error", func(args ...any) {
		if err, ok := args[0].(error); ok {
			errCh <- err
		}
	}); err != nil {
		t.Fatal(err)
	}
	client.mu.Lock()
	client.listener = listener
	client.listenerPath = "retry-listener"
	client.wg.Add(1)
	client.mu.Unlock()
	go client.acceptLoop(listener)

	select {
	case <-errCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Accept error was not emitted")
	}
	if err := writeUnixMessage(peer, []byte("after-retry")); err != nil {
		t.Fatal(err)
	}
	if got := readTestMessage(t, client); string(got) != "after-retry" {
		t.Fatalf("message after Accept retry = %q", got)
	}
}

func TestUnixClientFollowsContextAndClosesIdempotently(t *testing.T) {
	directory := newTestUnixDirectory(t)
	ctx, cancel := context.WithCancel(context.Background())
	client := newTestUnixClient(t, ctx, filepath.Join(directory, "socket.io"))
	listenerPath := filepath.Join(directory, "listener")
	if err := client.Listen(listenerPath); err != nil {
		t.Fatal(err)
	}

	cancel()
	const callers = 4
	errs := make(chan error, callers)
	for range callers {
		go func() { errs <- client.Close() }()
	}
	for range callers {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("Close() error = %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent Close() did not return")
		}
	}

	if err := client.Listen(listenerPath); !errors.Is(err, ErrUnixClientClosed) {
		t.Fatalf("Listen() error = %v, want ErrUnixClientClosed", err)
	}
	if err := client.Send(listenerPath, []byte("message")); !errors.Is(err, ErrUnixClientClosed) {
		t.Fatalf("Send() error = %v, want ErrUnixClientClosed", err)
	}
	if _, err := client.ReadMessage(); !errors.Is(err, ErrUnixClientClosed) {
		t.Fatalf("ReadMessage() error = %v, want ErrUnixClientClosed", err)
	}
}

func TestUnixClientCloseDrainsQueuedMessages(t *testing.T) {
	client := newTestUnixClient(t, context.Background(), filepath.Join(newTestUnixDirectory(t), "socket.io"))
	client.msgCh <- []byte("first")
	client.msgCh <- []byte("second")

	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if queued := len(client.msgCh); queued != 0 {
		t.Fatalf("queued messages after Close = %d, want 0", queued)
	}
}

func TestUnixClientCloseInterruptsWrite(t *testing.T) {
	client := newTestUnixClient(t, context.Background(), filepath.Join(newTestUnixDirectory(t), "socket.io"))
	local, remote := net.Pipe()
	defer func() { _ = remote.Close() }()

	started := make(chan struct{})
	conn := &writeSignalConn{Conn: local, started: started}
	peer := &peerConn{conn: conn}
	const targetPath = "blocked-peer"
	client.mu.Lock()
	client.peers[targetPath] = peer
	client.mu.Unlock()

	sendDone := make(chan error, 1)
	go func() { sendDone <- client.Send(targetPath, []byte("blocked")) }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("Send() did not start writing")
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- client.Close() }()
	select {
	case err := <-closeDone:
		if err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not interrupt the blocked write")
	}
	select {
	case err := <-sendDone:
		if !errors.Is(err, ErrUnixClientClosed) {
			t.Fatalf("Send() error = %v, want ErrUnixClientClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send() remained blocked after Close")
	}
}

func TestUnixClientBroadcast(t *testing.T) {
	directory := newTestUnixDirectory(t)
	basePath := filepath.Join(directory, "socket.io")
	senderPath := basePath + ".sender"
	receiverPath := basePath + ".receiver"

	sender := newTestUnixClient(t, context.Background(), basePath)
	if err := sender.Listen(senderPath); err != nil {
		t.Fatal(err)
	}
	receiver := newTestUnixClient(t, context.Background(), basePath)
	if err := receiver.Listen(receiverPath); err != nil {
		t.Fatal(err)
	}

	regularPath := basePath + ".regular"
	if err := os.WriteFile(regularPath, []byte("not a socket"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(basePath+".directory", 0o700); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(receiverPath, basePath+".symlink")

	payload := []byte("broadcast")
	if err := sender.Broadcast(payload); err != nil {
		t.Fatal(err)
	}
	if got := readTestMessage(t, receiver); !bytes.Equal(got, payload) {
		t.Fatalf("broadcast payload = %q, want %q", got, payload)
	}
	if got := readTestMessage(t, sender); !bytes.Equal(got, payload) {
		t.Fatalf("self broadcast payload = %q, want %q", got, payload)
	}

	sender.mu.Lock()
	_, receiverPooled := sender.peers[receiverPath]
	_, selfPooled := sender.peers[senderPath]
	_, regularPooled := sender.peers[regularPath]
	sender.mu.Unlock()
	if !receiverPooled || !selfPooled || regularPooled {
		t.Fatalf("unexpected peer pool state: receiver=%v self=%v regular=%v", receiverPooled, selfPooled, regularPooled)
	}

	if err := receiver.Close(); err != nil {
		t.Fatal(err)
	}
	if err := sender.Broadcast([]byte("after-close")); err != nil {
		t.Fatal(err)
	}
	sender.mu.Lock()
	_, receiverPooled = sender.peers[receiverPath]
	sender.mu.Unlock()
	if receiverPooled {
		t.Fatal("Broadcast() retained a disappeared peer")
	}
}

func TestUnixClientBroadcastDoesNotMatchLongerBasePrefix(t *testing.T) {
	directory := newTestUnixDirectory(t)
	shortBase := filepath.Join(directory, "socket")
	longBase := filepath.Join(directory, "socket.io")
	shortPath := shortBase + ".receiver"
	longPath := longBase + ".receiver"

	shortReceiver := newTestUnixClient(t, context.Background(), shortBase)
	if err := shortReceiver.Listen(shortPath); err != nil {
		t.Fatal(err)
	}
	longReceiver := newTestUnixClient(t, context.Background(), longBase)
	if err := longReceiver.Listen(longPath); err != nil {
		t.Fatal(err)
	}
	sender := newTestUnixClient(t, context.Background(), shortBase)

	payload := []byte("short-base-only")
	if err := sender.Broadcast(payload); err != nil {
		t.Fatal(err)
	}
	if got := readTestMessage(t, shortReceiver); !bytes.Equal(got, payload) {
		t.Fatalf("short-base payload = %q, want %q", got, payload)
	}

	sender.mu.Lock()
	_, shortPooled := sender.peers[shortPath]
	_, longPooled := sender.peers[longPath]
	sender.mu.Unlock()
	if !shortPooled || longPooled {
		t.Fatalf("unexpected peer pool state: short=%v long=%v", shortPooled, longPooled)
	}

	directPayload := []byte("direct-to-long-base")
	if err := sender.Send(longPath, directPayload); err != nil {
		t.Fatal(err)
	}
	if got := readTestMessage(t, longReceiver); !bytes.Equal(got, directPayload) {
		t.Fatalf("direct long-base payload = %q, want %q", got, directPayload)
	}
	sender.mu.Lock()
	longPeer := sender.peers[longPath]
	sender.mu.Unlock()
	if longPeer == nil {
		t.Fatal("direct long-base peer was not retained")
	}

	secondPayload := []byte("short-base-again")
	if err := sender.Broadcast(secondPayload); err != nil {
		t.Fatal(err)
	}
	if got := readTestMessage(t, shortReceiver); !bytes.Equal(got, secondPayload) {
		t.Fatalf("second short-base payload = %q, want %q", got, secondPayload)
	}
	sender.mu.Lock()
	currentLongPeer := sender.peers[longPath]
	sender.mu.Unlock()
	if currentLongPeer != longPeer {
		t.Fatal("short-base Broadcast pruned the explicit long-base peer")
	}
}

func TestUnixClientBroadcastContinuesAfterWriteTimeout(t *testing.T) {
	directory := newTestUnixDirectory(t)
	basePath := filepath.Join(directory, "socket.io")
	slowPath := basePath + ".a-slow"
	healthyPath := basePath + ".z-healthy"

	slowListener, err := listenUnix(slowPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = slowListener.Close() })

	healthy := newTestUnixClient(t, context.Background(), basePath)
	if listenErr := healthy.Listen(healthyPath); listenErr != nil {
		t.Fatal(listenErr)
	}

	const writeTimeout = 100 * time.Millisecond
	sender, err := NewUnixClient(context.Background(), basePath, &UnixClientOptions{
		WriteTimeout: writeTimeout,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sender.Close() })

	local, remote := net.Pipe()
	t.Cleanup(func() { _ = remote.Close() })
	sender.mu.Lock()
	sender.peers[slowPath] = &peerConn{conn: local}
	sender.mu.Unlock()

	errCh := make(chan error, 1)
	if err := sender.On("error", func(args ...any) {
		if len(args) > 0 {
			if err, ok := args[0].(error); ok {
				errCh <- err
			}
		}
	}); err != nil {
		t.Fatal(err)
	}

	payload := []byte("after-slow-peer")
	started := time.Now()
	if err := sender.Broadcast(payload); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("Broadcast() took %s after a %s write timeout", elapsed, writeTimeout)
	}
	if got := readTestMessage(t, healthy); !bytes.Equal(got, payload) {
		t.Fatalf("healthy peer payload = %q, want %q", got, payload)
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, os.ErrDeadlineExceeded) {
			t.Fatalf("error event = %v, want write timeout", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for the slow-peer error")
	}

	sender.mu.Lock()
	_, retained := sender.peers[slowPath]
	sender.mu.Unlock()
	if retained {
		t.Fatal("Broadcast() retained the timed-out peer")
	}
}

func TestUnixClientBroadcastReturnsDiscoveryError(t *testing.T) {
	basePath := filepath.Join(newTestUnixDirectory(t), "missing", "socket.io")
	client := newTestUnixClient(t, context.Background(), basePath)
	if err := client.Broadcast([]byte("message")); err == nil {
		t.Fatal("Broadcast() succeeded with a missing discovery directory")
	}
}

func TestUnixSocketPlatformOps(t *testing.T) {
	listenerPath := filepath.Join(newTestUnixDirectory(t), "socket.io.sock")
	listener, err := listenUnix(listenerPath)
	if err != nil {
		t.Fatalf("listenUnix failed: %v", err)
	}
	defer func() { _ = listener.Close() }()

	accepted := make(chan []byte, 1)
	acceptErr := make(chan error, 1)
	go func() {
		acceptedConn, acceptConnErr := listener.Accept()
		if acceptConnErr != nil {
			acceptErr <- acceptConnErr
			return
		}
		defer func() { _ = acceptedConn.Close() }()
		data, readErr := readUnixMessage(acceptedConn)
		if readErr != nil {
			acceptErr <- readErr
			return
		}
		accepted <- data
	}()

	conn, err := dialUnix(context.Background(), listenerPath, defaultUnixDialTimeout)
	if err != nil {
		t.Fatalf("dialUnix failed: %v", err)
	}
	defer func() { _ = conn.Close() }()
	payload := []byte("platform-frame")
	if err := writeUnixMessage(conn, payload); err != nil {
		t.Fatalf("writeUnixMessage failed: %v", err)
	}

	select {
	case err := <-acceptErr:
		t.Fatalf("accept/read failed: %v", err)
	case got := <-accepted:
		if !bytes.Equal(got, payload) {
			t.Fatalf("payload = %q, want %q", got, payload)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for platform frame")
	}
}

func newTestUnixClient(t *testing.T, ctx context.Context, socketPath string) *UnixClient {
	t.Helper()
	client, err := NewUnixClient(ctx, socketPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func newTestUnixDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("", "sio-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return directory
}

func readTestMessage(t *testing.T, client *UnixClient) []byte {
	t.Helper()
	type result struct {
		data []byte
		err  error
	}
	resultCh := make(chan result, 1)
	go func() {
		data, err := client.ReadMessage()
		resultCh <- result{data: data, err: err}
	}()
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatal(result.err)
		}
		return result.data
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Unix message")
		return nil
	}
}

type writeSignalConn struct {
	net.Conn
	once    sync.Once
	started chan struct{}
}

func (c *writeSignalConn) Write(payload []byte) (int, error) {
	c.once.Do(func() { close(c.started) })
	return c.Conn.Write(payload)
}

type retryListener struct {
	conn      net.Conn
	closed    chan struct{}
	closeOnce sync.Once
	accepts   atomic.Uint32
}

func newRetryListener(conn net.Conn) *retryListener {
	return &retryListener{conn: conn, closed: make(chan struct{})}
}

func (l *retryListener) Accept() (net.Conn, error) {
	switch l.accepts.Add(1) {
	case 1:
		return nil, errors.New("temporary accept failure")
	case 2:
		return l.conn, nil
	default:
		<-l.closed
		return nil, net.ErrClosed
	}
}

func (l *retryListener) Close() error {
	l.closeOnce.Do(func() { close(l.closed) })
	return nil
}

func (l *retryListener) Addr() net.Addr { return l.conn.LocalAddr() }
