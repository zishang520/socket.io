package parser

import (
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/types"
	"github.com/zishang520/socket.io/parsers/engine/v3/utils"
)

type parserv3 struct{}

var (
	defaultParserv3 Parser = &parserv3{}
)

func Parserv3() Parser {
	return defaultParserv3
}

// Current protocol version.
func (*parserv3) Protocol() int {
	return 3
}

func (p *parserv3) EncodePacket(data *packet.Packet, supportsBinary bool, utf8encode ...bool) (types.BufferInterface, error) {
	if data == nil {
		return nil, ErrPacketNil
	}

	utf8en := false
	if len(utf8encode) > 0 {
		utf8en = utf8encode[0]
	}

	if c, ok := data.Data.(io.Closer); ok {
		defer c.Close()
	}

	switch v := data.Data.(type) {
	case *types.StringBuffer, *strings.Reader:
		encode := types.NewStringBuffer(nil)
		_type, _type_ok := PACKET_TYPES[data.Type]
		if !_type_ok {
			return nil, ErrPacketType
		}
		// Sending data as a utf-8 string
		if err := encode.WriteByte(_type); err != nil {
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
		// Encode Buffer data
		if !supportsBinary {
			// Encodes a packet with binary data in a base64 string
			encode := types.NewStringBuffer(nil)
			_type, _type_ok := PACKET_TYPES[data.Type]
			if !_type_ok {
				return nil, ErrPacketType
			}
			if _, err := encode.Write([]byte{'b', _type}); err != nil {
				return nil, err
			}
			b64 := base64.NewEncoder(base64.StdEncoding, encode)
			defer b64.Close()

			if _, err := io.Copy(b64, v); err != nil {
				return nil, err
			}
			return encode, nil
		}
		encode := types.NewBytesBuffer(nil)
		_type, _type_ok := PACKET_TYPES[data.Type]
		if !_type_ok {
			return nil, ErrPacketType
		}
		if err := encode.WriteByte(_type - '0'); err != nil {
			return nil, err
		}
		if _, err := io.Copy(encode, v); err != nil {
			return nil, err
		}
		return encode, nil
	}
	// default nil
	encode := types.NewStringBuffer(nil)
	_type, _type_ok := PACKET_TYPES[data.Type]
	if !_type_ok {
		return nil, ErrPacketType
	}
	if err := encode.WriteByte(_type); err != nil {
		return nil, err
	}
	return encode, nil
}

// Decodes a packet. Data also available as an ArrayBuffer if requested.
func (p *parserv3) DecodePacket(data types.BufferInterface, utf8decode ...bool) (*packet.Packet, error) {
	if data == nil {
		return ERROR_PACKET, ErrDataNil
	}

	utf8de := false
	if len(utf8decode) > 0 {
		utf8de = utf8decode[0]
	}

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
			packetType, ok := PACKET_TYPES_REVERSE[msgType]
			if !ok {
				return ERROR_PACKET, fmt.Errorf(`%w, unknown data type [%c]`, ErrParser, msgType)
			}
			decode := types.NewBytesBuffer(nil)
			if _, err := decode.ReadFrom(base64.NewDecoder(base64.StdEncoding, v)); err != nil {
				return ERROR_PACKET, err
			}
			return &packet.Packet{Type: packetType, Data: decode}, nil
		}
		packetType, ok := PACKET_TYPES_REVERSE[msgType]
		if !ok {
			return ERROR_PACKET, fmt.Errorf(`%w, unknown data type [%c]`, ErrParser, msgType)
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

	// Default case
	packetType, ok := PACKET_TYPES_REVERSE[msgType+'0']
	if !ok {
		return ERROR_PACKET, fmt.Errorf(`%w, unknown data type [%c]`, ErrParser, msgType+'0')
	}
	decode := types.NewBytesBuffer(nil)
	if _, err := io.Copy(decode, data); err != nil {
		return ERROR_PACKET, err
	}
	return &packet.Packet{Type: packetType, Data: decode}, nil
}

func (p *parserv3) hasBinary(packets []*packet.Packet) bool {
	if len(packets) == 0 {
		return false
	}
	for _, packet := range packets {
		if packet != nil {
			switch packet.Data.(type) {
			case *types.StringBuffer:
			case *strings.Reader:
			case nil:
			default:
				return true
			}
		}
	}

	return false
}

// Encodes multiple messages (payload).
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
	supportsBin := false
	if len(supportsBinary) > 0 {
		supportsBin = supportsBinary[0]
	}

	if supportsBin && p.hasBinary(packets) {
		return p.encodePayloadAsBinary(packets)
	}

	enPayload := types.NewStringBuffer(nil)

	if len(packets) == 0 {
		if _, err := enPayload.WriteString(`0:`); err != nil {
			return nil, err
		}
		return enPayload, nil
	}

	for _, packet := range packets {
		buf, err := p.EncodePacket(packet, supportsBin, false)
		if err != nil {
			return nil, err
		}
		if _, err := enPayload.WriteString(strconv.FormatInt(int64(utils.Utf16Count(buf.Bytes())), 10) + ":" + buf.String()); err != nil {
			return nil, err
		}
	}

	return enPayload, nil
}

