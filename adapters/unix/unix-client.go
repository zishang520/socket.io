// Package unix provides a Unix Domain Socket client wrapper for Socket.IO Unix adapter.
// This package offers a unified interface for Unix Domain Socket operations with event handling
// support using datagram or stream connections for pub/sub communication.
package unix

import (
	"context"
	"fmt"
	"net"
	"sync"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// UnixClient wraps a Unix Domain Socket connection and provides context management
// and event emitting capabilities for the Socket.IO Unix adapter.
//
// The client uses a datagram-oriented Unix Domain Socket for pub/sub messaging.
// Messages are sent and received via the Unix socket connection.
//
// The client supports error event emission, which allows higher-level components
// to handle Unix socket-related errors gracefully.
type UnixClient struct {
	types.EventEmitter

	// SocketPath is the path of the Unix Domain Socket used for communication.
	SocketPath string

	// Context is the context used for Unix socket operations.
	// This context controls the lifecycle of operations.
	Context context.Context

	// conn is the Unix Domain Socket connection for sending messages.
	conn net.Conn
	mu   sync.Mutex

	// listener is the Unix Domain Socket listener for receiving messages.
	listener net.PacketConn
	// listenerPath is the unique path for this client's listening socket.
	listenerPath string
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

	return &UnixClient{
		EventEmitter: types.NewEventEmitter(),
		SocketPath:   socketPath,
		Context:      ctx,
	}
}

// Listen starts listening for incoming Unix Domain Socket messages on a unique path.
// The listener path is derived from the main socket path with the given suffix.
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

	listener, err := net.ListenPacket("unixgram", listenerPath)
	if err != nil {
		return fmt.Errorf("failed to listen on Unix socket %q: %w", listenerPath, err)
	}

	c.listener = listener
	return nil
}

// ReadMessage reads a message from the listener socket.
// This method blocks until a message is received or the context is canceled.
//
// Returns the received message bytes and the sender address, or an error.
func (c *UnixClient) ReadMessage(buf []byte) (int, net.Addr, error) {
	c.mu.Lock()
	listener := c.listener
	c.mu.Unlock()

	if listener == nil {
		return 0, nil, fmt.Errorf("listener not started")
	}

	return listener.ReadFrom(buf)
}

// Send sends a message to the specified Unix Domain Socket path.
//
// Parameters:
//   - targetPath: The path of the target Unix Domain Socket.
//   - payload: The message payload bytes.
func (c *UnixClient) Send(targetPath string, payload []byte) error {
	addr, err := net.ResolveUnixAddr("unixgram", targetPath)
	if err != nil {
		return fmt.Errorf("failed to resolve Unix address %q: %w", targetPath, err)
	}

	conn, err := net.DialUnix("unixgram", nil, addr)
	if err != nil {
		return fmt.Errorf("failed to dial Unix socket %q: %w", targetPath, err)
	}
	defer conn.Close()

	if _, err := conn.Write(payload); err != nil {
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

// Close closes the Unix Domain Socket connections and cleans up resources.
func (c *UnixClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var errs []error

	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			errs = append(errs, err)
		}
		c.conn = nil
	}

	if c.listener != nil {
		if err := c.listener.Close(); err != nil {
			errs = append(errs, err)
		}
		c.listener = nil
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing Unix client: %v", errs)
	}
	return nil
}
