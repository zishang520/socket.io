package socket

import (
	"io"
	"strings"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// prepareRetryData consumes readers before enqueueing so every attempt has the same data.
// It follows the default parser's text/binary distinction and supported containers.
func prepareRetryData(data any) (any, error) {
	switch value := data.(type) {
	case *strings.Reader:
		text, err := io.ReadAll(value)
		return string(text), err
	case *types.StringBuffer:
		// JSON encoding does not consume this text buffer.
		return value, nil
	case io.Reader:
		if closer, ok := value.(io.Closer); ok {
			defer func() { _ = closer.Close() }()
		}
		return io.ReadAll(value)
	case []any:
		if value == nil {
			return value, nil
		}
		result := make([]any, len(value))
		for i, item := range value {
			converted, err := prepareRetryData(item)
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
			converted, err := prepareRetryData(item)
			if err != nil {
				return nil, err
			}
			result[key] = converted
		}
		return result, nil
	default:
		return data, nil
	}
}
