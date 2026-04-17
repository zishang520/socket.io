// Package emitter provides an API for broadcasting messages to Socket.IO servers via Unix Domain Socket
// without requiring a full Socket.IO server instance.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/unix/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	// DefaultEmitterKey is the default channel prefix for the emitter.
	DefaultEmitterKey = "socket.io"

	// DefaultSocketPath is the default path for the Unix Domain Socket.
	DefaultSocketPath = "/tmp/socket.io.sock"
)

type (
	// EmitterOptionsInterface defines the interface for configuring emitter options.
	// It provides getters and setters for all configurable options.
	EmitterOptionsInterface interface {
		// SetKey sets the channel prefix.
		SetKey(string)
		// GetRawKey returns the raw Optional wrapper for the key setting.
		GetRawKey() types.Optional[string]
		// Key returns the channel prefix, or empty string if not set.
		Key() string

		// SetParser sets the parser for encoding messages.
		SetParser(unix.Parser)
		// GetRawParser returns the raw Optional wrapper for the parser setting.
		GetRawParser() types.Optional[unix.Parser]
		// Parser returns the parser, or nil if not set.
		Parser() unix.Parser

		// SetSocketPath sets the Unix Domain Socket path.
		SetSocketPath(string)
		// GetRawSocketPath returns the raw Optional wrapper for the socketPath setting.
		GetRawSocketPath() types.Optional[string]
		// SocketPath returns the Unix Domain Socket path, or empty string if not set.
		SocketPath() string
	}

	// EmitterOptions holds configuration options for the Unix Domain Socket emitter.
	// All fields are optional and will use default values if not explicitly set.
	EmitterOptions struct {
		// key is the channel prefix used for constructing channel names.
		// Default: "socket.io"
		key types.Optional[string]

		// parser is the encoder/decoder used for serializing messages.
		// Default: nil (uses JSON)
		parser types.Optional[unix.Parser]

		// socketPath is the path of the Unix Domain Socket.
		// Default: "/tmp/socket.io.sock"
		socketPath types.Optional[string]
	}
)

// DefaultEmitterOptions creates a new EmitterOptions instance with default values.
func DefaultEmitterOptions() *EmitterOptions {
	return &EmitterOptions{}
}

// Assign copies non-nil option values from another EmitterOptionsInterface.
// This allows merging configuration from multiple sources.
func (o *EmitterOptions) Assign(data EmitterOptionsInterface) EmitterOptionsInterface {
	if data == nil {
		return o
	}

	if data.GetRawKey() != nil {
		o.SetKey(data.Key())
	}
	if data.Parser() != nil {
		o.SetParser(data.Parser())
	}
	if data.GetRawSocketPath() != nil {
		o.SetSocketPath(data.SocketPath())
	}

	return o
}

// SetKey sets the channel prefix.
func (o *EmitterOptions) SetKey(key string) {
	o.key = types.NewSome(key)
}

// GetRawKey returns the raw Optional value for key.
func (o *EmitterOptions) GetRawKey() types.Optional[string] {
	return o.key
}

// Key returns the configured channel prefix, or empty string if not set.
func (o *EmitterOptions) Key() string {
	if o.key == nil {
		return ""
	}
	return o.key.Get()
}

// SetParser sets the parser for message encoding/decoding.
func (o *EmitterOptions) SetParser(parser unix.Parser) {
	o.parser = types.NewSome(parser)
}

// GetRawParser returns the raw Optional value for parser.
func (o *EmitterOptions) GetRawParser() types.Optional[unix.Parser] {
	return o.parser
}

// Parser returns the configured parser, or nil if not set.
func (o *EmitterOptions) Parser() unix.Parser {
	if o.parser == nil {
		return nil
	}
	return o.parser.Get()
}

// SetSocketPath sets the Unix Domain Socket path.
func (o *EmitterOptions) SetSocketPath(path string) {
	o.socketPath = types.NewSome(path)
}

// GetRawSocketPath returns the raw Optional value for socketPath.
func (o *EmitterOptions) GetRawSocketPath() types.Optional[string] {
	return o.socketPath
}

// SocketPath returns the configured Unix Domain Socket path, or empty string if not set.
func (o *EmitterOptions) SocketPath() string {
	if o.socketPath == nil {
		return ""
	}
	return o.socketPath.Get()
}
