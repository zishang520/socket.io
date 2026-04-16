// Package emitter provides an API for broadcasting messages to Socket.IO servers via MongoDB
// without requiring a full Socket.IO server instance.
package emitter

import (
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	// DefaultEmitterKey is a placeholder to match the interface pattern.
	// The MongoDB emitter does not use channel prefixes since it uses a single collection.
	DefaultEmitterKey = "socket.io"
)

type (
	// EmitterOptionsInterface defines the interface for configuring emitter options.
	// It provides getters and setters for all configurable options.
	EmitterOptionsInterface interface {
		// SetAddCreatedAtField sets whether to add a createdAt field to documents.
		SetAddCreatedAtField(bool)
		// GetRawAddCreatedAtField returns the raw Optional wrapper for the addCreatedAtField setting.
		GetRawAddCreatedAtField() types.Optional[bool]
		// AddCreatedAtField returns the addCreatedAtField value, or false if not set.
		AddCreatedAtField() bool
	}

	// EmitterOptions holds configuration options for the MongoDB emitter.
	// All fields are optional and will use default values if not explicitly set.
	EmitterOptions struct {
		// addCreatedAtField indicates whether to add a createdAt field to each MongoDB document.
		// Required when using a TTL index instead of a capped collection.
		// Default: false
		addCreatedAtField types.Optional[bool]
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

	if data.GetRawAddCreatedAtField() != nil {
		o.SetAddCreatedAtField(data.AddCreatedAtField())
	}

	return o
}

// SetAddCreatedAtField sets whether to add a createdAt field to documents.
func (o *EmitterOptions) SetAddCreatedAtField(v bool) {
	o.addCreatedAtField = types.NewSome(v)
}

// GetRawAddCreatedAtField returns the raw Optional value for addCreatedAtField.
func (o *EmitterOptions) GetRawAddCreatedAtField() types.Optional[bool] {
	return o.addCreatedAtField
}

// AddCreatedAtField returns the configured addCreatedAtField value, or false if not set.
func (o *EmitterOptions) AddCreatedAtField() bool {
	if o.addCreatedAtField == nil {
		return false
	}
	return o.addCreatedAtField.Get()
}
