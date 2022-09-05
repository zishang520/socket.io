package parser

import (
	"github.com/zishang520/engine.io/events"
	"github.com/zishang520/engine.io/types"
)

type Parser interface {
	// A socket.io Encoder instance
	Encoder() Encoder

	// A socket.io Decoder instance
	Decoder() Decoder
}

// A socket.io Encoder instance
type Encoder interface {
	Encode(*Packet) []types.BufferInterface
}

// A socket.io Decoder instance
type Decoder interface {
	events.EventEmitter

	Add(any) error
	Destroy()
}

type parser struct {
	encoder Encoder
	decoder Decoder
}

func (p *parser) Encoder() Encoder {
	return p.encoder
}

func (p *parser) Decoder() Decoder {
	return p.decoder
}

func NewParser() Parser {
	return &parser{
		encoder: NewEncoder(),
		decoder: NewDecoder(),
	}
}
