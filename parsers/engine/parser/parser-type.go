package parser

import (
	"errors"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type Parser interface {
	Protocol() int
	EncodePacket(*packet.Packet, bool, ...bool) (types.BufferInterface, error)
	DecodePacket(types.BufferInterface, ...bool) (*packet.Packet, error)
	EncodePayload([]*packet.Packet, ...bool) (types.BufferInterface, error)
	DecodePayload(types.BufferInterface) ([]*packet.Packet, error)
}

const (
	SEPARATOR byte = 0x1E
	Protocol  int  = 4
)

// Packet types.
var (
	PACKET_TYPES map[packet.Type]byte = map[packet.Type]byte{
		packet.OPEN:    '0',
		packet.CLOSE:   '1',
		packet.PING:    '2',
		packet.PONG:    '3',
		packet.MESSAGE: '4',
		packet.UPGRADE: '5',
		packet.NOOP:    '6',
	}

	PACKET_TYPES_REVERSE map[byte]packet.Type = map[byte]packet.Type{
		'0': packet.OPEN,
		'1': packet.CLOSE,
		'2': packet.PING,
		'3': packet.PONG,
		'4': packet.MESSAGE,
		'5': packet.UPGRADE,
		'6': packet.NOOP,
	}

	// Premade error packet.
	ERROR_PACKET = &packet.Packet{Type: packet.ERROR, Data: types.NewStringBufferString(`parser error`)}

	ErrPacketNil         = errors.New("Packet must not be nil")
	ErrPacketType        = errors.New("Invalid packet type")
	ErrDataNil           = errors.New("Data must not be nil")
	ErrInvalidDataLength = errors.New("Invalid data length")
	ErrParser            = errors.New("Parsing error")
)
