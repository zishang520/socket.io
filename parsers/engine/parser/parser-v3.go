package parser

import (
	"encoding/base64"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type parserv3 struct{}

var (
	defaultParserv3 Parser = &parserv3{}

	bytePool = sync.Pool{
		New: func() any {
			b := make([]byte, 0, 32)
			return &b
		},
	}
)

func Parserv3() Parser {
	return defaultParserv3
}

func (*parserv3) Protocol() int {
	return 3
}

func (p *parserv3) EncodePacket(data *packet.Packet, supportsBinary bool, utf8encode ...bool) (types.BufferInterface, error) {
	if data == nil {
		return nil, ErrPacketNil
	}

	if c, ok := data.Data.(io.Closer); ok {
		defer c.Close()
	}

	packetTypeByte, ok := PACKET_TYPES[data.Type]
	if !ok {
		return nil, ErrPacketType
	}

	utf8en := len(utf8encode) > 0 && utf8encode[0]

	switch v := data.Data.(type) {
	case *types.StringBuffer, *strings.Reader:
		encode := types.NewStringBuffer(nil)
		if err := encode.WriteByte(packetTypeByte); err != nil {
			return nil, err
		}
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
		if !supportsBinary {
			// Base64 encoding for binary data
			encode := types.NewStringBuffer(nil)
			// Optimize: Write bytes directly instead of allocating a slice
			if err := encode.WriteByte('b'); err != nil {
				return nil, err
			}
			if err := encode.WriteByte(packetTypeByte); err != nil {
				return nil, err
			}

			b64 := base64.NewEncoder(base64.StdEncoding, encode)
			// Explicit close check
			if _, err := io.Copy(b64, v); err != nil {
				b64.Close()
				return nil, err
			}
			if err := b64.Close(); err != nil {
				return nil, err
			}
			return encode, nil
		}

		// Binary support
		encode := types.NewBytesBuffer(nil)
		if err := encode.WriteByte(packetTypeByte - '0'); err != nil {
			return nil, err
		}
		if _, err := io.Copy(encode, v); err != nil {
			return nil, err
		}
		return encode, nil
	}

	// Default (Packet with no data)
	encode := types.NewStringBuffer(nil)
	if err := encode.WriteByte(packetTypeByte); err != nil {
		return nil, err
	}
	return encode, nil
}

func (p *parserv3) DecodePacket(data types.BufferInterface, utf8decode ...bool) (*packet.Packet, error) {
	if data == nil {
		return ERROR_PACKET, ErrDataNil
	}

	utf8de := len(utf8decode) > 0 && utf8decode[0]

	msgType, err := data.ReadByte()
	if err != nil {
		return ERROR_PACKET, err
	}

	// Optimization: Handle StringBuffer (Text)
	if sb, ok := data.(*types.StringBuffer); ok {
		if msgType == 'b' {
			// Base64
			msgType, err = data.ReadByte()
			if err != nil {
				return ERROR_PACKET, err
			}
			packetType, ok := PACKET_TYPES_REVERSE[msgType]
			if !ok {
				return ERROR_PACKET, fmt.Errorf(`%w, unknown data type [%c]`, ErrParser, msgType)
			}
			decode := types.NewBytesBuffer(nil)
			if _, err := decode.ReadFrom(base64.NewDecoder(base64.StdEncoding, sb)); err != nil {
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
			if _, err := decode.ReadFrom(utils.NewUtf8Decoder(sb)); err != nil {
				return ERROR_PACKET, err
			}
		} else {
			if _, err := decode.ReadFrom(sb); err != nil {
				return ERROR_PACKET, err
			}
		}
		return &packet.Packet{Type: packetType, Data: decode}, nil
	}

	// Binary Buffer
	// msgType in binary is raw int (0,1..), convert to ASCII char for map lookup
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
	for _, pkt := range packets {
		if pkt == nil {
			continue
		}
		switch pkt.Data.(type) {
		case *types.StringBuffer, *strings.Reader, nil:
			continue
		default:
			return true
		}
	}
	return false
}

func (p *parserv3) EncodePayload(packets []*packet.Packet, supportsBinary ...bool) (types.BufferInterface, error) {
	supportsBin := len(supportsBinary) > 0 && supportsBinary[0]

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

	// Zero-alloc Integer conversion buffer
	ptr := bytePool.Get().(*[]byte)
	defer bytePool.Put(ptr)

	for _, pkt := range packets {
		buf, err := p.EncodePacket(pkt, supportsBin, false)
		if err != nil {
			return nil, err
		}

		// Calculate UTF16 length
		lenVal := int64(utils.Utf16Count(buf.Bytes()))

		// Reset buffer and append int
		*ptr = (*ptr)[:0]
		*ptr = strconv.AppendInt(*ptr, lenVal, 10)

		// Direct Write: <length>:<data>
		if _, err := enPayload.Write(*ptr); err != nil {
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

func (p *parserv3) encodeOneBinaryPacket(packet *packet.Packet) (types.BufferInterface, error) {
	if packet == nil {
		return nil, ErrPacketNil
	}

	buf, err := p.EncodePacket(packet, true, true)
	if err != nil {
		return nil, err
	}

	binarypacket := types.NewBytesBuffer(nil)
	ptr := bytePool.Get().(*[]byte)
	defer bytePool.Put(ptr)
	*ptr = (*ptr)[:0]

	if sb, ok := buf.(*types.StringBuffer); ok {
		// String packet in binary: <0><len-utf16><FF><utf8-data>
		// len-utf16 digits are written as raw integers 0-9
		if err := binarypacket.WriteByte(0); err != nil {
			return nil, err
		}

		lenVal := int64(utils.Utf16Count(sb.Bytes()))
		*ptr = strconv.AppendInt(*ptr, lenVal, 10)

		// Convert ASCII digits to raw values (e.g., '1' -> 1)
		digitBuf := *ptr
		for _, b := range digitBuf {
			if err := binarypacket.WriteByte(b - '0'); err != nil {
				return nil, err
			}
		}

		if err := binarypacket.WriteByte(0xFF); err != nil {
			return nil, err
		}
		if _, err := sb.WriteTo(utils.NewUtf8Encoder(binarypacket)); err != nil {
			return nil, err
		}
		return binarypacket, nil
	}

	// Binary packet: <1><len-bytes><FF><data>
	if err := binarypacket.WriteByte(1); err != nil {
		return nil, err
	}

	lenVal := int64(buf.Len())
	*ptr = strconv.AppendInt(*ptr, lenVal, 10)

	digitBuf := *ptr
	for _, b := range digitBuf {
		if err := binarypacket.WriteByte(b - '0'); err != nil {
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

func (p *parserv3) DecodePayload(data types.BufferInterface) (packets []*packet.Packet, _ error) {
	// Fallback to binary decoding if not StringBuffer
	v, ok := data.(*types.StringBuffer)
	if !ok {
		return p.decodePayloadAsBinary(data)
	}

	packets = make([]*packet.Packet, 0, 8)

	for v.Len() > 0 {
		// Read length until ':'
		// We implement a simplified manual parse to avoid ReadString allocs if needed,
		// but since StringBuffer usually optimized ReadString, we stick to logic but optimize parsing.

		// Optimization: Peek or Read bytes to parse int without string alloc
		// But types.StringBuffer interface might be limited.
		// Using ReadString is safe but generates garbage.
		// For high perf, if StringBuffer exposes Bytes(), we should use that.
		// Assuming standard Interface usage:

		lengthStr, err := v.ReadString(':')
		if err != nil {
			return packets, err
		}
		l := len(lengthStr)
		if l < 2 { // At least "0:"
			return packets, ErrInvalidDataLength
		}

		// Avoid ParseInt by manual calculation (optional, but ParseInt is fast enough for small strings)
		// For zero-alloc strictness, we would avoid ReadString.
		// Keeping ParseInt here for safety, can be optimized if we access raw bytes.
		packetLen, err := strconv.ParseInt(lengthStr[:l-1], 10, 64)
		if err != nil {
			return packets, err
		}

		if packetLen == 0 {
			continue
		}

		msg := types.NewStringBuffer(nil)

		// PacketLen is UTF-16 length. We must read corresponding UTF-8 runes.
		currentLen := 0
		targetLen := int(packetLen)

		// This loop is the bottleneck for large packets.
		for currentLen < targetLen {
			r, _, err := v.ReadRune()
			if err != nil {
				return packets, err
			}
			// Use the optimized Utf16Len
			currentLen += utils.Utf16Len(r)

			// WriteRune handles UTF-8 encoding
			if _, err := msg.WriteRune(r); err != nil {
				return packets, err
			}
		}

		if msg.Len() > 0 {
			packet, err := p.DecodePacket(msg, false)
			if err != nil {
				return packets, err
			}
			packets = append(packets, packet)
		}
	}
	return packets, nil
}

func (p *parserv3) decodePayloadAsBinary(bufferTail types.BufferInterface) (packets []*packet.Packet, _ error) {
	packets = make([]*packet.Packet, 0, 8)

	for bufferTail.Len() > 0 {
		startByte, err := bufferTail.ReadByte()
		if err != nil {
			return packets, err
		}
		isString := startByte == 0x00

		// Optimization: Manually parse length (raw 0-9 bytes) until 0xFF
		// Avoids ReadBytes(0xFF) allocation
		var packetLen int64
		for {
			b, err := bufferTail.ReadByte()
			if err != nil {
				return packets, err
			}
			if b == 0xFF {
				break
			}
			if b > 9 {
				return packets, ErrInvalidDataLength
			}
			packetLen = packetLen*10 + int64(b)
		}

		if isString {
			data := types.NewStringBuffer(nil)

			// String packet in binary: data is UTF-8, but packetLen is UTF-16 length.
			// We must reconstruct the string carefully.

			// Use a small stack buffer for rune decoding
			buf := make([]byte, 0, 4)

			currentLen := 0
			targetLen := int(packetLen)

			for currentLen < targetLen {
				buf = buf[:0]
				// Read a full UTF-8 rune
				for len(buf) < 4 {
					r, _, err := bufferTail.ReadRune()
					if err != nil {
						if err == io.EOF && len(buf) > 0 {
							break
						}
						return packets, err
					}
					buf = append(buf, byte(r))
					if utf8.FullRune(buf) {
						break
					}
				}

				r, l := utf8.DecodeRune(buf)
				if r == utf8.RuneError && l == 1 {
					// Handle error if strictness required
				}

				currentLen += utils.Utf16Len(r)

				// Write decoded/transcoded data
				if _, err := data.Write(utils.Utf8decodeBytes(buf)); err != nil {
					return packets, err
				}
			}

			// Note: The Seek logic in original code was suspicious or handled over-read.
			// Since we read exactly rune-by-rune, we shouldn't need seek.

			if data.Len() > 0 {
				packet, err := p.DecodePacket(data, false)
				if err != nil {
					return packets, err
				}
				packets = append(packets, packet)
			}
		} else {
			// Binary packet: packetLen is byte length
			data := bufferTail.Next(int(packetLen))
			if len(data) > 0 {
				packet, err := p.DecodePacket(types.NewBytesBuffer(data), false)
				if err != nil {
					return packets, err
				}
				packets = append(packets, packet)
			}
		}
	}
	return packets, nil
}
