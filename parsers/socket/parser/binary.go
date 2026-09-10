package parser

import (
	"errors"
	"io"
	"math"
	"strings"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Placeholder represents a placeholder for binary data in JSON serialization.
// When a packet contains binary data, the binary is extracted and replaced
// with a placeholder containing the index into the buffers array.
type Placeholder struct {
	Placeholder bool  `json:"_placeholder" msgpack:"_placeholder"`
	Num         int64 `json:"num" msgpack:"num"`
}

// DeconstructPacket extracts binary data and replaces it with placeholders.
// It changes packet only on success. Reader data is consumed and cannot be rolled back.
func DeconstructPacket(packet *Packet) (*Packet, []types.BufferInterface, error) {
	var buffers []types.BufferInterface
	data, err := prepareData(packet.Data, &buffers)
	if err != nil {
		return nil, nil, err
	}
	packet.Data = data
	packet.Attachments = new(uint64(len(buffers)))
	return packet, buffers, nil
}

// prepareData copies containers, converts text readers, and optionally extracts binary.
// A nil buffers pointer leaves binary values alone for non-EVENT/ACK packets.
func prepareData(data any, buffers *[]types.BufferInterface) (any, error) {
	switch value := data.(type) {
	case *strings.Reader:
		return types.NewStringBufferReader(value)
	case *types.StringBuffer:
		return value, nil
	case []any:
		if value == nil {
			return value, nil
		}
		result := make([]any, len(value))
		for i, item := range value {
			converted, err := prepareData(item, buffers)
			if err != nil {
				return nil, err
			}
			result[i] = converted
		}
		return result, nil
	case map[string]any:
		if value == nil {
			return value, nil
		}
		result := make(map[string]any, len(value))
		for key, item := range value {
			converted, err := prepareData(item, buffers)
			if err != nil {
				return nil, err
			}
			result[key] = converted
		}
		return result, nil
	}
	if buffers == nil || !IsBinary(data) {
		return data, nil
	}
	buffer := types.NewBytesBuffer(nil)
	switch value := data.(type) {
	case io.Reader:
		if closer, ok := value.(io.Closer); ok {
			defer func() { _ = closer.Close() }()
		}
		if _, err := buffer.ReadFrom(value); err != nil {
			return nil, err
		}
	case []byte:
		_, _ = buffer.Write(value)
	}
	placeholder := &Placeholder{Placeholder: true, Num: int64(len(*buffers))}
	*buffers = append(*buffers, buffer)
	return placeholder, nil
}

// ErrIllegalAttachments is returned when a placeholder references an invalid buffer index.
var ErrIllegalAttachments = errors.New("illegal attachments")

// ReconstructPacket reconstructs a binary packet from its placeholder packet
// and the corresponding buffers. It replaces all placeholders with their
// corresponding binary data from the buffers slice.
func ReconstructPacket(packet *Packet, buffers []types.BufferInterface) (*Packet, error) {
	data, err := reconstructData(packet.Data, buffers)
	if err != nil {
		return nil, err
	}
	packet.Data = data
	packet.Attachments = nil // Attachments are no longer needed after reconstruction
	return packet, nil
}

// reconstructData recursively traverses the data structure and replaces
// placeholders with their corresponding binary data from the buffers.
func reconstructData(data any, buffers []types.BufferInterface) (any, error) {
	switch value := data.(type) {
	case *Placeholder:
		if value == nil || !value.Placeholder {
			return value, nil
		}
		if value.Num < 0 || value.Num >= int64(len(buffers)) {
			return nil, ErrIllegalAttachments
		}
		return buffers[value.Num], nil
	case []any:
		if value == nil {
			return value, nil
		}
		result := make([]any, len(value))
		for i, item := range value {
			reconstructed, err := reconstructData(item, buffers)
			if err != nil {
				return nil, err
			}
			result[i] = reconstructed
		}
		return result, nil
	case map[string]any:
		if value == nil {
			return value, nil
		}
		if value["_placeholder"] == true {
			num, ok := value["num"].(float64)
			if !ok || num < 0 || num >= float64(len(buffers)) || math.Trunc(num) != num {
				return nil, ErrIllegalAttachments
			}
			return buffers[int(num)], nil
		}
		result := make(map[string]any, len(value))
		for key, item := range value {
			reconstructed, err := reconstructData(item, buffers)
			if err != nil {
				return nil, err
			}
			result[key] = reconstructed
		}
		return result, nil
	default:
		return data, nil
	}
}
