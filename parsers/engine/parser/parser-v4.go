package parser

import (
	"bufio"
	"bytes"
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
func (p *parserv4) EncodePacket(pkt *packet.Packet, supportsBinary bool, _ ...bool) (types.BufferInterface, error) {
	if pkt == nil {
		return nil, ErrPacketNil
	}

	// Ensure data is closed if it implements io.Closer
	if c, ok := pkt.Data.(io.Closer); ok {
		defer c.Close()
	}

	typeByte, ok := lookupPacketByte(pkt.Type)
	if !ok {
		return nil, ErrPacketType
	}

	switch v := pkt.Data.(type) {
	case *types.StringBuffer, *strings.Reader:
		return p.encodeStringData(typeByte, v)

	case io.Reader:
		return p.encodeBinaryData(v, supportsBinary)
	}

	// Packet with no data - just write the type byte
	return p.encodeEmptyPacket(typeByte)
}

// encodeStringData encodes string data for v4 protocol.
func (p *parserv4) encodeStringData(typeByte byte, data io.Reader) (types.BufferInterface, error) {
	encode := types.NewStringBuffer(nil)
	if err := encode.WriteByte(typeByte); err != nil {
		return nil, err
	}
	if _, err := io.Copy(encode, data); err != nil {
		return nil, err
	}
	return encode, nil
}

// encodeBinaryData encodes binary data, using base64 if binary is not supported.
func (p *parserv4) encodeBinaryData(data io.Reader, supportsBinary bool) (types.BufferInterface, error) {
	if !supportsBinary {
		return p.encodeAsBase64(data)
	}

	// Binary support - write raw bytes (no type prefix in v4)
	encode := types.NewBytesBuffer(nil)
	if _, err := io.Copy(encode, data); err != nil {
		return nil, err
	}
	return encode, nil
}

// encodeAsBase64 encodes data as base64 for transports that don't support binary.
func (p *parserv4) encodeAsBase64(data io.Reader) (types.BufferInterface, error) {
	encode := types.NewStringBuffer(nil)
	if err := encode.WriteByte('b'); err != nil {
		return nil, err
	}

	b64 := base64.NewEncoder(base64.StdEncoding, encode)
	if _, err := io.Copy(b64, data); err != nil {
		b64.Close()
		return nil, err
	}
	if err := b64.Close(); err != nil {
		return nil, err
	}
	return encode, nil
}

// encodeEmptyPacket encodes a packet with no data.
func (p *parserv4) encodeEmptyPacket(typeByte byte) (types.BufferInterface, error) {
	encode := types.NewStringBuffer(nil)
	if err := encode.WriteByte(typeByte); err != nil {
		return nil, err
	}
	return encode, nil
}

// DecodePacket decodes a single packet from Engine.IO v4 wire format.
// The utf8decode parameter is ignored in v4 (kept for interface compatibility).
func (p *parserv4) DecodePacket(data types.BufferInterface, _ ...bool) (*packet.Packet, error) {
	if data == nil {
		return ERROR_PACKET, ErrDataNil
	}

	// Handle string buffer (text data)
	if sb, ok := data.(*types.StringBuffer); ok {
		return p.decodeStringPacket(sb)
	}

	// Handle binary buffer - always a MESSAGE packet in v4
	return p.decodeBinaryPacket(data)
}

// decodeStringPacket decodes a text-based packet.
func (p *parserv4) decodeStringPacket(sb *types.StringBuffer) (*packet.Packet, error) {
	msgType, err := sb.ReadByte()
	if err != nil {
		return ERROR_PACKET, err
	}

	// Handle base64-encoded binary data
	if msgType == 'b' {
		return p.decodeBase64Packet(sb)
	}

	packetType, ok := lookupPacketType(msgType)
	if !ok {
		return ERROR_PACKET, fmt.Errorf("%w: [%c]", ErrUnknownPacketType, msgType)
	}

	stringBuffer := types.NewStringBuffer(nil)
	if _, err := stringBuffer.ReadFrom(sb); err != nil {
		return ERROR_PACKET, err
	}
	return &packet.Packet{Type: packetType, Data: stringBuffer}, nil
}

// decodeBase64Packet decodes a base64-encoded binary packet.
func (p *parserv4) decodeBase64Packet(sb *types.StringBuffer) (*packet.Packet, error) {
	decode := types.NewBytesBuffer(nil)
	if _, err := decode.ReadFrom(base64.NewDecoder(base64.StdEncoding, sb)); err != nil {
		return ERROR_PACKET, err
	}
	// Base64 packets are always MESSAGE type in v4
	return &packet.Packet{Type: packet.MESSAGE, Data: decode}, nil
}

// decodeBinaryPacket decodes a raw binary packet.
// Binary packets are always MESSAGE type in v4.
func (p *parserv4) decodeBinaryPacket(data types.BufferInterface) (*packet.Packet, error) {
	decode := types.NewBytesBuffer(nil)
	if _, err := io.Copy(decode, data); err != nil {
		return ERROR_PACKET, err
	}
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
			if err := enPayload.WriteByte(SEPARATOR); err != nil {
				return nil, err
			}
		}

		if _, err := io.Copy(enPayload, buf); err != nil {
			return nil, err
		}
	}

	return enPayload, nil
}

// separatorSplitFunc is a bufio.SplitFunc that splits on SEPARATOR bytes.
func separatorSplitFunc(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, SEPARATOR); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}

// DecodePayload decodes a payload buffer into multiple packets.
// Packets are separated by SEPARATOR (0x1E).
func (p *parserv4) DecodePayload(data types.BufferInterface) ([]*packet.Packet, error) {
	scanner := bufio.NewScanner(data)
	scanner.Split(separatorSplitFunc)

	packets := make([]*packet.Packet, 0, 4)

	for scanner.Scan() {
		scanBytes := scanner.Bytes()
		if len(scanBytes) == 0 {
			continue
		}

		pkt, err := p.DecodePacket(types.NewStringBuffer(scanBytes))
		if err != nil {
			return packets, err
		}
		packets = append(packets, pkt)
	}

	return packets, scanner.Err()
}
