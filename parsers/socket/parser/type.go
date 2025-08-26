package parser

type (
	PacketType int

	Packet struct {
		Type        PacketType `json:"type" msgpack:"type"`
		Nsp         string     `json:"nsp" msgpack:"nsp"`
		Data        any        `json:"data,omitempty" msgpack:"data,omitempty"`
		Id          *uint64    `json:"id,omitempty" msgpack:"id,omitempty"`
		Attachments *uint64    `json:"attachments,omitempty" msgpack:"attachments,omitempty"`
	}
)

const (
	CONNECT PacketType = iota
	DISCONNECT
	EVENT
	ACK
	CONNECT_ERROR
	BINARY_EVENT
	BINARY_ACK
)

func (t PacketType) Valid() bool {
	return t >= 0 && t <= 6
}

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
	}
	return "UNKNOWN"
}
