package parser

import (
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type parserv3 struct{}

var defaultParserv3 Parser = &parserv3{}

// Parserv3 returns the singleton Engine.IO v3 protocol parser.
func Parserv3() Parser {
	return defaultParserv3
}

// Protocol returns the current protocol version.
func (*parserv3) Protocol() int {
	return 3
}

// EncodePacket encodes a single packet for Engine.IO v3 protocol.
func (p *parserv3) EncodePacket(data *packet.Packet, supportsBinary bool, utf8encode ...bool) (types.BufferInterface, error) {
	if data == nil {
		return nil, ErrPacketNil
	}

	if c, ok := data.Data.(io.Closer); ok {
		defer func() { _ = c.Close() }()
	}

	utf8en := len(utf8encode) > 0 && utf8encode[0]

	switch v := data.Data.(type) {
	case *types.StringBuffer, *strings.Reader:
		// Sending data as a utf-8 string
		encode := types.NewStringBuffer(nil)
		typeByte, ok := lookupPacketByte(data.Type)
		if !ok {
			return nil, ErrPacketType
		}
		_ = encode.WriteByte(typeByte)
		// data fragment is optional
		if utf8en {
			if _, err := io.Copy(utils.NewUtf8Encoder(encode), v); err != nil {
				return nil, err
			}
		} else {
			if _, err := io.Copy(encode, v); err != nil {
				return nil, err
			}
		}
		return encode, nil

	case io.Reader:
		typeByte, ok := lookupPacketByte(data.Type)
		if !ok {
			return nil, ErrPacketType
		}
		// Encode Buffer data
		if !supportsBinary {
			// Encodes a packet with binary data in a base64 string
			encode := types.NewStringBuffer(nil)
			_, _ = encode.Write([]byte{'b', typeByte})
			b64 := base64.NewEncoder(base64.StdEncoding, encode)
			if _, err := io.Copy(b64, v); err != nil {
				_ = b64.Close()
				return nil, err
			}
			if err := b64.Close(); err != nil {
				return nil, err
			}
			return encode, nil
		}
		encode := types.NewBytesBuffer(nil)
		_ = encode.WriteByte(typeByte - '0')
		if _, err := io.Copy(encode, v); err != nil {
			return nil, err
		}
		return encode, nil
	}

	// default nil data
	typeByte, ok := lookupPacketByte(data.Type)
	if !ok {
		return nil, ErrPacketType
	}
	return types.NewStringBuffer([]byte{typeByte}), nil
}

// DecodePacket decodes a packet. Data also available as an ArrayBuffer if requested.
func (p *parserv3) DecodePacket(data types.BufferInterface, utf8decode ...bool) (*packet.Packet, error) {
	if data == nil {
		return newErrorPacket(), ErrDataNil
	}

	utf8de := len(utf8decode) > 0 && utf8decode[0]

	msgType, err := data.ReadByte()
	if err != nil {
		return newErrorPacket(), err
	}

	switch v := data.(type) {
	case *types.StringBuffer:
		if msgType == 'b' {
			// Decodes a packet encoded in a base64 string.
			msgType, err = data.ReadByte()
			if err != nil {
				return newErrorPacket(), err
			}
			packetType, ok := lookupPacketType(msgType)
			if !ok {
				return newErrorPacket(), fmt.Errorf("%w: [%c]", ErrUnknownPacketType, msgType)
			}
			decode := types.NewBytesBuffer(nil)
			if _, err := decode.ReadFrom(base64.NewDecoder(base64.StdEncoding, v)); err != nil {
				return newErrorPacket(), err
			}
			return &packet.Packet{Type: packetType, Data: decode}, nil
		}
		packetType, ok := lookupPacketType(msgType)
		if !ok {
			return newErrorPacket(), fmt.Errorf("%w: [%c]", ErrUnknownPacketType, msgType)
		}
		if !utf8de {
			return &packet.Packet{Type: packetType, Data: v}, nil
		}
		decode := types.NewStringBuffer(nil)
		if _, err := decode.ReadFrom(utils.NewUtf8Decoder(v)); err != nil {
			return newErrorPacket(), err
		}
		raw, err := decodeV3UTF8(decode.Bytes())
		if err != nil {
			return newErrorPacket(), err
		}
		return &packet.Packet{Type: packetType, Data: types.NewStringBuffer(raw)}, nil
	}

	// Default case: binary buffer
	packetType, ok := lookupPacketType(msgType + '0')
	if !ok {
		return newErrorPacket(), fmt.Errorf("%w: [%c]", ErrUnknownPacketType, msgType+'0')
	}
	return &packet.Packet{Type: packetType, Data: data}, nil
}

func (p *parserv3) hasBinary(packets []*packet.Packet) bool {
	for _, pkt := range packets {
		if pkt == nil {
			continue
		}
		switch pkt.Data.(type) {
		case *types.StringBuffer, *strings.Reader, nil:
			// not binary
		default:
			return true
		}
	}
	return false
}

// EncodePayload encodes multiple messages (payload).
//
//	<length>:<packet>
//
// Example:
//
//	12:4hello world3:4hi
//
// In text payloads, binary packets use <length>:b<type><base64>, for example
// 6:b4AQ==. When binary support is enabled, payloads containing binary data
// use binary framing instead.
func (p *parserv3) EncodePayload(packets []*packet.Packet, supportsBinary ...bool) (types.BufferInterface, error) {
	supportsBin := len(supportsBinary) > 0 && supportsBinary[0]

	if supportsBin && p.hasBinary(packets) {
		return p.encodePayloadAsBinary(packets)
	}

	enPayload := types.NewStringBuffer(nil)

	if len(packets) == 0 {
		_, _ = enPayload.WriteString("0:")
		return enPayload, nil
	}

	for _, pkt := range packets {
		buf, err := p.EncodePacket(pkt, supportsBin, false)
		if err != nil {
			return nil, err
		}
		// <length>:<data>
		_, _ = enPayload.WriteString(strconv.FormatInt(int64(utils.Utf16Count(buf.Bytes())), 10))
		_ = enPayload.WriteByte(':')
		_, _ = enPayload.Write(buf.Bytes())
	}

	return enPayload, nil
}

// The destination is always an internal in-memory buffer.
func writeBinaryPacketHeader(dst types.BufferInterface, marker byte, length int) {
	dst.Grow(22)
	encoded := append(dst.AvailableBuffer(), marker)
	encoded = strconv.AppendInt(encoded, int64(length), 10)
	// Engine.IO v3 stores length digits as numeric bytes, not ASCII.
	for i := 1; i < len(encoded); i++ {
		encoded[i] -= '0'
	}
	encoded = append(encoded, 0xFF)
	_, _ = dst.Write(encoded)
}

// encodePayloadAsBinary encodes multiple messages (payload) as binary.
//
// <1 = binary, 0 = string><number from 0-9><number from 0-9>[...]<number
// 255><data>
//
// Example:
// 1 3 255 1 2 3, if the binary contents are interpreted as 8 bit integers
func (p *parserv3) encodePayloadAsBinary(packets []*packet.Packet) (types.BufferInterface, error) {
	enPayload := types.NewBytesBuffer(nil)

	for _, pkt := range packets {
		buf, err := p.EncodePacket(pkt, true, false)
		if err != nil {
			return nil, err
		}

		marker := byte(1)
		if _, ok := buf.(*types.StringBuffer); ok {
			marker = 0
		}
		writeBinaryPacketHeader(enPayload, marker, buf.Len())
		_, _ = enPayload.Write(buf.Bytes())
	}

	return enPayload, nil
}

// DecodePayload decodes data when a payload is maybe expected. Possible binary contents are
// decoded from their base64 representation.
func (p *parserv3) DecodePayload(data types.BufferInterface) ([]*packet.Packet, error) {
	if v, ok := data.(*types.StringBuffer); ok {
		return p.decodeStringPayload(v)
	}
	return p.decodeBinaryPayload(data)
}

func (p *parserv3) decodeStringPayload(v *types.StringBuffer) ([]*packet.Packet, error) {
	packets := make([]*packet.Packet, 0, 8)
	if v.Len() == 0 {
		return packets, ErrParser
	}

	for v.Len() > 0 {
		length, err := v.ReadString(':')
		if err != nil {
			return packets, err
		}
		l := len(length)
		if l < 2 {
			return packets, ErrInvalidDataLength
		}
		packetLen, err := strconv.Atoi(length[:l-1])
		if err != nil {
			return packets, err
		}
		// Ensure packetLen can be safely converted to int and is non-negative.
		if packetLen < 0 {
			return packets, ErrInvalidDataLength
		}
		// Read packet data (packetLen is UTF-16 length)
		msg := types.NewStringBuffer(nil)
		for i := 0; i < packetLen; {
			r, _, e := v.ReadRune()
			if e != nil {
				return packets, e
			}
			i += utils.Utf16Len(r)
			if i > packetLen {
				return packets, ErrInvalidDataLength
			}
			_, _ = msg.WriteRune(r)
		}

		if msg.Len() > 0 {
			pkt, err := p.DecodePacket(msg, false)
			if err != nil {
				return packets, err
			}
			packets = append(packets, pkt)
		}
	}
	return packets, nil
}

// decodeBinaryPayload decodes data when a payload is maybe expected. Strings are decoded by
// interpreting each byte as a key code for entries marked to start with 0. See
// description of encodePayloadAsBinary.
func (p *parserv3) decodeBinaryPayload(data types.BufferInterface) ([]*packet.Packet, error) {
	var packets []*packet.Packet
	for data.Len() > 0 {
		marker, err := data.ReadByte()
		if err != nil {
			return packets, err
		}
		if marker > 1 {
			return packets, ErrInvalidDataLength
		}
		digits, err := data.ReadBytes(0xFF)
		if err != nil {
			return packets, err
		}
		if len(digits) < 2 {
			return packets, ErrInvalidDataLength
		}
		digits = digits[:len(digits)-1]
		for i := range digits {
			if digits[i] > 9 {
				return packets, ErrInvalidDataLength
			}
			digits[i] += '0'
		}
		length, err := strconv.Atoi(string(digits))
		if err != nil || length <= 0 || length > data.Len() {
			return packets, ErrInvalidDataLength
		}
		raw := data.Next(length)
		frame := types.NewBytesBuffer(raw)
		if marker == 0 {
			raw, err = decodeV3UTF8(raw)
			if err != nil {
				return packets, err
			}
			frame = types.NewStringBuffer(raw)
		}
		pkt, err := p.DecodePacket(frame, false)
		if err != nil {
			return packets, err
		}
		packets = append(packets, pkt)
	}
	return packets, nil
}

// Node's utf8.js strict:false replaces surrogate code points, but rejects
// other malformed UTF-8. Valid text keeps its original bytes.
func decodeV3UTF8(raw []byte) ([]byte, error) {
	if utf8.Valid(raw) {
		return raw, nil
	}
	decoded := make([]byte, 0, len(raw))
	for len(raw) > 0 {
		_, size := utf8.DecodeRune(raw)
		if size == 1 && raw[0] >= utf8.RuneSelf {
			if len(raw) < 3 || raw[0] != 0xED || raw[1] < 0xA0 || raw[1] > 0xBF || raw[2] < 0x80 || raw[2] > 0xBF {
				return nil, ErrParser
			}
			decoded = utf8.AppendRune(decoded, utf8.RuneError)
			raw = raw[3:]
			continue
		}
		decoded = append(decoded, raw[:size]...)
		raw = raw[size:]
	}
	return decoded, nil
}
