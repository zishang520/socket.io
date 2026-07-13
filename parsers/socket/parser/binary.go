package parser

import (
	"errors"
	"io"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Placeholder represents a placeholder for binary data in JSON serialization.
// When a packet contains binary data, the binary is extracted and replaced
// with a placeholder containing the index into the buffers array.
type Placeholder struct {
	Placeholder bool  `json:"_placeholder" msgpack:"_placeholder"`
	Num         int64 `json:"num" msgpack:"num"`
}

// DeconstructPacket extracts all binary data from a packet and replaces
// them with numbered placeholders. Returns the modified packet and a slice
// of buffers containing the extracted binary data.
func DeconstructPacket(packet *Packet) (*Packet, []types.BufferInterface) {
	var buffers []types.BufferInterface
	packet.Data = deconstructData(packet.Data, &buffers)
	packet.Attachments = new(uint64(len(buffers)))
	return packet, buffers
}

// deconstructData recursively traverses the data structure and replaces
// binary data with placeholders while collecting the binary data into buffers.
func deconstructData(data any, buffers *[]types.BufferInterface) any {
	if data == nil {
		return nil
	}

	if IsBinary(data) {
		return extractBinaryData(data, buffers)
	}

	switch typedData := data.(type) {
	case []any:
		return deconstructSlice(typedData, buffers)
	case map[string]any:
		return deconstructMap(typedData, buffers)
	default:
		return data
	}
}

// extractBinaryData extracts binary data, stores it in buffers, and returns a placeholder.
func extractBinaryData(data any, buffers *[]types.BufferInterface) *Placeholder {
	placeholder := &Placeholder{
		Placeholder: true,
		Num:         int64(len(*buffers)),
	}

	buffer := types.NewBytesBuffer(nil)
	switch typedData := data.(type) {
	case io.Reader:
		if closer, ok := data.(io.Closer); ok {
			defer func() { _ = closer.Close() }()
		}
		_, _ = buffer.ReadFrom(typedData)
	case []byte:
		_, _ = buffer.Write(typedData)
	}

	*buffers = append(*buffers, buffer)
	return placeholder
}

// deconstructSlice processes a slice, deconstructing any binary data within.
func deconstructSlice(data []any, buffers *[]types.BufferInterface) []any {
	result := make([]any, 0, len(data))
	for _, item := range data {
		result = append(result, deconstructData(item, buffers))
	}
	return result
}

// deconstructMap processes a map, deconstructing any binary data within.
func deconstructMap(data map[string]any, buffers *[]types.BufferInterface) map[string]any {
	result := make(map[string]any, len(data))
	for key, value := range data {
		result[key] = deconstructData(value, buffers)
	}
	return result
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
	switch typedData := data.(type) {
	case nil:
		return nil, nil
	case []any:
		return reconstructSlice(typedData, buffers)
	case map[string]any:
		return reconstructMap(typedData, buffers)
	default:
		return data, nil
	}
}

// reconstructSlice processes a slice, reconstructing any placeholders within.
func reconstructSlice(data []any, buffers []types.BufferInterface) ([]any, error) {
	result := make([]any, 0, len(data))
	for _, item := range data {
		reconstructed, err := reconstructData(item, buffers)
		if err != nil {
			return nil, err
		}
		result = append(result, reconstructed)
	}
	return result, nil
}

// reconstructMap processes a map, reconstructing any placeholders within.
// If the map itself is a placeholder, it returns the corresponding buffer.
func reconstructMap(data map[string]any, buffers []types.BufferInterface) (any, error) {
	// Check if this map is a placeholder
	if placeholderFlag, ok := data["_placeholder"].(bool); ok && placeholderFlag {
		if num, ok := data["num"].(float64); ok && num >= 0 {
			index := int64(num)
			if num == float64(index) {
				if index >= int64(len(buffers)) {
					return nil, ErrIllegalAttachments
				}
				return buffers[index], nil
			}
		}
	}

	// Not a placeholder, reconstruct nested data
	result := make(map[string]any, len(data))
	for key, value := range data {
		reconstructed, err := reconstructData(value, buffers)
		if err != nil {
			return nil, err
		}
		result[key] = reconstructed
	}
	return result, nil
}
