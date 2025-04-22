package engine

import (
	"io"
	"net/http"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// SocketWithoutUpgrade provides a WebSocket-like interface to connect to an Engine.IO server.
// This implementation maintains a single transport connection without attempting to upgrade
// to more efficient transports after initial connection.
//
// Key features:
//   - Single transport connection (no upgrade mechanism)
//   - Event-based communication
//   - Support for binary data
//   - Tree-shaking friendly (transports must be explicitly included)
//
// Example usage:
//
//	socket := NewSocketWithoutUpgrade("http://localhost:8080", &SocketOptions{
//	    Transports: types.NewSet[string](transports.WEBSOCKET),
//	})
//
//	socket.On(SocketStateOpen, func() {
//	    socket.Send("hello")
//	})
//
// See: [SocketWithUpgrade] for an implementation with transport upgrade support
// See: [Socket] for the recommended high-level interface
type SocketWithoutUpgrade interface {
	// EventEmitter provides event-based communication capabilities
	types.EventEmitter

	// Prototype sets the prototype interface for method rewriting
	Prototype(SocketWithoutUpgrade)

	// Proto returns the prototype interface instance
	Proto() SocketWithoutUpgrade

	// SetPriorWebsocketSuccess updates the WebSocket success state
	// This is used to optimize future connection attempts
	SetPriorWebsocketSuccess(bool)

	// SetUpgrading updates the upgrading state of the socket
	// This indicates whether a transport upgrade is in progress
	SetUpgrading(bool)

	// Id returns the unique identifier for this socket connection
	Id() string

	// Transport returns the current transport being used
	Transport() Transport

	// ReadyState returns the current state of the socket connection
	ReadyState() SocketState

	// WriteBuffer returns the packet buffer for queued writes
	WriteBuffer() *types.Slice[*packet.Packet]

	// Opts returns the socket options configuration
	Opts() SocketOptionsInterface

	// Transports returns the set of available transport types
	Transports() *types.Slice[string]

	// Upgrading returns whether a transport upgrade is in progress
	Upgrading() bool

	// CookieJar returns the cookie jar used for HTTP requests
	CookieJar() http.CookieJar

	// PriorWebsocketSuccess returns whether previous WebSocket connections were successful
	PriorWebsocketSuccess() bool

	// Protocol returns the Engine.IO protocol version being used
	Protocol() int

	// Construct initializes the socket with the given URI and options
	Construct(string, SocketOptionsInterface)

	// CreateTransport creates a new transport instance of the specified type
	CreateTransport(string) Transport

	// SetTransport updates the current transport being used
	SetTransport(Transport)

	// OnOpen handles the socket open event
	OnOpen()

	// OnHandshake handles the initial handshake data from the server
	OnHandshake(*HandshakeData)

	// Flush sends all queued packets in the write buffer
	Flush()

	// HasPingExpired checks if the current ping timeout has expired
	HasPingExpired() bool

	// Write queues data to be sent to the server
	// Parameters:
	//   - data: The data to send
	//   - options: Optional packet configuration
	//   - callback: Optional callback to be called after the write completes
	Write(io.Reader, *packet.Options, func()) SocketWithoutUpgrade

	// Send is an alias for Write
	Send(io.Reader, *packet.Options, func()) SocketWithoutUpgrade

	// Close terminates the socket connection
	Close() SocketWithoutUpgrade
}

// SocketWithUpgrade extends SocketWithoutUpgrade to add transport upgrade capabilities.
// This implementation will attempt to upgrade the initial transport to a more efficient one
// after the connection is established.
//
// Key features:
//   - Automatic transport upgrade mechanism
//   - Fallback to lower-level transports if upgrade fails
//   - Event-based communication
//   - Support for binary data
//
// Example usage:
//
//	socket := NewSocketWithUpgrade("http://localhost:8080", &SocketOptions{
//	    Transports: types.NewSet[string](transports.WEBSOCKET),
//	})
//
//	socket.On("open", func() {
//	    socket.Send("hello")
//	})
//
// Events:
//   - "open": Emitted when the connection is established
//   - "close": Emitted when the connection is closed
//   - "error": Emitted when an error occurs
//   - "message": Emitted when data is received
//   - "upgrade": Emitted when transport upgrade is successful
//   - "upgradeError": Emitted when transport upgrade fails
//
// See: [SocketWithoutUpgrade] for the base implementation without upgrade support
// See: [Socket] for the recommended high-level interface
type SocketWithUpgrade interface {
	SocketWithoutUpgrade
}

// Socket provides the recommended high-level interface for Engine.IO client connections.
// This interface extends SocketWithUpgrade to provide the most feature-complete implementation
// with automatic transport upgrades and optimal performance.
//
// Key features:
//   - Automatic transport upgrade mechanism
//   - Multiple transport support (WebSocket, WebTransport, Polling)
//   - Event-based communication
//   - Support for binary data
//   - Automatic reconnection
//   - Comprehensive error handling
//   - Binary data support
//   - Cross-platform compatibility
//
// Example usage:
//
//	socket := NewSocket("http://localhost:8080", DefaultSocketOptions())
//
//	socket.On("open", func() {
//	    socket.Send("hello")
//	})
//
//	socket.On("message", func(data any) {
//	    fmt.Printf("Received: %v\n", data)
//	})
//
//	socket.On("error", func(err any) {
//	    fmt.Printf("Error: %v\n", err)
//	})
//
// Events:
//   - "open": Emitted when the connection is established
//   - "close": Emitted when the connection is closed
//   - "error": Emitted when an error occurs
//   - "message": Emitted when data is received
//   - "upgrade": Emitted when transport upgrade is successful
//   - "upgradeError": Emitted when transport upgrade fails
//   - "ping": Emitted when a ping packet is received
//   - "pong": Emitted when a pong packet is sent
//
// See: [SocketWithoutUpgrade] for a simpler implementation without transport upgrade
// See: [SocketWithUpgrade] for the base implementation with upgrade support
type Socket interface {
	SocketWithUpgrade
}
