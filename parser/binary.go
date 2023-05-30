package parser

import (
	"errors"
	"io"

	"github.com/mitchellh/mapstructure"
	"github.com/zishang520/engine.io-go-parser/types"
)

type Placeholder struct {
	Placeholder bool `json:"_placeholder" mapstructure:"_placeholder"`
	Num         int  `json:"num" mapstructure:"num"`
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
		_placeholder := &Placeholder{Placeholder: true, Num: len(*buffers)}
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
	} else {
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
		}
	}
	return data
}

// Reconstructs a binary packet from its placeholder packet and buffers
func ReconstructPacket(data *Packet, buffers []types.BufferInterface) (_ *Packet, err error) {
	data.Data, err = _reconstructPacket(data.Data, &buffers)
	data.Attachments = nil // no longer useful
	return data, nil
}

func _reconstructPacket(data any, buffers *[]types.BufferInterface) (any, error) {
	if data == nil {
		return nil, nil
	}
	switch d := data.(type) {
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
		var _placeholder *Placeholder
		if mapstructure.Decode(d, &_placeholder) == nil {
			if _placeholder.Placeholder {
				if _placeholder.Num >= 0 && _placeholder.Num < len(*buffers) {
					return (*buffers)[_placeholder.Num], nil // appropriate buffer (should be natural order anyway)
				}
				return nil, errors.New("illegal attachments")
			}
		}
		newData := map[string]any{}
		for k, v := range d {
			_data, err := _reconstructPacket(v, buffers)
			if err != nil {
				return nil, err
			}
			newData[k] = _data
		}
		return newData, nil
	}
	return data, nil
}
