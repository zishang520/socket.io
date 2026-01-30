package parser

import (
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Encoder defines the interface for Socket.IO packet encoding.
// Implementations convert Packet structures into wire format.
type Encoder interface {
	// Encode converts a packet into a sequence of buffers.
	// For non-binary packets, returns a single string buffer.
	// For binary packets, returns the header buffer followed by binary data buffers.
	Encode(*Packet) []types.BufferInterface
}

// Decoder defines the interface for Socket.IO packet decoding.
// Implementations parse wire format data into Packet structures.
type Decoder interface {
	types.EventEmitter

	// Add processes incoming data (string or binary).
	// Emits "decoded" event when a complete packet is available.
	Add(any) error

	// Destroy releases resources and stops any ongoing operations.
	Destroy()
}

// Parser defines the interface for creating Encoder and Decoder instances.
// It serves as a factory for the Socket.IO parser components.
type Parser interface {
	// NewEncoder creates a new Encoder instance.
	NewEncoder() Encoder

	// NewDecoder creates a new Decoder instance.
	NewDecoder() Decoder
}

// parser is the default implementation of the Parser interface.
type parser struct{}

// NewEncoder creates a new Encoder instance.
func (p *parser) NewEncoder() Encoder {
	return NewEncoder()
}

// NewDecoder creates a new Decoder instance.
func (p *parser) NewDecoder() Decoder {
	return NewDecoder()
}

// NewParser creates a new Parser instance that can create
// Encoder and Decoder instances for Socket.IO packet handling.
func NewParser() Parser {
	return &parser{}
}
