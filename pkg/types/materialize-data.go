package types

import (
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// MaterializeData materializes Socket.IO buffers and readers as native
// strings or byte slices for a wire encoder. It visits []any and map[string]any
// values, consuming and closing binary readers as the Socket.IO parser does.
// It returns the prepared value, whether the value was replaced, and whether
// it contains binary data. Read errors are returned; consumed readers cannot be rolled back.
func MaterializeData(data any) (any, bool, bool, error) {
	switch value := data.(type) {
	case nil:
		return nil, false, false, nil
	case *strings.Reader:
		if value == nil {
			return nil, true, false, nil
		}
		var payload strings.Builder
		payload.Grow(value.Len())
		_, _ = value.WriteTo(&payload)
		return payload.String(), true, false, nil
	case *StringBuffer:
		if value == nil || value.Buffer == nil {
			return nil, true, false, nil
		}
		return value.String(), true, false, nil
	case []byte:
		return utils.NonNilSlice(value), value == nil, true, nil
	case *BytesBuffer:
		if value == nil {
			return nil, true, false, nil
		}
		var payload []byte
		if value.Buffer != nil {
			payload = value.Bytes()
		}
		return utils.NonNilSlice(payload), true, true, nil
	case io.Reader:
		if utils.IsNil(data) {
			return data, false, false, nil
		}
		payload, err := io.ReadAll(value)
		if closer, ok := data.(io.Closer); ok {
			_ = closer.Close()
		}
		if err != nil {
			return nil, false, false, err
		}
		return utils.NonNilSlice(payload), true, true, nil
	case []any:
		var result []any
		var binary bool
		for i, item := range value {
			encoded, changed, hasBinary, err := MaterializeData(item)
			if err != nil {
				return nil, false, false, err
			}
			binary = binary || hasBinary
			if !changed {
				continue
			}
			if result == nil {
				result = slices.Clone(value)
			}
			result[i] = encoded
		}
		if result != nil {
			return result, true, binary, nil
		}
		return data, false, binary, nil
	case map[string]any:
		var result map[string]any
		var binary bool
		for key, item := range value {
			encoded, changed, hasBinary, err := MaterializeData(item)
			if err != nil {
				return nil, false, false, err
			}
			binary = binary || hasBinary
			if !changed {
				continue
			}
			if result == nil {
				result = maps.Clone(value)
			}
			result[key] = encoded
		}
		if result != nil {
			return result, true, binary, nil
		}
		return data, false, binary, nil
	default:
		return data, false, false, nil
	}
}
