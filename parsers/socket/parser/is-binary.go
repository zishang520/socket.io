package parser

import (
	"io"
	"strings"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// IsBinary determines if the given data is a binary type.
// Returns true for []byte and io.Reader types (excluding StringBuffer and strings.Reader),
// which are treated as binary data in the Socket.IO protocol.
func IsBinary(data any) bool {
	switch data.(type) {
	case *types.StringBuffer, *strings.Reader:
		// StringBuffer and strings.Reader are text-based, not binary
		return false
	case []byte, io.Reader:
		// Byte slices and other readers are considered binary
		return true
	default:
		return false
	}
}

// HasBinary recursively checks if the data contains any binary content.
// It traverses slices and maps to detect nested binary data.
func HasBinary(data any) bool {
	switch v := data.(type) {
	case nil:
		return false
	case []any:
		for _, item := range v {
			if HasBinary(item) {
				return true
			}
		}
		return false
	case map[string]any:
		for _, value := range v {
			if HasBinary(value) {
				return true
			}
		}
		return false
	default:
		return IsBinary(data)
	}
}
