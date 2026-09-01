// Package emitter provides an API for broadcasting messages to Socket.IO servers via PostgreSQL
// without requiring a full Socket.IO server instance.
package emitter

import (
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

const (
	// DefaultChannelPrefix is the default PostgreSQL channel prefix for the emitter.
	DefaultChannelPrefix = "socket.io"

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
		// SetChannelPrefix sets the PostgreSQL channel prefix for notifications.
		SetChannelPrefix(string)
		// GetRawChannelPrefix returns the raw Optional wrapper for the channel prefix.
		GetRawChannelPrefix() types.Optional[string]
		// ChannelPrefix returns the PostgreSQL channel prefix, or empty string if not set.
		ChannelPrefix() string

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
		// channelPrefix is the PostgreSQL channel prefix used for constructing channel names.
		// Default: "socket.io"
		channelPrefix types.Optional[string]

		// tableName is the name of the attachment table for large payloads.
		// Default: "socket_io_attachments"
		tableName types.Optional[string]

		// payloadThreshold is the byte threshold for using attachment storage.
		// Default: 8000
		payloadThreshold types.Optional[int]
	}
)

// DefaultEmitterOptions returns empty options.
// Defaults are applied when the emitter is constructed.
func DefaultEmitterOptions() *EmitterOptions {
	return &EmitterOptions{}
}

// Assign copies non-nil option values from another EmitterOptionsInterface.
// This allows merging configuration from multiple sources.
func (o *EmitterOptions) Assign(data EmitterOptionsInterface) EmitterOptionsInterface {
	if utils.IsNil(data) {
		return o
	}

	if data.GetRawChannelPrefix() != nil {
		o.SetChannelPrefix(data.ChannelPrefix())
	}
	if data.GetRawTableName() != nil {
		o.SetTableName(data.TableName())
	}
	if data.GetRawPayloadThreshold() != nil {
		o.SetPayloadThreshold(data.PayloadThreshold())
	}

	return o
}

// SetChannelPrefix sets the PostgreSQL notification channel prefix.
func (o *EmitterOptions) SetChannelPrefix(channelPrefix string) {
	o.channelPrefix = types.NewSome(channelPrefix)
}

// GetRawChannelPrefix returns the raw Optional value for channelPrefix.
func (o *EmitterOptions) GetRawChannelPrefix() types.Optional[string] {
	return o.channelPrefix
}

// ChannelPrefix returns the configured channel prefix, or empty string if not set.
func (o *EmitterOptions) ChannelPrefix() string {
	if o.channelPrefix == nil {
		return ""
	}
	return o.channelPrefix.Get()
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
