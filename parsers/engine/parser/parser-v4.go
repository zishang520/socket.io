package parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type parserv4 struct{}

var (
	defaultParserv4 Parser = &parserv4{}
)

func Parserv4() Parser {
	return defaultParserv4
}

// Current protocol version.
func (*parserv4) Protocol() int {
	return Protocol
}

func (p *parserv4) EncodePacket(data *packet.Packet, supportsBinary bool, _ ...bool) (types.BufferInterface, error) {
	if data == nil {
		return nil, ErrPacketNil
	}

	if c, ok := data.Data.(io.Closer); ok {
		defer c.Close()
	}

	typeByte, ok := PACKET_TYPES[data.Type]
	if !ok {
		return nil, ErrPacketType
	}

	switch v := data.Data.(type) {
	case *types.StringBuffer, *strings.Reader:
		encode := types.NewStringBuffer(nil)
		if err := encode.WriteByte(typeByte); err != nil {
			return nil, err
		}
		if _, err := io.Copy(encode, v); err != nil {
			return nil, err
		}
		return encode, nil

	case io.Reader:
		if !supportsBinary {
			encode := types.NewStringBuffer(nil)
			if err := encode.WriteByte('b'); err != nil {
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
		if _, err := io.Copy(encode, v); err != nil {
			return nil, err
		}
		return encode, nil
	}

	encode := types.NewStringBuffer(nil)
	if err := encode.WriteByte(typeByte); err != nil {
		return nil, err
	}
	return encode, nil
}

func (p *parserv4) DecodePacket(data types.BufferInterface, _ ...bool) (*packet.Packet, error) {
	if data == nil {
		return ERROR_PACKET, ErrDataNil
	}

	switch v := data.(type) {
	case *types.StringBuffer:
		msgType, err := v.ReadByte()
		if err != nil {
			return ERROR_PACKET, err
		}

		if msgType == 'b' {
			// base64 string -> binary message
			decode := types.NewBytesBuffer(nil)
			if _, err := decode.ReadFrom(base64.NewDecoder(base64.StdEncoding, v)); err != nil {
				return ERROR_PACKET, err
			}
			return &packet.Packet{Type: packet.MESSAGE, Data: decode}, nil
		}

		packetType, ok := PACKET_TYPES_REVERSE[msgType]
		if !ok {
			return ERROR_PACKET, fmt.Errorf(`%w, unknown data type [%c]`, ErrParser, msgType)
		}

		stringBuffer := types.NewStringBuffer(nil)
		if _, err := stringBuffer.ReadFrom(v); err != nil {
			return ERROR_PACKET, err
		}
		return &packet.Packet{Type: packetType, Data: stringBuffer}, nil
	}

	// Binary packet
	decode := types.NewBytesBuffer(nil)
	if _, err := io.Copy(decode, data); err != nil {
		return ERROR_PACKET, err
	}
	return &packet.Packet{Type: packet.MESSAGE, Data: decode}, nil
}

func (p *parserv4) EncodePayload(packets []*packet.Packet, _ ...bool) (types.BufferInterface, error) {
	enPayload := types.NewStringBuffer(nil)

	if len(packets) == 0 {
		return enPayload, nil
	}

	for i, packet := range packets {
		buf, err := p.EncodePacket(packet, false)
		if err != nil {
			return nil, err
		}

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

func (p *parserv4) DecodePayload(data types.BufferInterface) (packets []*packet.Packet, _ error) {
	buf := data.Bytes()
	if len(buf) == 0 {
		return make([]*packet.Packet, 0), nil
	}

	packets = make([]*packet.Packet, 0, 8)

	for len(buf) > 0 {
		var payload []byte

		idx := bytes.IndexByte(buf, SEPARATOR)
		if idx >= 0 {
			payload = buf[:idx]
			buf = buf[idx+1:]
		} else {
			payload = buf
			buf = nil
		}

		if len(payload) == 0 {
			continue
		}

		packet, err := p.DecodePacket(types.NewStringBuffer(payload))
		if err != nil {
			return packets, err
		}
		packets = append(packets, packet)
	}

	return packets, nil
}