func (p *parserv3) encodeOneBinaryPacket(packet *packet.Packet) (types.BufferInterface, error) {
	if packet == nil {
		return nil, ErrPacketNil
	}
	binarypacket := types.NewBytesBuffer(nil)

	buf, err := p.EncodePacket(packet, true, true)
	if err != nil {
		return nil, err
	}

	if _, ok := buf.(*types.StringBuffer); ok {
		encodingLength := strconv.FormatInt(int64(utils.Utf16Count(buf.Bytes())), 10) // JS length
		if err := binarypacket.WriteByte(0); err != nil {
			return nil, err
		}
		for i, l := 0, len(encodingLength); i < l; i++ {
			if err := binarypacket.WriteByte(encodingLength[i] - '0'); err != nil {
				return nil, err
			}
		}
		if err := binarypacket.WriteByte(0xFF); err != nil {
			return nil, err
		}
		if _, err := buf.WriteTo(utils.NewUtf8Encoder(binarypacket)); err != nil {
			return nil, err
		}

		return binarypacket, nil
	}

	encodingLength := strconv.FormatInt(int64(buf.Len()), 10)
	// is binary (true binary = 1)
	if err := binarypacket.WriteByte(1); err != nil {
		return nil, err
	}
	for i, l := 0, len(encodingLength); i < l; i++ {
		if err := binarypacket.WriteByte(encodingLength[i] - '0'); err != nil {
			return nil, err
		}
	}
	if err := binarypacket.WriteByte(0xFF); err != nil {
		return nil, err
	}
	if _, err := binarypacket.ReadFrom(buf); err != nil {
		return nil, err
	}

	return binarypacket, nil
}

// Encodes multiple messages (payload) as binary.
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

	for _, packet := range packets {
		buf, err := p.encodeOneBinaryPacket(packet)
		if err != nil {
			return nil, err
		}
		if _, err := enPayload.ReadFrom(buf); err != nil {
			return nil, err
		}
	}

	return enPayload, nil
}

// Decodes data when a payload is maybe expected. Possible binary contents are
// decoded from their base64 representation
func (p *parserv3) DecodePayload(data types.BufferInterface) (packets []*packet.Packet, _ error) {
	switch v := data.(type) {
	case *types.StringBuffer:
		PACKETLEN := 0
		for v.Len() > 0 {
			length, err := v.ReadString(':')
			if err != nil {
				return packets, err
			}
			_l := len(length)
			if _l < 1 {
				return packets, ErrInvalidDataLength
			}
			packetLen, err := strconv.ParseInt(length[:_l-1], 10, 0)
			if err != nil {
				return packets, err
			}

			PACKETLEN = int(packetLen)
			msg := types.NewStringBuffer(nil)
			for i := 0; i < PACKETLEN; {
				r, _, e := v.ReadRune()
				if e != nil {
					return packets, e
				}
				i += utils.Utf16Len(r)
				if _, err := msg.WriteRune(r); err != nil {
					return packets, err
				}
			}

			if msg.Len() > 0 {
				if packet, err := p.DecodePacket(msg, false); err == nil {
					packets = append(packets, packet)
				} else {
					return packets, nil
				}
			}
		}
		return packets, nil
	}
	return p.decodePayloadAsBinary(data)
}

// Decodes data when a payload is maybe expected. Strings are decoded by
// interpreting each byte as a key code for entries marked to start with 0. See
// description of encodePayloadAsBinary
func (p *parserv3) decodePayloadAsBinary(bufferTail types.BufferInterface) (packets []*packet.Packet, _ error) {
	PACKETLEN := 0
	for bufferTail.Len() > 0 {
		startByte, err := bufferTail.ReadByte()
		if err != nil {
			// parser error in individual packet - ignoring payload
			return packets, err
		}
		isString := startByte == 0x00
		length, err := bufferTail.ReadBytes(0xFF)
		if err != nil {
			return packets, err
		}
		_l := len(length)
		if _l < 1 {
			return packets, ErrInvalidDataLength
		}
		lenByte := length[:_l-1]
		for k, l := 0, len(lenByte); k < l; k++ {
			lenByte[k] = lenByte[k] + '0'
		}
		packetLen, err := strconv.ParseInt(string(lenByte), 10, 0)
		if err != nil {
			return packets, err
		}
		PACKETLEN = int(packetLen)
		if isString {
			data := types.NewStringBuffer(nil)
			buf := make([]byte, 0, 4) // rune bytes
			for k := 0; k < PACKETLEN; {
				// read utf8
				for len(buf) < 4 {
					r, _, err := bufferTail.ReadRune()
					if err != nil {
						if err == io.EOF {
							break
						}
						return packets, err
					}
					if !utf8.ValidRune(r) {
						r = 0xFFFD
					}
					buf = append(buf, byte(r))
				}
				r, l := utf8.DecodeRune(buf)
				k += utils.Utf16Len(r)
				if _, err := data.Write(utils.Utf8decodeBytes(buf[0:l])); err != nil {
					return packets, err
				}
				buf = buf[l:]
			}
			if cursor := len(utils.Utf8encodeBytes(buf)); cursor > 0 {
				bufferTail.Seek(-int64(cursor), io.SeekCurrent)
			}
			if data.Len() > 0 {
				if packet, err := p.DecodePacket(data, false); err == nil {
					packets = append(packets, packet)
				} else {
					return packets, err
				}
			}
		} else {
			if data := bufferTail.Next(PACKETLEN); len(data) > 0 {
				if packet, err := p.DecodePacket(types.NewBytesBuffer(data), false); err == nil {
					packets = append(packets, packet)
				} else {
					return packets, err
				}
			}
		}
	}
	return packets, nil
}
