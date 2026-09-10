package parser

import (
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// parserv4 implements the Engine.IO protocol version 4 parser.
type parserv4 struct{}

// defaultParserv4 is a singleton instance of the v4 parser.
var defaultParserv4 Parser = &parserv4{}

// Parserv4 returns the singleton Engine.IO v4 protocol parser.
func Parserv4() Parser {
	return defaultParserv4
}

// Protocol returns the protocol version (4).
func (*parserv4) Protocol() int {
	return Protocol
}

// EncodePacket encodes a single packet for Engine.IO v4 protocol.
// supportsBinary indicates whether the transport supports binary frames.
// The utf8encode parameter is ignored in v4 (kept for interface compatibility).
func (*parserv4) EncodePacket(pkt *packet.Packet, supportsBinary bool, _ ...bool) (types.BufferInterface, error) {
	if pkt == nil {
		return nil, ErrPacketNil
	}
	if c, ok := pkt.Data.(io.Closer); ok {
		defer func() { _ = c.Close() }()
	}

	typeByte, ok := lookupPacketByte(pkt.Type)
	if !ok {
		return nil, ErrPacketType
	}

	switch data := pkt.Data.(type) {
	case *types.StringBuffer, *strings.Reader:
		encoded := types.NewStringBuffer(nil)
		_ = encoded.WriteByte(typeByte)
		if _, err := io.Copy(encoded, data); err != nil {
			return nil, err
		}
		return encoded, nil
	case io.Reader:
		if supportsBinary {
			encoded := types.NewBytesBuffer(nil)
			if _, err := io.Copy(encoded, data); err != nil {
				return nil, err
			}
			return encoded, nil
		}

		encoded := types.NewStringBuffer(nil)
		_ = encoded.WriteByte('b')
		encoder := base64.NewEncoder(base64.StdEncoding, encoded)
		if _, err := io.Copy(encoder, data); err != nil {
			_ = encoder.Close()
			return nil, err
		}
		if err := encoder.Close(); err != nil {
			return nil, err
		}
		return encoded, nil
	default:
		return types.NewStringBuffer([]byte{typeByte}), nil
	}
}

// DecodePacket decodes a single packet from Engine.IO v4 wire format.
// The utf8decode parameter is ignored in v4 (kept for interface compatibility).
func (p *parserv4) DecodePacket(data types.BufferInterface, _ ...bool) (*packet.Packet, error) {
	if data == nil {
		return newErrorPacket(), ErrDataNil
	}

	// Handle string buffer (text data)
	if sb, ok := data.(*types.StringBuffer); ok {
		return p.decodeStringPacket(sb)
	}

	// Handle binary buffer - always a MESSAGE packet in v4
	return &packet.Packet{Type: packet.MESSAGE, Data: data}, nil
}

// decodeStringPacket decodes a text-based packet.
func (p *parserv4) decodeStringPacket(sb *types.StringBuffer) (*packet.Packet, error) {
	msgType, err := sb.ReadByte()
	if err != nil {
		return newErrorPacket(), err
	}

	// Handle base64-encoded binary data
	if msgType == 'b' {
		return p.decodeBase64Packet(sb)
	}

	packetType, ok := lookupPacketType(msgType)
	if !ok {
		return newErrorPacket(), fmt.Errorf("%w: [%c]", ErrUnknownPacketType, msgType)
	}

	return &packet.Packet{Type: packetType, Data: sb}, nil
}

// decodeBase64Packet decodes a base64-encoded binary packet.
func (p *parserv4) decodeBase64Packet(sb *types.StringBuffer) (*packet.Packet, error) {
	decode := types.NewBytesBuffer(nil)
	if _, err := decode.ReadFrom(base64.NewDecoder(base64.StdEncoding, sb)); err != nil {
		return newErrorPacket(), err
	}
	// Base64 packets are always MESSAGE type in v4
	return &packet.Packet{Type: packet.MESSAGE, Data: decode}, nil
}

// EncodePayload encodes multiple packets into a single payload for Engine.IO v4.
// Packets are separated by SEPARATOR (0x1E).
// The supportsBinary parameter is ignored in v4 (kept for interface compatibility).
func (p *parserv4) EncodePayload(packets []*packet.Packet, _ ...bool) (types.BufferInterface, error) {
	enPayload := types.NewStringBuffer(nil)

	if len(packets) == 0 {
		return enPayload, nil
	}

	for i, pkt := range packets {
		buf, err := p.EncodePacket(pkt, false)
		if err != nil {
			return nil, err
		}

		// Add separator before non-first packets
		if i > 0 {
			_ = enPayload.WriteByte(SEPARATOR)
		}

		_, _ = enPayload.Write(buf.Bytes())
	}

	return enPayload, nil
}

// DecodePayload decodes a payload buffer into multiple packets.
// Packets are separated by SEPARATOR (0x1E).
func (p *parserv4) DecodePayload(data types.BufferInterface) ([]*packet.Packet, error) {
	if data == nil {
		return nil, ErrDataNil
	}
	packets := make([]*packet.Packet, 0, 4)
	for {
		segment, err := data.ReadBytes(SEPARATOR)
		if err != nil && err != io.EOF {
			return packets, err
		}
		if err == nil {
			segment = segment[:len(segment)-1]
		}
		pkt, decodeErr := p.DecodePacket(types.NewStringBuffer(segment))
		if decodeErr != nil {
			return packets, decodeErr
		}
		packets = append(packets, pkt)
		if err == io.EOF {
			return packets, nil
		}
	}
}
