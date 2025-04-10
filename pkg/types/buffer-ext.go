package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

type BufferInterface interface {
	io.ReadWriteSeeker
	io.ReaderFrom
	io.WriterTo
	io.ByteScanner
	io.ByteWriter
	io.RuneScanner
	io.StringWriter
	WriteRune(rune) (int, error)
	Bytes() []byte
	AvailableBuffer() []byte
	fmt.Stringer
	fmt.GoStringer
	Len() int
	Size() int64
	Cap() int
	Available() int
	Truncate(int)
	Reset()
	Grow(int)
	Next(int) []byte
	ReadBytes(byte) ([]byte, error)
	ReadString(byte) (string, error)
	Clone() BufferInterface
}

// Clone creates a deep copy of the Buffer.
// The returned Buffer has the same content and capacity as the original,
// but it is an independent copy.
func (b *Buffer) Clone() *Buffer {
	if b == nil {
		return nil
	}

	clone := &Buffer{
		buf:      make([]byte, len(b.buf), cap(b.buf)),
		off:      b.off,
		lastRead: b.lastRead,
	}

	copy(clone.buf, b.buf)

	return clone
}

// Size returns the original length of the underlying byte slice.
// Size is the number of bytes available for reading via ReadAt.
// The returned value is always the same and is not affected by calls
// to any other method.
func (b *Buffer) Size() int64 { return int64(len(b.buf)) }

// Seek implements the io.Seeker interface.
func (b *Buffer) Seek(offset int64, whence int) (int64, error) {
	b.lastRead = opInvalid
	var abs int
	switch whence {
	case io.SeekStart:
		abs = int(offset)
	case io.SeekCurrent:
		abs = b.off + int(offset)
	case io.SeekEnd:
		abs = len(b.buf) + int(offset)
	default:
		return 0, errors.New("types.Buffer.Seek: invalid whence")
	}
	if abs < 0 || abs > len(b.buf) {
		return 0, errors.New("types.Buffer.Seek: negative position")
	}
	b.off = abs
	return int64(abs), nil
}

// IndexByte returns the index of the first instance of c in b, or -1 if c is not present in b.
func IndexByte(b []byte, c byte) int {
	return bytes.IndexByte(b, c)
}

// bytes buffer
type BytesBuffer struct {
	*Buffer
}

func (b *BytesBuffer) Clone() BufferInterface {
	if b == nil || b.Buffer == nil {
		return nil
	}
	return &BytesBuffer{b.Buffer.Clone()}
}

func (b *BytesBuffer) GoString() string {
	if b == nil || b.Buffer == nil {
		// Special case, useful in debugging.
		return "<nil>"
	}
	return fmt.Sprintf("%v", b.Buffer.Bytes())
}

func NewBytesBufferReader(r io.Reader) (BufferInterface, error) {
	b := NewBytesBuffer(nil)
	_, err := b.ReadFrom(r)
	return b, err
}

func NewBytesBuffer(buf []byte) BufferInterface {
	return &BytesBuffer{NewBuffer(buf)}
}

func NewBytesBufferString(s string) BufferInterface {
	return &BytesBuffer{NewBufferString(s)}
}

// string buffer
type StringBuffer struct {
	*Buffer
}

func (sb *StringBuffer) Clone() BufferInterface {
	if sb == nil || sb.Buffer == nil {
		return nil
	}
	return &StringBuffer{sb.Buffer.Clone()}
}

func (sb *StringBuffer) GoString() string {
	if sb == nil || sb.Buffer == nil {
		// Special case, useful in debugging.
		return "<nil>"
	}
	return sb.Buffer.String()
}

// MarshalJSON returns sb as the JSON encoding of m.
func (sb *StringBuffer) MarshalJSON() ([]byte, error) {
	if sb == nil || sb.Buffer == nil {
		return []byte(`""`), nil
	}

	return json.Marshal(sb.String())
}

// UnmarshalJSON decodes a JSON-encoded string into the StringBuffer.
func (sb *StringBuffer) UnmarshalJSON(data []byte) error {
	if sb == nil {
		return nil
	}

	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	// Clear and populate the buffer with the new string.
	sb.Buffer = NewBufferString(str)
	return nil
}

func NewStringBufferReader(r io.Reader) (BufferInterface, error) {
	b := NewStringBuffer(nil)
	_, err := b.ReadFrom(r)
	return b, err
}

func NewStringBuffer(buf []byte) BufferInterface {
	return &StringBuffer{NewBuffer(buf)}
}

func NewStringBufferString(s string) BufferInterface {
	return &StringBuffer{NewBufferString(s)}
}
