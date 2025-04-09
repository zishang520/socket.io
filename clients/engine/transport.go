package engine

import (
	"context"
	"net/url"
	"strings"
	"sync/atomic"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

// Transport represents the base transport implementation that provides common functionality
// for all transport types (WebSocket, WebTransport, Polling, etc.).
// It implements the basic lifecycle of a transport connection and provides event-based
// communication capabilities.
//
// Transport instances handle the low-level details of establishing and maintaining
// connections with the server, including connection state management, packet
// encoding/decoding, and error handling.
type transport struct {
	types.EventEmitter

	// _proto_ is the prototype interface used for method rewriting in Go.
	// This allows for proper interface implementation and method overriding.
	_proto_ Transport

	// query contains the URL query parameters for the transport connection.
	// These parameters are used in the connection URL and can include configuration options.
	query url.Values

	// writable indicates whether the transport is currently able to send data.
	// This is an atomic boolean to ensure thread-safe access.
	writable atomic.Bool

	// opts contains the socket options configuration for this transport.
	// This includes settings like host, port, security, and other connection parameters.
	opts SocketOptionsInterface

	// supportsBinary indicates whether the transport supports binary data transmission.
	// This is determined by the ForceBase64 option in the socket configuration.
	supportsBinary bool

	// readyState represents the current state of the transport connection.
	// This is an atomic pointer to ensure thread-safe state management.
	readyState atomic.Pointer[TransportState]

	// socket is the parent socket instance that owns this transport.
	// It's used for communication between the transport and the socket.
	socket Socket
}

// Prototype sets the prototype interface for method rewriting.
// This is used to implement proper interface inheritance in Go.
//
// Parameters:
//   - _proto_: The prototype interface to be used for method rewriting
func (s *transport) Prototype(_proto_ Transport) {
	s._proto_ = _proto_
}

// Proto returns the prototype interface instance.
//
// Returns:
//   - Transport: The current prototype interface implementation
func (s *transport) Proto() Transport {
	return s._proto_
}

// Query returns the URL query parameters for the transport.
//
// Returns:
//   - url.Values: The current query parameters
func (t *transport) Query() url.Values {
	return t.query
}

// SetWritable updates the writable state of the transport.
// This is used to control whether the transport can send data.
//
// Parameters:
//   - writable: The new writable state to set
func (t *transport) SetWritable(writable bool) {
	t.writable.Store(writable)
}

// Writable returns whether the transport is currently able to send data.
//
// Returns:
//   - bool: true if the transport can send data, false otherwise
func (t *transport) Writable() bool {
	return t.writable.Load()
}

// Opts returns the socket options configuration for this transport.
//
// Returns:
//   - SocketOptionsInterface: The current socket options configuration
func (t *transport) Opts() SocketOptionsInterface {
	return t.opts
}

// SupportsBinary returns whether the transport supports binary data transmission.
//
// Returns:
//   - bool: true if binary data is supported, false if base64 encoding is required
func (t *transport) SupportsBinary() bool {
	return t.supportsBinary
}

// SetReadyState updates the current state of the transport connection.
// This is used to track the lifecycle of the transport (opening, open, closed).
//
// Parameters:
//   - readyState: The new state to set for the transport
func (t *transport) SetReadyState(readyState TransportState) {
	t.readyState.Store(&readyState)
}

// ReadyState returns the current state of the transport connection.
//
// Returns:
//   - TransportState: The current state of the transport, or empty string if no state is set
func (t *transport) ReadyState() TransportState {
	if readyState := t.readyState.Load(); readyState != nil {
		return *readyState
	}
	return ""
}

// Socket returns the parent socket instance that owns this transport.
//
// Returns:
//   - Socket: The parent socket instance
func (t *transport) Socket() Socket {
	return t.socket
}

// MakeTransport creates a new transport instance with default settings.
// This is the factory function for creating a new transport.
//
// Returns:
//   - Transport: A new transport instance initialized with default settings
func MakeTransport() Transport {
	s := &transport{
		EventEmitter: types.NewEventEmitter(),
	}

	s.writable.Store(false)

	s.Prototype(s)

	return s
}

// NewTransport creates a new transport instance with the specified socket and options.
//
// Parameters:
//   - socket: The parent socket instance
//   - opts: The socket options configuration
//
// Returns:
//   - Transport: A new transport instance configured with the specified options
func NewTransport(socket Socket, opts SocketOptionsInterface) Transport {
	s := MakeTransport()

	s.Construct(socket, opts)

	return s
}

// Construct initializes the transport with the given socket and options.
// This is an internal method used by NewTransport to set up the connection.
//
// Parameters:
//   - socket: The parent socket instance
//   - opts: The socket options configuration
func (t *transport) Construct(socket Socket, opts SocketOptionsInterface) {
	t.opts = opts
	t.query = opts.Query()
	t.socket = socket
	t.supportsBinary = !opts.ForceBase64()
}

// OnError emits an error event with the specified reason and description.
// This is used to handle transport-level errors.
//
// Parameters:
//   - reason: A string describing the error
//   - description: The underlying error that caused this transport error
//   - context: Additional context information about the error
//
// Returns:
//   - Transport: The transport instance for method chaining
func (t *transport) OnError(reason string, description error, context context.Context) Transport {
	t.Emit("error", NewTransportError(reason, description, context).Err())
	return t
}

// Open initiates the transport connection.
// This sets the ready state to opening and calls the transport-specific open implementation.
//
// Returns:
//   - Transport: The transport instance for method chaining
func (t *transport) Open() Transport {
	t.SetReadyState(TransportStateOpening)
	t._proto_.DoOpen()

	return t
}

// Close terminates the transport connection.
// This is called when the transport needs to be closed, either due to an error
// or normal shutdown.
//
// Returns:
//   - Transport: The transport instance for method chaining
func (t *transport) Close() Transport {
	if readyState := t.ReadyState(); TransportStateOpening == readyState || TransportStateOpen == readyState {
		t._proto_.DoClose()
		t._proto_.OnClose(nil)
	}

	return t
}

// Send transmits multiple packets through the transport.
// This is only possible when the transport is in the open state.
//
// Parameters:
//   - packets: An array of packets to be sent
func (t *transport) Send(packets []*packet.Packet) {
	if TransportStateOpen == t.ReadyState() {
		t._proto_.Write(packets)
	} else {
		// this might happen if the transport was silently closed in the beforeunload event handler
		client_transport_log.Debug("transport is not open, discarding packets")
	}
}

// OnOpen is called when the transport connection is successfully established.
// This updates the ready state and emits an open event.
func (t *transport) OnOpen() {
	t.SetReadyState(TransportStateOpen)
	t.SetWritable(true)
	t.Emit("open")
}

// OnData processes incoming data from the transport.
// This decodes the data into packets and forwards them to OnPacket.
//
// Parameters:
//   - data: The raw data received from the transport
func (t *transport) OnData(data types.BufferInterface) {
	p, _ := parser.Parserv4().DecodePacket(data)
	t.OnPacket(p)
}

// OnPacket handles decoded packets from the transport.
// This emits a packet event with the decoded data.
//
// Parameters:
//   - data: The decoded packet
func (t *transport) OnPacket(data *packet.Packet) {
	t.Emit("packet", data)
}

// OnClose is called when the transport connection is closed.
// This updates the ready state and emits a close event with any error details.
//
// Parameters:
//   - details: Optional error details about why the connection was closed
func (t *transport) OnClose(details error) {
	t.SetReadyState(TransportStateClosed)
	t.Emit("close", details)
}

// Name returns the name of the transport.
// This is implemented by specific transport types.
//
// Returns:
//   - string: The transport name
func (t *transport) Name() string { return "" }

// Pause temporarily suspends the transport to prevent packet loss during upgrades.
// This is implemented by specific transport types.
//
// Parameters:
//   - func(): A callback function to be called when the transport is paused
func (t *transport) Pause(func()) {}

// CreateUri constructs a URL for the transport connection.
// This combines the schema, hostname, port, and path with any query parameters.
//
// Parameters:
//   - schema: The URL schema (e.g., "ws", "http")
//   - query: Optional query parameters to include in the URL
//
// Returns:
//   - *url.URL: The constructed URL for the transport connection
func (t *transport) CreateUri(schema string, query url.Values) *url.URL {
	uri := &url.URL{
		Scheme: schema,
		Host:   t._hostname() + t._port(),
		Path:   t.opts.Path(),
	}
	if query != nil {
		uri.RawQuery = query.Encode()
	}
	return uri
}

// _hostname returns the formatted hostname for the transport.
// This handles IPv6 addresses by wrapping them in square brackets.
//
// Returns:
//   - string: The formatted hostname
func (t *transport) _hostname() string {
	hostname := t.opts.Hostname()
	if strings.Contains(hostname, ":") {
		return "[" + hostname + "]"
	}
	return hostname
}

// _port returns the formatted port string for the transport.
// This only includes the port if it's not the default port for the protocol.
//
// Returns:
//   - string: The formatted port string, including the colon if needed
func (t *transport) _port() string {
	port := t.opts.Port()
	if port != "" && ((t.opts.Secure() && port != "443") || (!t.opts.Secure() && port != "80")) {
		return ":" + port
	}
	return ""
}

// DoOpen is a placeholder method that should be implemented by specific transport types.
// It handles the actual opening of the transport connection.
func (t *transport) DoOpen() {}

// DoClose is a placeholder method that should be implemented by specific transport types.
// It handles the actual closing of the transport connection.
func (t *transport) DoClose() {}

// Write is a placeholder method that should be implemented by specific transport types.
// It handles the actual writing of packets to the transport.
//
// Parameters:
//   - []*packet.Packet: The packets to be written to the transport
func (t *transport) Write([]*packet.Packet) {}
