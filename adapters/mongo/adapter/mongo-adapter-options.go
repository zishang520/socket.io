// Package adapter provides configuration options for the MongoDB-based Socket.IO adapter.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Default configuration values for MongoAdapterOptions.
const (
	// DefaultHeartbeatInterval is the default interval between heartbeats.
	DefaultHeartbeatInterval = 5_000 * time.Millisecond

	// DefaultHeartbeatTimeout is the default timeout for heartbeat responses.
	DefaultHeartbeatTimeout int64 = 10_000
)

type (
	// MongoAdapterOptionsInterface defines the interface for configuring MongoAdapterOptions.
	// It extends ClusterAdapterOptionsInterface with adapter-specific settings.
	MongoAdapterOptionsInterface interface {
		adapter.ClusterAdapterOptionsInterface

		SetAddCreatedAtField(bool)
		GetRawAddCreatedAtField() types.Optional[bool]
		AddCreatedAtField() bool

		SetErrorHandler(func(error))
		GetRawErrorHandler() types.Optional[func(error)]
		ErrorHandler() func(error)
	}

	// MongoAdapterOptions holds configuration for the MongoDB adapter.
	//
	// Fields:
	//   - addCreatedAtField: Whether to add a createdAt field to each MongoDB document.
	//     Required when using a TTL index instead of a capped collection.
	//     Default: false.
	//   - errorHandler: Custom error handler callback.
	//     Default: nil (errors are logged via debug).
	MongoAdapterOptions struct {
		adapter.ClusterAdapterOptions

		addCreatedAtField types.Optional[bool]
		errorHandler      types.Optional[func(error)]
	}
)

// DefaultMongoAdapterOptions returns a new MongoAdapterOptions with default values.
func DefaultMongoAdapterOptions() *MongoAdapterOptions {
	return &MongoAdapterOptions{}
}

// Assign copies non-nil fields from another MongoAdapterOptionsInterface.
// This method is useful for merging user-provided options with defaults.
func (s *MongoAdapterOptions) Assign(data MongoAdapterOptionsInterface) MongoAdapterOptionsInterface {
	if data == nil {
		return s
	}

	s.ClusterAdapterOptions.Assign(data)

	if data.GetRawAddCreatedAtField() != nil {
		s.SetAddCreatedAtField(data.AddCreatedAtField())
	}
	if data.GetRawErrorHandler() != nil {
		s.SetErrorHandler(data.ErrorHandler())
	}

	return s
}

// SetAddCreatedAtField sets whether to add a createdAt field to documents.
func (s *MongoAdapterOptions) SetAddCreatedAtField(v bool) {
	s.addCreatedAtField = types.NewSome(v)
}

// GetRawAddCreatedAtField returns the raw Optional value for addCreatedAtField.
func (s *MongoAdapterOptions) GetRawAddCreatedAtField() types.Optional[bool] {
	return s.addCreatedAtField
}

// AddCreatedAtField returns the configured addCreatedAtField value.
// Returns false if not set.
func (s *MongoAdapterOptions) AddCreatedAtField() bool {
	if s.addCreatedAtField == nil {
		return false
	}
	return s.addCreatedAtField.Get()
}

// SetErrorHandler sets the error handler callback.
func (s *MongoAdapterOptions) SetErrorHandler(handler func(error)) {
	s.errorHandler = types.NewSome(handler)
}

// GetRawErrorHandler returns the raw Optional value for errorHandler.
func (s *MongoAdapterOptions) GetRawErrorHandler() types.Optional[func(error)] {
	return s.errorHandler
}

// ErrorHandler returns the configured error handler callback, or nil if not set.
func (s *MongoAdapterOptions) ErrorHandler() func(error) {
	if s.errorHandler == nil {
		return nil
	}
	return s.errorHandler.Get()
}
