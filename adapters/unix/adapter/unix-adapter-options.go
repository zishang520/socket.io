// Package adapter provides configuration options for the Unix Domain Socket-based Socket.IO adapter.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/unix/v3/emitter"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Default configuration values for UnixAdapterOptions.
const (
	// DefaultChannelPrefix is the default channel prefix for Unix Domain Socket adapter.
	DefaultChannelPrefix = "socket.io"

	// DefaultSocketPath is the default path for the Unix Domain Socket.
	DefaultSocketPath = "/tmp/socket.io.sock"

	// DefaultHeartbeatInterval is the default interval between heartbeats.
	DefaultHeartbeatInterval = 5_000 * time.Millisecond

	// DefaultHeartbeatTimeout is the default timeout for heartbeat responses.
	DefaultHeartbeatTimeout int64 = 10_000
)

type (
	// UnixAdapterOptionsInterface defines the interface for configuring UnixAdapterOptions.
	// It extends EmitterOptionsInterface and ClusterAdapterOptionsInterface with adapter-specific settings.
	UnixAdapterOptionsInterface interface {
		emitter.EmitterOptionsInterface
		adapter.ClusterAdapterOptionsInterface

		SetErrorHandler(func(error))
		GetRawErrorHandler() types.Optional[func(error)]
		ErrorHandler() func(error)
	}

	// UnixAdapterOptions holds configuration for the Unix Domain Socket adapter.
	//
	// Fields:
	//   - errorHandler: Custom error handler callback.
	//     Default: nil (errors are logged via debug).
	UnixAdapterOptions struct {
		emitter.EmitterOptions
		adapter.ClusterAdapterOptions

		errorHandler types.Optional[func(error)]
	}
)

// DefaultUnixAdapterOptions returns a new UnixAdapterOptions with default values.
func DefaultUnixAdapterOptions() *UnixAdapterOptions {
	return &UnixAdapterOptions{}
}

// Assign copies non-nil fields from another UnixAdapterOptionsInterface.
// This method is useful for merging user-provided options with defaults.
func (s *UnixAdapterOptions) Assign(data UnixAdapterOptionsInterface) UnixAdapterOptionsInterface {
	if data == nil {
		return s
	}

	s.EmitterOptions.Assign(data)
	s.ClusterAdapterOptions.Assign(data)

	if data.GetRawErrorHandler() != nil {
		s.SetErrorHandler(data.ErrorHandler())
	}

	return s
}

// SetErrorHandler sets the error handler callback.
func (s *UnixAdapterOptions) SetErrorHandler(handler func(error)) {
	s.errorHandler = types.NewSome(handler)
}

// GetRawErrorHandler returns the raw Optional value for errorHandler.
func (s *UnixAdapterOptions) GetRawErrorHandler() types.Optional[func(error)] {
	return s.errorHandler
}

// ErrorHandler returns the configured error handler callback, or nil if not set.
func (s *UnixAdapterOptions) ErrorHandler() func(error) {
	if s.errorHandler == nil {
		return nil
	}
	return s.errorHandler.Get()
}
