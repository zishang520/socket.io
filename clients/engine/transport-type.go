package engine

import (
	"context"
	"net/url"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	// Transport defines the interface for all transport implementations in Engine.IO.
	// It provides a common set of methods that all transport types (WebSocket,
	// HTTP long-polling, WebTransport) must implement to ensure consistent
	// behavior across different transport mechanisms.
	//
	// The Transport interface is designed to be transport-agnostic, allowing
	// different transport implementations to provide their specific functionality
	// while maintaining a consistent API for the higher-level socket layer.
	Transport interface {
		// EventEmitter provides event-based communication capabilities
		types.EventEmitter

		// Prototype sets the prototype interface for method rewriting
		Prototype(Transport)

		// Proto returns the prototype interface instance
		Proto() Transport

		// SetWritable updates whether the transport can send data
		SetWritable(bool)

		// SetReadyState updates the current state of the transport
		SetReadyState(TransportState)

		// Name returns the identifier for this transport type
		Name() string

		// Query returns the URL query parameters for the transport
		Query() url.Values

		// Writable returns whether the transport can currently send data
		Writable() bool

		// Opts returns the socket options configuration
		Opts() SocketOptionsInterface

		// SupportsBinary returns whether the transport supports binary data
		SupportsBinary() bool

		// ReadyState returns the current state of the transport
		ReadyState() TransportState

		// Socket returns the parent socket instance
		Socket() Socket

		// Construct initializes the transport with the given socket and options
		Construct(Socket, SocketOptionsInterface)

		// OnError handles transport-level errors
		OnError(string, error, context.Context) Transport

		// Open initiates the transport connection
		Open() Transport

		// Close terminates the transport connection
		Close() Transport

		// Send transmits packets through the transport
		Send([]*packet.Packet)

		// OnOpen handles successful connection establishment
		OnOpen()

		// OnData processes incoming raw data
		OnData(types.BufferInterface)

		// OnPacket handles decoded packets
		OnPacket(*packet.Packet)

		// OnClose handles connection termination
		OnClose(error)

		// Pause temporarily suspends the transport
		Pause(func())

		// CreateUri constructs a URL for the transport connection
		CreateUri(string, url.Values) *url.URL

		// DoOpen implements transport-specific connection initialization
		DoOpen()

		// DoClose implements transport-specific connection termination
		DoClose()

		// Write implements transport-specific packet transmission
		Write([]*packet.Packet)
	}

	// Polling represents the HTTP long-polling transport type.
	// This transport uses regular HTTP requests to simulate real-time communication
	// by maintaining a persistent connection through repeated requests.
	//
	// Features:
	//   - Maximum compatibility across browsers and networks
	//   - Works through most proxies and firewalls
	//   - Fallback transport when WebSocket is not available
	//   - Automatic reconnection handling
	Polling interface {
		Transport
	}

	// WebSocket represents the WebSocket transport type.
	// This transport provides full-duplex communication over a single TCP connection,
	// offering better performance than polling.
	//
	// Features:
	//   - Full-duplex communication
	//   - Lower latency than polling
	//   - Binary data support
	//   - Built-in heartbeat mechanism
	//   - Automatic reconnection
	WebSocket interface {
		Transport
	}

	// WebTransport represents the WebTransport transport type.
	// This transport provides low-latency, bidirectional communication using the
	// QUIC protocol, offering several advantages over WebSocket.
	//
	// Features:
	//   - Lower latency than WebSocket
	//   - Better multiplexing support
	//   - Built-in congestion control
	//   - Support for unreliable datagrams
	//   - Independent streams for parallel data transfer
	//   - Modern security features
	//
	// Note: WebTransport requires browser support and a compatible server.
	// See: https://developer.mozilla.org/en-US/docs/Web/API/WebTransport
	// See: https://caniuse.com/webtransport
	WebTransport interface {
		Transport
	}
)

// WebSocketBuilder implements the transport builder pattern for WebSocket connections.
// It provides a factory method for creating new WebSocket transport instances.
type WebSocketBuilder struct{}

// New creates a new WebSocket transport instance.
//
// Parameters:
//   - socket: The parent socket instance
//   - opts: The socket options configuration
//
// Returns:
//   - Transport: A new WebSocket transport instance
func (*WebSocketBuilder) New(socket Socket, opts SocketOptionsInterface) Transport {
	return NewWebSocket(socket, opts)
}

// Name returns the identifier for the WebSocket transport type.
//
// Returns:
//   - string: The transport name ("websocket")
func (*WebSocketBuilder) Name() string {
	return transports.WEBSOCKET
}

// WebTransportBuilder implements the transport builder pattern for WebTransport connections.
// It provides a factory method for creating new WebTransport instances.
type WebTransportBuilder struct{}

// New creates a new WebTransport instance.
//
// Parameters:
//   - socket: The parent socket instance
//   - opts: The socket options configuration
//
// Returns:
//   - Transport: A new WebTransport instance
func (*WebTransportBuilder) New(socket Socket, opts SocketOptionsInterface) Transport {
	return NewWebTransport(socket, opts)
}

// Name returns the identifier for the WebTransport transport type.
//
// Returns:
//   - string: The transport name ("webtransport")
func (*WebTransportBuilder) Name() string {
	return transports.WEBTRANSPORT
}

// PollingBuilder implements the transport builder pattern for HTTP long-polling connections.
// It provides a factory method for creating new Polling transport instances.
type PollingBuilder struct{}

// New creates a new Polling transport instance.
//
// Parameters:
//   - socket: The parent socket instance
//   - opts: The socket options configuration
//
// Returns:
//   - Transport: A new Polling transport instance
func (*PollingBuilder) New(socket Socket, opts SocketOptionsInterface) Transport {
	return NewPolling(socket, opts)
}

// Name returns the identifier for the Polling transport type.
//
// Returns:
//   - string: The transport name ("polling")
func (*PollingBuilder) Name() string {
	return transports.POLLING
}
