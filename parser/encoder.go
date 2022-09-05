package parser

import (
	"encoding/json"
	"github.com/zishang520/engine.io/types"
	"strconv"
	"strings"
)

// A socket.io Encoder instance
type encoder struct {
}

func NewEncoder() Encoder {
	return &encoder{}
}

// Encode a packet as a single string if non-binary, or as a
// buffer sequence, depending on packet type.
func (e *encoder) Encode(packet *Packet) []types.BufferInterface {
	parser_log.Debug("encoding packet %v", packet)
	if packet.Type == EVENT || packet.Type == ACK {
		if HasBinary(packet.Data) {
			if packet.Type == EVENT {
				packet.Type = BINARY_EVENT
			} else {
				packet.Type = BINARY_ACK
			}
			return e.encodeAsBinary(packet)
		}
	}
	return []types.BufferInterface{e.encodeAsString(packet)}
}

func _encodeData(data any) any {
	if data == nil {
		return nil
	}

	// *strings.Reader special handling
	if tdata, ok := data.(*strings.Reader); ok {
		rdata, _ := types.NewStringBufferReader(tdata)
		return rdata
	}

	switch tdata := data.(type) {
	case []any:
		newData := make([]any, 0, len(tdata))
		for _, v := range tdata {
			newData = append(newData, _encodeData(v))
		}
		return newData
	case map[string]any:
		newData := map[string]any{}
		for k, v := range tdata {
			newData[k] = _encodeData(v)
		}
		return newData
	}

	return data
}

// Encode packet as string.
func (e *encoder) encodeAsString(packet *Packet) types.BufferInterface {
	// first is type
	str := types.NewStringBuffer([]byte{byte(packet.Type)})
	// attachments if we have them
	if packet.Type == BINARY_EVENT || packet.Type == BINARY_ACK {
		str.WriteString(strconv.FormatUint(packet.Attachments, 10))
		str.WriteByte('-')
	}
	// if we have a namespace other than `/`
	// we append it followed by a comma `,`
	if len(packet.Nsp) > 0 && "/" != packet.Nsp {
		str.WriteString(packet.Nsp)
		str.WriteByte(',')
	}
	// immediately followed by the id
	if 0 < packet.Id {
		str.WriteString(strconv.FormatUint(packet.Id, 10))
	}
	// json data
	if nil != packet.Data {
		if b, err := json.Marshal(_encodeData(packet.Data)); err == nil {
			str.Write(b)
		}
	}
	parser_log.Debug("encoded %v as %v", packet, str)
	return str
}

// Encode packet as 'buffer sequence' by removing blobs, and
// deconstructing packet into object with placeholders and
// a list of buffers.
func (e *encoder) encodeAsBinary(obj *Packet) []types.BufferInterface {
	packet, buffers := DeconstructPacket(obj)
	pack := e.encodeAsString(packet)
	buffers = append([]types.BufferInterface{pack}, buffers...) // add packet info to beginning of data list
	return buffers                                              // write all the buffers
}
