// Package emitter provides an API for broadcasting messages to Socket.IO servers via PostgreSQL
// without requiring a full Socket.IO server instance.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/postgres/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	// DefaultEmitterKey is the default PostgreSQL channel prefix for the emitter.
	DefaultEmitterKey = "socket.io"

	// DefaultTableName is the default name for the attachment storage table.
	DefaultTableName = "socket_io_attachments"

	// DefaultPayloadThreshold is the default byte threshold for using attachment storage.
	// PostgreSQL's NOTIFY payload limit is 8000 bytes.
	DefaultPayloadThreshold = 8000
)

type (
	// EmitterOptionsInterface defines the interface for configuring emitter options.
	// It provides getters and setters for all configurable options.
	EmitterOptionsInterface interface {
		// SetKey sets the PostgreSQL channel prefix for notifications.
		SetKey(string)
		// GetRawKey returns the raw Optional wrapper for the key setting.
		GetRawKey() types.Optional[string]
		// Key returns the PostgreSQL channel prefix, or empty string if not set.
		Key() string

		// SetParser sets the parser for encoding messages.
		SetParser(postgres.Parser)
		// GetRawParser returns the raw Optional wrapper for the parser setting.
		GetRawParser() types.Optional[postgres.Parser]
		// Parser returns the parser, or nil if not set.
		Parser() postgres.Parser

		// SetTableName sets the attachment table name.
		SetTableName(string)
		// GetRawTableName returns the raw Optional wrapper for the tableName setting.
		GetRawTableName() types.Optional[string]
		// TableName returns the attachment table name, or empty string if not set.
		TableName() string

		// SetPayloadThreshold sets the byte threshold for attachment storage.
		SetPayloadThreshold(int)
		// GetRawPayloadThreshold returns the raw Optional wrapper for the payloadThreshold setting.
		GetRawPayloadThreshold() types.Optional[int]
		// PayloadThreshold returns the payload threshold, or 0 if not set.
		PayloadThreshold() int
	}

	// EmitterOptions holds configuration options for the PostgreSQL emitter.
	// All fields are optional and will use default values if not explicitly set.
	EmitterOptions struct {
		// key is the PostgreSQL channel prefix used for constructing channel names.
		// Default: "socket.io"
		key types.Optional[string]

		// parser is the encoder/decoder used for serializing messages.
		// Default: MessagePack parser
		parser types.Optional[postgres.Parser]

		// tableName is the name of the attachment table for large payloads.
		// Default: "socket_io_attachments"
		tableName types.Optional[string]

		// payloadThreshold is the byte threshold for using attachment storage.
		// Default: 8000
		payloadThreshold types.Optional[int]
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
	if data.GetRawTableName() != nil {
		o.SetTableName(data.TableName())
	}
	if data.GetRawPayloadThreshold() != nil {
		o.SetPayloadThreshold(data.PayloadThreshold())
	}

	return o
}

// SetKey sets the PostgreSQL channel prefix.
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
func (o *EmitterOptions) SetParser(parser postgres.Parser) {
	o.parser = types.NewSome(parser)
}

// GetRawParser returns the raw Optional value for parser.
func (o *EmitterOptions) GetRawParser() types.Optional[postgres.Parser] {
	return o.parser
}

// Parser returns the configured parser, or nil if not set.
func (o *EmitterOptions) Parser() postgres.Parser {
	if o.parser == nil {
		return nil
	}
	return o.parser.Get()
}

// SetTableName sets the attachment table name.
func (o *EmitterOptions) SetTableName(tableName string) {
	o.tableName = types.NewSome(tableName)
}

// GetRawTableName returns the raw Optional value for tableName.
func (o *EmitterOptions) GetRawTableName() types.Optional[string] {
	return o.tableName
}

// TableName returns the configured table name, or empty string if not set.
func (o *EmitterOptions) TableName() string {
	if o.tableName == nil {
		return ""
	}
	return o.tableName.Get()
}

// SetPayloadThreshold sets the byte threshold for attachment storage.
func (o *EmitterOptions) SetPayloadThreshold(threshold int) {
	o.payloadThreshold = types.NewSome(threshold)
}

// GetRawPayloadThreshold returns the raw Optional value for payloadThreshold.
func (o *EmitterOptions) GetRawPayloadThreshold() types.Optional[int] {
	return o.payloadThreshold
}

// PayloadThreshold returns the configured payload threshold, or 0 if not set.
func (o *EmitterOptions) PayloadThreshold() int {
	if o.payloadThreshold == nil {
		return 0
	}
	return o.payloadThreshold.Get()
}
