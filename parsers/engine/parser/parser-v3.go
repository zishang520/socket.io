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
		defer c.Close()
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
		if err := encode.WriteByte(typeByte); err != nil {
			return nil, err
		}
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
			if _, err := encode.Write([]byte{'b', typeByte}); err != nil {
				return nil, err
			}
			b64 := base64.NewEncoder(base64.StdEncoding, encode)
			if _, err := io.Copy(b64, v); err != nil {
				b64.Close()
				return nil, err
			}
			if err := b64.Close(); err != nil {
				return nil, err
			}
			return encode, nil
		}
		encode := types.NewBytesBuffer(nil)
		if err := encode.WriteByte(typeByte - '0'); err != nil {
			return nil, err
		}
		if _, err := io.Copy(encode, v); err != nil {
			return nil, err
		}
		return encode, nil
	}

	// default nil data
	encode := types.NewStringBuffer(nil)
	typeByte, ok := lookupPacketByte(data.Type)
	if !ok {
		return nil, ErrPacketType
	}
	if err := encode.WriteByte(typeByte); err != nil {
		return nil, err
	}
	return encode, nil
}

// DecodePacket decodes a packet. Data also available as an ArrayBuffer if requested.
func (p *parserv3) DecodePacket(data types.BufferInterface, utf8decode ...bool) (*packet.Packet, error) {
	if data == nil {
		return ERROR_PACKET, ErrDataNil
	}

	utf8de := len(utf8decode) > 0 && utf8decode[0]

	msgType, err := data.ReadByte()
	if err != nil {
		return ERROR_PACKET, err
	}

	switch v := data.(type) {
	case *types.StringBuffer:
		if msgType == 'b' {
			// Decodes a packet encoded in a base64 string.
			msgType, err = data.ReadByte()
			if err != nil {
				return ERROR_PACKET, err
			}
			packetType, ok := lookupPacketType(msgType)
			if !ok {
				return ERROR_PACKET, fmt.Errorf("%w: [%c]", ErrUnknownPacketType, msgType)
			}
			decode := types.NewBytesBuffer(nil)
			if _, err := decode.ReadFrom(base64.NewDecoder(base64.StdEncoding, v)); err != nil {
				return ERROR_PACKET, err
			}
			return &packet.Packet{Type: packetType, Data: decode}, nil
		}
		packetType, ok := lookupPacketType(msgType)
		if !ok {
			return ERROR_PACKET, fmt.Errorf("%w: [%c]", ErrUnknownPacketType, msgType)
		}
		decode := types.NewStringBuffer(nil)
		if utf8de {
			if _, err := decode.ReadFrom(utils.NewUtf8Decoder(v)); err != nil {
				return ERROR_PACKET, err
			}
		} else {
			if _, err := decode.ReadFrom(v); err != nil {
				return ERROR_PACKET, err
			}
		}
		return &packet.Packet{Type: packetType, Data: decode}, nil
	}

	// Default case: binary buffer
	packetType, ok := lookupPacketType(msgType + '0')
	if !ok {
		return ERROR_PACKET, fmt.Errorf("%w: [%c]", ErrUnknownPacketType, msgType+'0')
	}
	decode := types.NewBytesBuffer(nil)
	if _, err := io.Copy(decode, data); err != nil {
		return ERROR_PACKET, err
	}
	return &packet.Packet{Type: packetType, Data: decode}, nil
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
//	<length>:data
//
// Example:
//
//	11:hello world2:hi
//
// If any contents are binary, they will be encoded as base64 strings. Base64
// encoded strings are marked with a b before the length specifier
func (p *parserv3) EncodePayload(packets []*packet.Packet, supportsBinary ...bool) (types.BufferInterface, error) {
	supportsBin := len(supportsBinary) > 0 && supportsBinary[0]

	if supportsBin && p.hasBinary(packets) {
		return p.encodePayloadAsBinary(packets)
	}

	enPayload := types.NewStringBuffer(nil)

	if len(packets) == 0 {
		if _, err := enPayload.WriteString("0:"); err != nil {
			return nil, err
		}
		return enPayload, nil
	}

	for _, pkt := range packets {
		buf, err := p.EncodePacket(pkt, supportsBin, false)
		if err != nil {
			return nil, err
		}
		// <length>:<data>
		if _, err := enPayload.WriteString(strconv.FormatInt(int64(utils.Utf16Count(buf.Bytes())), 10)); err != nil {
			return nil, err
		}
		if err := enPayload.WriteByte(':'); err != nil {
			return nil, err
		}
		if _, err := enPayload.Write(buf.Bytes()); err != nil {
			return nil, err
		}
	}

	return enPayload, nil
}

func (p *parserv3) encodeOneBinaryPacket(pkt *packet.Packet) (types.BufferInterface, error) {
	if pkt == nil {
		return nil, ErrPacketNil
	}

	buf, err := p.EncodePacket(pkt, true, true)
	if err != nil {
		return nil, err
	}

	binaryPacket := types.NewBytesBuffer(nil)

	if _, ok := buf.(*types.StringBuffer); ok {
		encodingLength := strconv.FormatInt(int64(utils.Utf16Count(buf.Bytes())), 10) // JS length
		if err := binaryPacket.WriteByte(0x00); err != nil {
			return nil, err
		}
		for i := 0; i < len(encodingLength); i++ {
			if err := binaryPacket.WriteByte(encodingLength[i] - '0'); err != nil {
				return nil, err
			}
		}
		if err := binaryPacket.WriteByte(0xFF); err != nil {
			return nil, err
		}
		if _, err := buf.WriteTo(utils.NewUtf8Encoder(binaryPacket)); err != nil {
			return nil, err
		}
		return binaryPacket, nil
	}

	// is binary (true binary = 1)
	encodingLength := strconv.FormatInt(int64(buf.Len()), 10)
	if err := binaryPacket.WriteByte(0x01); err != nil {
		return nil, err
	}
	for i := 0; i < len(encodingLength); i++ {
		if err := binaryPacket.WriteByte(encodingLength[i] - '0'); err != nil {
			return nil, err
		}
	}
	if err := binaryPacket.WriteByte(0xFF); err != nil {
		return nil, err
	}
	if _, err := binaryPacket.ReadFrom(buf); err != nil {
		return nil, err
	}
	return binaryPacket, nil
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

	if len(packets) == 0 {
		return enPayload, nil
	}

	for _, pkt := range packets {
		buf, err := p.encodeOneBinaryPacket(pkt)
		if err != nil {
			return nil, err
		}
		if _, err := enPayload.ReadFrom(buf); err != nil {
			return nil, err
		}
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

	for v.Len() > 0 {
		length, err := v.ReadString(':')
		if err != nil {
			return packets, err
		}
		l := len(length)
		if l < 2 {
			return packets, ErrInvalidDataLength
		}
		packetLen, err := strconv.ParseInt(length[:l-1], 10, 64)
		if err != nil {
			return packets, err
		}

		// Read packet data (packetLen is UTF-16 length)
		msg := types.NewStringBuffer(nil)
		for i := int64(0); i < packetLen; {
			r, _, e := v.ReadRune()
			if e != nil {
				return packets, e
			}
			i += int64(utils.Utf16Len(r))
			if _, err := msg.WriteRune(r); err != nil {
				return packets, err
			}
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
func (p *parserv3) decodeBinaryPayload(bufferTail types.BufferInterface) ([]*packet.Packet, error) {
	packets := make([]*packet.Packet, 0, 8)

	for bufferTail.Len() > 0 {
		startByte, err := bufferTail.ReadByte()
		if err != nil {
			return packets, err
		}
		isString := startByte == 0x00

		// Read length bytes until 0xFF
		lengthBytes, err := bufferTail.ReadBytes(0xFF)
		if err != nil {
			return packets, err
		}
		l := len(lengthBytes)
		if l < 1 {
			return packets, ErrInvalidDataLength
		}
		// Convert raw digits to ASCII digits
		lenByte := lengthBytes[:l-1]
		for k := 0; k < len(lenByte); k++ {
			lenByte[k] += '0'
		}
		packetLen, err := strconv.ParseInt(string(lenByte), 10, 64)
		if err != nil {
			return packets, err
		}

		if isString {
			data := types.NewStringBuffer(nil)
			runeBuf := make([]byte, 0, 4)

			for k := int64(0); k < packetLen; {
				runeBuf = runeBuf[:0]
				// read utf8 rune bytes
				for len(runeBuf) < 4 {
					r, _, err := bufferTail.ReadRune()
					if err != nil {
						if err == io.EOF && len(runeBuf) > 0 {
							break
						}
						return packets, err
					}
					runeBuf = append(runeBuf, byte(r))
					if utf8.FullRune(runeBuf) {
						break
					}
				}
				r, runeLen := utf8.DecodeRune(runeBuf)
				k += int64(utils.Utf16Len(r))
				if _, err := data.Write(utils.Utf8decodeBytes(runeBuf[:runeLen])); err != nil {
					return packets, err
				}
			}

			if data.Len() > 0 {
				pkt, err := p.DecodePacket(data, false)
				if err != nil {
					return packets, err
				}
				packets = append(packets, pkt)
			}
		} else {
			if rawData := bufferTail.Next(int(packetLen)); len(rawData) > 0 {
				pkt, err := p.DecodePacket(types.NewBytesBuffer(rawData), false)
				if err != nil {
					return packets, err
				}
				packets = append(packets, pkt)
			}
		}
	}
	return packets, nil
}
