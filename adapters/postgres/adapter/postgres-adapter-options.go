// Package adapter provides configuration options for the PostgreSQL-based Socket.IO adapter.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/postgres/v3/emitter"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Default configuration values for PostgresAdapterOptions.
const (
	// DefaultChannelPrefix is the default PostgreSQL channel prefix for LISTEN/NOTIFY.
	DefaultChannelPrefix = "socket.io"

	// DefaultTableName is the default name for the attachment storage table.
	DefaultTableName = "socket_io_attachments"

	// DefaultPayloadThreshold is the default byte threshold for using attachment storage.
	// PostgreSQL's NOTIFY payload limit is 8000 bytes.
	DefaultPayloadThreshold = 8000

	// DefaultCleanupInterval is the default interval in milliseconds for cleaning up old attachments.
	DefaultCleanupInterval int64 = 30_000

	// DefaultHeartbeatInterval is the default interval between heartbeats.
	DefaultHeartbeatInterval = 5_000 * time.Millisecond

	// DefaultHeartbeatTimeout is the default timeout for heartbeat responses.
	DefaultHeartbeatTimeout int64 = 10_000
)

type (
	// PostgresAdapterOptionsInterface defines the interface for configuring PostgresAdapterOptions.
	// It extends EmitterOptionsInterface and ClusterAdapterOptionsInterface with adapter-specific settings.
	PostgresAdapterOptionsInterface interface {
		emitter.EmitterOptionsInterface
		adapter.ClusterAdapterOptionsInterface

		SetCleanupInterval(int64)
		GetRawCleanupInterval() types.Optional[int64]
		CleanupInterval() int64

		SetErrorHandler(func(error))
		GetRawErrorHandler() types.Optional[func(error)]
		ErrorHandler() func(error)
	}

	// PostgresAdapterOptions holds configuration for the PostgreSQL adapter.
	//
	// Fields:
	//   - cleanupInterval: The interval in milliseconds between cleanup of old attachments.
	//     Default: 30_000 ms.
	//   - errorHandler: Custom error handler callback.
	//     Default: nil (errors are logged via debug).
	PostgresAdapterOptions struct {
		emitter.EmitterOptions
		adapter.ClusterAdapterOptions

		cleanupInterval types.Optional[int64]
		errorHandler    types.Optional[func(error)]
	}
)

// DefaultPostgresAdapterOptions returns a new PostgresAdapterOptions with default values.
func DefaultPostgresAdapterOptions() *PostgresAdapterOptions {
	return &PostgresAdapterOptions{}
}

// Assign copies non-nil fields from another PostgresAdapterOptionsInterface.
// This method is useful for merging user-provided options with defaults.
func (s *PostgresAdapterOptions) Assign(data PostgresAdapterOptionsInterface) PostgresAdapterOptionsInterface {
	if data == nil {
		return s
	}

	s.EmitterOptions.Assign(data)
	s.ClusterAdapterOptions.Assign(data)

	if data.GetRawCleanupInterval() != nil {
		s.SetCleanupInterval(data.CleanupInterval())
	}
	if data.GetRawErrorHandler() != nil {
		s.SetErrorHandler(data.ErrorHandler())
	}

	return s
}

// SetCleanupInterval sets the cleanup interval in milliseconds.
func (s *PostgresAdapterOptions) SetCleanupInterval(interval int64) {
	s.cleanupInterval = types.NewSome(interval)
}

// GetRawCleanupInterval returns the raw Optional value for cleanupInterval.
func (s *PostgresAdapterOptions) GetRawCleanupInterval() types.Optional[int64] {
	return s.cleanupInterval
}

// CleanupInterval returns the configured cleanup interval in milliseconds.
// Returns 0 if not set; callers should use DefaultCleanupInterval as fallback.
func (s *PostgresAdapterOptions) CleanupInterval() int64 {
	if s.cleanupInterval == nil {
		return 0
	}
	return s.cleanupInterval.Get()
}

// SetErrorHandler sets the error handler callback.
func (s *PostgresAdapterOptions) SetErrorHandler(handler func(error)) {
	s.errorHandler = types.NewSome(handler)
}

// GetRawErrorHandler returns the raw Optional value for errorHandler.
func (s *PostgresAdapterOptions) GetRawErrorHandler() types.Optional[func(error)] {
	return s.errorHandler
}

// ErrorHandler returns the configured error handler callback, or nil if not set.
func (s *PostgresAdapterOptions) ErrorHandler() func(error) {
	if s.errorHandler == nil {
		return nil
	}
	return s.errorHandler.Get()
}
