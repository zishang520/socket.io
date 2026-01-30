package parser

// PacketType represents the type of a Socket.IO packet.
type PacketType int

// Socket.IO packet types as defined in the protocol.
const (
	// CONNECT is used to establish a connection with the server.
	CONNECT PacketType = iota
	// DISCONNECT is used to gracefully close a connection.
	DISCONNECT
	// EVENT is used to send an event with data.
	EVENT
	// ACK is used to acknowledge a received event.
	ACK
	// CONNECT_ERROR is used to report a connection error.
	CONNECT_ERROR
	// BINARY_EVENT is used to send an event containing binary data.
	BINARY_EVENT
	// BINARY_ACK is used to acknowledge a received binary event.
	BINARY_ACK
)

// Valid returns true if the packet type is a valid Socket.IO packet type.
func (t PacketType) Valid() bool {
	return t >= CONNECT && t <= BINARY_ACK
}

// String returns the string representation of the packet type.
func (t PacketType) String() string {
	switch t {
	case CONNECT:
		return "CONNECT"
	case DISCONNECT:
		return "DISCONNECT"
	case EVENT:
		return "EVENT"
	case ACK:
		return "ACK"
	case CONNECT_ERROR:
		return "CONNECT_ERROR"
	case BINARY_EVENT:
		return "BINARY_EVENT"
	case BINARY_ACK:
		return "BINARY_ACK"
	default:
		return "UNKNOWN"
	}
}

// Packet represents a Socket.IO packet.
// It contains the type, namespace, data, optional ID for acknowledgment,
// and the count of binary attachments for binary packets.
type Packet struct {
	// Type is the packet type.
	Type PacketType `json:"type" msgpack:"type"`
	// Nsp is the namespace this packet belongs to.
	Nsp string `json:"nsp" msgpack:"nsp"`
	// Data is the payload of the packet.
	Data any `json:"data,omitempty" msgpack:"data,omitempty"`
	// Id is the optional packet ID for acknowledgment.
	Id *uint64 `json:"id,omitempty" msgpack:"id,omitempty"`
	// Attachments is the number of binary attachments for binary packets.
	Attachments *uint64 `json:"attachments,omitempty" msgpack:"attachments,omitempty"`
}
