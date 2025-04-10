package parser

import (
	"errors"
	"fmt"
	"io"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

type Placeholder struct {
	Placeholder bool  `json:"_placeholder" msgpack:"_placeholder"`
	Num         int64 `json:"num" msgpack:"num"`
}

// Replaces every io.Reader | []byte in packet with a numbered placeholder.
func DeconstructPacket(packet *Packet) (pack *Packet, buffers []types.BufferInterface) {
	pack = packet
	pack.Data = _deconstructPacket(packet.Data, &buffers)
	attachments := uint64(len(buffers))
	pack.Attachments = &attachments // number of binary 'attachments'
	return pack, buffers
}

func _deconstructPacket(data any, buffers *[]types.BufferInterface) any {
	if data == nil {
		return nil
	}

	if IsBinary(data) {
		_placeholder := &Placeholder{Placeholder: true, Num: int64(len(*buffers))}
		rdata := types.NewBytesBuffer(nil)
		switch tdata := data.(type) {
		case io.Reader:
			if c, ok := data.(io.Closer); ok {
				defer c.Close()
			}
			rdata.ReadFrom(tdata)
		case []byte:
			rdata.Write(tdata)
		}
		*buffers = append(*buffers, rdata)
		return _placeholder
	}

	switch tdata := data.(type) {
	case []any:
		newData := make([]any, 0, len(tdata))
		for _, v := range tdata {
			newData = append(newData, _deconstructPacket(v, buffers))
		}
		return newData
	case map[string]any:
		newData := map[string]any{}
		for k, v := range tdata {
			newData[k] = _deconstructPacket(v, buffers)
		}
		return newData
	default:
		return data
	}
}

// Reconstructs a binary packet from its placeholder packet and buffers
func ReconstructPacket(packet *Packet, buffers []types.BufferInterface) (*Packet, error) {
	data, err := _reconstructPacket(packet.Data, &buffers)
	if err != nil {
		return nil, err
	}
	packet.Data = data
	packet.Attachments = nil // Attachments are no longer needed
	return packet, nil
}

func extractValue[T any](m map[string]any, key string) (v T, err error) {
	val, ok := m[key]
	if !ok {
		return v, fmt.Errorf("missing '%s' field", key)
	}
	if v, ok := val.(T); ok {
		return v, nil
	}
	return v, fmt.Errorf("invalid type for '%s' field: expected %T, got %T", key, v, val)
}

func processPlaceholder(d map[string]any) (*Placeholder, error) {
	placeholder, err := extractValue[bool](d, "_placeholder")
	if err != nil {
		return nil, err
	}

	num, err := extractValue[float64](d, "num")
	if err != nil {
		return nil, err
	}

	return &Placeholder{
		Placeholder: placeholder,
		Num:         int64(num),
	}, nil
}

func _reconstructPacket(data any, buffers *[]types.BufferInterface) (any, error) {
	switch d := data.(type) {
	case nil:
		return nil, nil
	case []any:
		newData := make([]any, 0, len(d))
		for _, v := range d {
			_data, err := _reconstructPacket(v, buffers)
			if err != nil {
				return nil, err
			}
			newData = append(newData, _data)
		}
		return newData, nil
	case map[string]any:
		if _placeholder, err := processPlaceholder(d); err == nil && _placeholder.Placeholder {
			if _placeholder.Num >= 0 && _placeholder.Num < int64(len(*buffers)) {
				return (*buffers)[_placeholder.Num], nil
			}
			return nil, errors.New("illegal attachments")
		}

		newData := make(map[string]any, len(d))
		for k, v := range d {
			_data, err := _reconstructPacket(v, buffers)
			if err != nil {
				return nil, err
			}
			newData[k] = _data
		}
		return newData, nil
	default:
		return data, nil
	}
}
