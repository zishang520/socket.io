package parser

import (
	"io"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

// MockBuffer is a mock implementation of BufferInterface for testing purposes
type MockBuffer struct {
	data []byte
}

func (mb *MockBuffer) Read(p []byte) (n int, err error) {
	return copy(p, mb.data), nil
}

func (mb *MockBuffer) Write(p []byte) (n int, err error) {
	mb.data = append(mb.data, p...)
	return len(p), nil
}

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name string
		data any
		want bool
	}{
		{"nil value", nil, false},
		{"StringBuffer", &types.StringBuffer{}, false},
		{"strings.Reader", &strings.Reader{}, false},
		{"[]byte", []byte("test"), true},
		{"io.Reader", &MockBuffer{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBinary(tt.data); got != tt.want {
				t.Errorf("IsBinary(%v) = %v; want %v", tt.data, got, tt.want)
			}
		})
	}
}

func TestHasBinary(t *testing.T) {
	tests := []struct {
		name string
		data any
		want bool
	}{
		{"nil value", nil, false},
		{"[]any with no binary", []any{"string", 123, nil}, false},
		{"[]any with binary", []any{"string", []byte("binary data"), 123}, true},
		{"map[string]any with no binary", map[string]any{"key": "value"}, false},
		{"map[string]any with binary", map[string]any{"key": []byte("binary data")}, true},
		{"nested structure with binary", map[string]any{"key": []any{"string", []byte("binary data")}}, true},
		{"io.Reader in map", map[string]any{"key": io.Reader(&MockBuffer{})}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := HasBinary(tt.data); got != tt.want {
				t.Errorf("HasBinary(%v) = %v; want %v", tt.data, got, tt.want)
			}
		})
	}
}
