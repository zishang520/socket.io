package types

import (
	"io"

	"github.com/zishang520/socket.io/parsers/engine/v3/types"
)

// Buffer types alias for simplicity
type (
	BufferInterface = types.BufferInterface
	Buffer          = types.Buffer
	BytesBuffer     = types.BytesBuffer
	StringBuffer    = types.StringBuffer
)

// NewBytesBuffer creates a new BytesBuffer from a byte slice.
func NewBytesBuffer(buf []byte) BufferInterface {
	return types.NewBytesBuffer(buf)
}

// NewBytesBufferReader creates a new BytesBuffer from an io.Reader.
func NewBytesBufferReader(r io.Reader) (BufferInterface, error) {
	return types.NewBytesBufferReader(r)
}

// NewBytesBufferString creates a new BytesBuffer from a string.
func NewBytesBufferString(s string) BufferInterface {
	return types.NewBytesBufferString(s)
}

// NewStringBuffer creates a new StringBuffer from a byte slice.
func NewStringBuffer(buf []byte) BufferInterface {
	return types.NewStringBuffer(buf)
}

// NewStringBufferReader creates a new StringBuffer from an io.Reader.
func NewStringBufferReader(r io.Reader) (BufferInterface, error) {
	return types.NewStringBufferReader(r)
}

// NewStringBufferString creates a new StringBuffer from a string.
func NewStringBufferString(s string) BufferInterface {
	return types.NewStringBufferString(s)
}

// NewBuffer creates a new Buffer from a byte slice.
func NewBuffer(buf []byte) *Buffer {
	return types.NewBuffer(buf)
}

// NewBufferString creates a new Buffer from a string.
func NewBufferString(s string) *Buffer {
	return types.NewBufferString(s)
}
