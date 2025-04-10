package types

import (
	"io"

	"github.com/zishang520/socket.io/v3/types"
)

type (
	// Alias  [types.BufferInterface]
	//
	// Deprecated: will be removed in future versions, please use [types.BufferInterface]
	BufferInterface = types.BufferInterface
	// Alias  [types.Buffer]
	//
	// Deprecated: will be removed in future versions, please use [types.Buffer]
	Buffer = types.Buffer
	// Alias  [types.BytesBuffer]
	//
	// Deprecated: will be removed in future versions, please use [types.BytesBuffer]
	BytesBuffer = types.BytesBuffer
	// Alias  [types.StringBuffer]
	//
	// Deprecated: will be removed in future versions, please use [types.StringBuffer]
	StringBuffer = types.StringBuffer
)

// Alias  [types.NewBytesBuffer]
//
// Deprecated: will be removed in future versions, please use [types.NewBytesBuffer]
func NewBytesBuffer(buf []byte) BufferInterface {
	return types.NewBytesBuffer(buf)
}

// Alias  [types.NewBytesBufferReader]
//
// Deprecated: will be removed in future versions, please use [types.NewBytesBufferReader]
func NewBytesBufferReader(r io.Reader) (BufferInterface, error) {
	return types.NewBytesBufferReader(r)
}

// Alias  [types.NewBytesBufferString]
//
// Deprecated: will be removed in future versions, please use [types.NewBytesBufferString]
func NewBytesBufferString(s string) BufferInterface {
	return types.NewBytesBufferString(s)
}

// Alias  [types.NewStringBuffer]
//
// Deprecated: will be removed in future versions, please use [types.NewStringBuffer]
func NewStringBuffer(buf []byte) BufferInterface {
	return types.NewStringBuffer(buf)
}

// Alias  [types.NewStringBufferReader]
//
// Deprecated: will be removed in future versions, please use [types.NewStringBufferReader]
func NewStringBufferReader(r io.Reader) (BufferInterface, error) {
	return types.NewStringBufferReader(r)
}

// Alias  [types.NewStringBufferString]
//
// Deprecated: will be removed in future versions, please use [types.NewStringBufferString]
func NewStringBufferString(s string) BufferInterface {
	return types.NewStringBufferString(s)
}

// Alias  [types.NewBuffer]
//
// Deprecated: will be removed in future versions, please use [types.NewBuffer]
func NewBuffer(buf []byte) *Buffer {
	return types.NewBuffer(buf)
}

// Alias  [types.NewBufferString]
//
// Deprecated: will be removed in future versions, please use [types.NewBufferString]
func NewBufferString(s string) *Buffer {
	return types.NewBufferString(s)
}
