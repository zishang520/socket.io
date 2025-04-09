package engine

// TransportCtor defines the interface for transport constructors.
// This interface is used to create new transport instances with specific configurations.
type TransportCtor interface {
	// Name returns the identifier for this transport type.
	// This is used to identify the transport in configuration and upgrade processes.
	Name() string

	// New creates a new transport instance with the specified socket and options.
	//
	// Parameters:
	//   - socket: The parent socket instance
	//   - opts: The socket options configuration
	//
	// Returns: A new Transport instance
	New(Socket, SocketOptionsInterface) Transport
}

// HandshakeData represents the data exchanged during the initial handshake
// between the client and server. This data contains essential connection parameters
// and configuration options.
type HandshakeData struct {
	// Sid is the unique session identifier assigned by the server.
	// This ID is used to maintain the connection state and identify the client.
	Sid string `json:"sid,omitempty" msgpack:"sid,omitempty"`

	// Upgrades contains a list of transport types that the server supports
	// for upgrading the current connection. This is used in the transport
	// upgrade process to determine available upgrade options.
	Upgrades []string `json:"upgrades,omitempty" msgpack:"upgrades,omitempty"`

	// PingInterval specifies the interval (in milliseconds) between ping messages
	// sent by the client to keep the connection alive.
	PingInterval int64 `json:"pingInterval,omitempty" msgpack:"pingInterval,omitempty"`

	// PingTimeout specifies the maximum time (in milliseconds) to wait for a pong
	// response before considering the connection dead.
	PingTimeout int64 `json:"pingTimeout,omitempty" msgpack:"pingTimeout,omitempty"`

	// MaxPayload specifies the maximum size (in bytes) of a single packet that
	// can be transmitted over the connection.
	MaxPayload int64 `json:"maxPayload,omitempty" msgpack:"maxPayload,omitempty"`
}

// SocketState represents the current state of a Socket connection.
// This is used to track the lifecycle of the socket connection.
type SocketState string

const (
	// SocketStateOpening indicates that the socket is in the process of establishing
	// a connection with the server.
	SocketStateOpening SocketState = "opening"

	// SocketStateOpen indicates that the socket has successfully established a
	// connection with the server and is ready for communication.
	SocketStateOpen SocketState = "open"

	// SocketStateClosing indicates that the socket is in the process of closing
	// its connection with the server.
	SocketStateClosing SocketState = "closing"

	// SocketStateClosed indicates that the socket has completely closed its
	// connection with the server.
	SocketStateClosed SocketState = "closed"
)

// TransportState represents the current state of a Transport connection.
// This is used to track the lifecycle of individual transport connections.
type TransportState string

const (
	// TransportStateOpening indicates that the transport is in the process of
	// establishing a connection.
	TransportStateOpening TransportState = "opening"

	// TransportStateOpen indicates that the transport has successfully established
	// a connection and is ready for communication.
	TransportStateOpen TransportState = "open"

	// TransportStateClosed indicates that the transport has completely closed its
	// connection.
	TransportStateClosed TransportState = "closed"

	// TransportStatePausing indicates that the transport is in the process of
	// temporarily suspending its connection. This is typically used during
	// transport upgrades.
	TransportStatePausing TransportState = "pausing"

	// TransportStatePaused indicates that the transport has temporarily suspended
	// its connection. This state is maintained during transport upgrades.
	TransportStatePaused TransportState = "paused"
)
