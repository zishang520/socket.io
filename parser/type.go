package parser

type PacketType byte

type Packet struct {
	Type        PacketType
	Nsp         string
	Data        any
	Id          uint64
	Attachments uint64
}

const (
	CONNECT       PacketType = '0'
	DISCONNECT    PacketType = '1'
	EVENT         PacketType = '2'
	ACK           PacketType = '3'
	CONNECT_ERROR PacketType = '4'
	BINARY_EVENT  PacketType = '5'
	BINARY_ACK    PacketType = '6'
)

func (t PacketType) Valid() bool {
	return t >= '0' && t <= '6'
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
