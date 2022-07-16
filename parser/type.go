package parser

type PacketType byte

func (t PacketType) Valid() bool {
	return t >= '0' && t <= '6'
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

type Packet struct {
	Type        PacketType
	Nsp         string
	Data        interface{}
	Id          uint64
	Attachments uint64
}
