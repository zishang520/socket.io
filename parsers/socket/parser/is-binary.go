package parser

import (
	"io"
	"strings"

	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

// IsBinary returns true if the data is a binary type (Buffer or File).
func IsBinary(data any) bool {
	switch data.(type) {
	case *types.StringBuffer, *strings.Reader:
		return false
	case []byte, io.Reader:
		return true
	default:
		return false
	}
}

// HasBinary checks recursively if the data contains any binary data.
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
	case map[string]any:
		for _, value := range v {
			if HasBinary(value) {
				return true
			}
		}
	default:
		return IsBinary(data)
	}
	return false
}
