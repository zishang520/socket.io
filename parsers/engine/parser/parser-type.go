// Package parser provides Engine.IO protocol packet encoding and decoding.
package parser

import (
	"errors"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Parser defines the interface for Engine.IO packet encoding and decoding.
type Parser interface {
	// Protocol returns the Engine.IO protocol version.
	Protocol() int
	// EncodePacket encodes a packet to a buffer.
	// supportsBinary indicates whether the transport supports binary data.
	// utf8encode (v3 only) indicates whether to perform UTF-8 encoding.
	EncodePacket(pkt *packet.Packet, supportsBinary bool, utf8encode ...bool) (types.BufferInterface, error)
	// DecodePacket decodes a buffer to a packet.
	// utf8decode (v3 only) indicates whether to perform UTF-8 decoding.
	DecodePacket(data types.BufferInterface, utf8decode ...bool) (*packet.Packet, error)
	// EncodePayload encodes multiple packets into a single payload.
	// supportsBinary (v3 only) indicates whether to use binary encoding.
	EncodePayload(packets []*packet.Packet, supportsBinary ...bool) (types.BufferInterface, error)
	// DecodePayload decodes a payload buffer into multiple packets.
	DecodePayload(data types.BufferInterface) ([]*packet.Packet, error)
}

// Protocol constants.
const (
	// SEPARATOR is the packet separator for Engine.IO v4 payloads.
	SEPARATOR byte = 0x1E
	// Protocol is the current Engine.IO protocol version.
	Protocol int = 4
)

// Packet type to byte mappings for wire format.
var (
	// PACKET_TYPES maps packet types to their wire format bytes.
	PACKET_TYPES = map[packet.Type]byte{
		packet.OPEN:    '0',
		packet.CLOSE:   '1',
		packet.PING:    '2',
		packet.PONG:    '3',
		packet.MESSAGE: '4',
		packet.UPGRADE: '5',
		packet.NOOP:    '6',
	}

	// PACKET_TYPES_REVERSE maps wire format bytes back to packet types.
	PACKET_TYPES_REVERSE = map[byte]packet.Type{
		'0': packet.OPEN,
		'1': packet.CLOSE,
		'2': packet.PING,
		'3': packet.PONG,
		'4': packet.MESSAGE,
		'5': packet.UPGRADE,
		'6': packet.NOOP,
	}
)

// Pre-defined error packet for parser errors.
var ERROR_PACKET = &packet.Packet{
	Type: packet.ERROR,
	Data: types.NewStringBufferString(`parser error`),
}

// Sentinel errors for parser operations.
var (
	// ErrPacketNil is returned when the packet is nil.
	ErrPacketNil = errors.New("packet must not be nil")
	// ErrPacketType is returned when the packet type is invalid.
	ErrPacketType = errors.New("invalid packet type")
	// ErrDataNil is returned when the data is nil.
	ErrDataNil = errors.New("data must not be nil")
	// ErrInvalidDataLength is returned when the data length is invalid.
	ErrInvalidDataLength = errors.New("invalid data length")
	// ErrParser is returned when a parsing error occurs.
	ErrParser = errors.New("parsing error")
	// ErrUnknownPacketType is returned when an unknown packet type byte is encountered.
	ErrUnknownPacketType = errors.New("unknown packet type")
)

// lookupPacketType returns the packet type for the given byte.
// Returns the packet type and true if found, otherwise empty type and false.
func lookupPacketType(b byte) (packet.Type, bool) {
	pt, ok := PACKET_TYPES_REVERSE[b]
	return pt, ok
}

// lookupPacketByte returns the wire format byte for the given packet type.
// Returns the byte and true if found, otherwise 0 and false.
func lookupPacketByte(t packet.Type) (byte, bool) {
	b, ok := PACKET_TYPES[t]
	return b, ok
}
