// Package packet defines the Engine.IO packet types and structures.
package packet

import (
	"io"
	"sync"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Type represents the Engine.IO packet type.
type Type string

// String returns the string representation of the packet type.
func (t Type) String() string {
	return string(t)
}

// IsValid checks if the packet type is a valid Engine.IO packet type.
func (t Type) IsValid() bool {
	switch t {
	case OPEN, CLOSE, PING, PONG, MESSAGE, UPGRADE, NOOP:
		return true
	default:
		return false
	}
}

// Options contains optional packet configuration.
type Options struct {
	// Compress indicates whether the packet should be compressed.
	Compress *bool `json:"compress,omitempty" msgpack:"compress,omitempty"`
	// WsPreEncodedFrame contains a pre-encoded WebSocket frame.
	WsPreEncodedFrame types.BufferInterface `json:"wsPreEncodedFrame,omitempty" msgpack:"wsPreEncodedFrame,omitempty"`
	// PreparedFrame, when non-nil, is shared across all recipients of a
	// broadcast and lets transports build their per-broadcast prepared
	// frame (e.g. ws.NewPreparedMessage) once instead of once per
	// recipient. The same cache may be used by multiple transport
	// types; entries are keyed by transport name.
	PreparedFrame *PreparedFrame `json:"-" msgpack:"-"`
}

// NewOptions creates a new Options with the given compress flag.
func NewOptions(compress bool) *Options {
	return &Options{Compress: &compress}
}

// Packet represents an Engine.IO packet.
type Packet struct {
	// Type is the packet type (OPEN, CLOSE, PING, PONG, MESSAGE, UPGRADE, NOOP, ERROR).
	Type Type `json:"type" msgpack:"type"`
	// Data contains the packet payload.
	Data io.Reader `json:"data,omitempty" msgpack:"data,omitempty"`
	// Options contains optional packet configuration.
	Options *Options `json:"options,omitempty" msgpack:"options,omitempty"`
}

// New creates a new packet with the given type and data.
func New(packetType Type, data io.Reader) *Packet {
	return &Packet{
		Type: packetType,
		Data: data,
	}
}

// NewWithOptions creates a new packet with the given type, data, and options.
func NewWithOptions(packetType Type, data io.Reader, options *Options) *Packet {
	return &Packet{
		Type:    packetType,
		Data:    data,
		Options: options,
	}
}

// PreparedFrame is a transport-keyed, build-once cache that lets a
// broadcast amortize the cost of preparing its WebSocket frame across
// every recipient. Allocate one instance per broadcast and pass it via
// Options.PreparedFrame; the zero value is ready to use and safe for
// concurrent use.
type PreparedFrame struct {
	mu      sync.Mutex
	entries map[string]*preparedEntry
}

type preparedEntry struct {
	once sync.Once
	val  any
	err  error
}

// Do returns the cached value for key, invoking build at most once per
// key even when called concurrently. If p is nil, build is invoked on
// every call (no caching).
func (p *PreparedFrame) Do(key string, build func() (any, error)) (any, error) {
	if p == nil {
		return build()
	}
	p.mu.Lock()
	if p.entries == nil {
		p.entries = make(map[string]*preparedEntry)
	}
	e, ok := p.entries[key]
	if !ok {
		e = &preparedEntry{}
		p.entries[key] = e
	}
	p.mu.Unlock()

	e.once.Do(func() { e.val, e.err = build() })
	return e.val, e.err
}

// Packet types for Engine.IO protocol.
const (
	// OPEN is sent from the server when a new transport is opened.
	OPEN Type = "open"
	// CLOSE is sent to request the close of this transport.
	CLOSE Type = "close"
	// PING is sent by the client for keep-alive (heartbeat).
	PING Type = "ping"
	// PONG is sent by the server in response to a PING.
	PONG Type = "pong"
	// MESSAGE is used for actual message transport.
	MESSAGE Type = "message"
	// UPGRADE is sent before upgrading the transport.
	UPGRADE Type = "upgrade"
	// NOOP is used as a no-operation packet.
	NOOP Type = "noop"
	// ERROR indicates a parsing or other error.
	ERROR Type = "error"
)
