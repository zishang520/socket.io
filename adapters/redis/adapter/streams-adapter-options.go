// Package adapter provides configuration options for the Redis Streams-based Socket.IO adapter.
// The streams adapter uses Redis Streams for message persistence and session recovery.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Default configuration values for RedisStreamsAdapterOptions.
const (
	DefaultStreamName       = "socket.io"
	DefaultStreamMaxLen     = 10_000
	DefaultStreamReadCount  = 100
	DefaultSessionKeyPrefix = "sio:session:"
)

type (
	// RedisStreamsAdapterOptionsInterface defines the interface for configuring RedisStreamsAdapterOptions.
	// It extends ClusterAdapterOptionsInterface with streams-specific settings.
	RedisStreamsAdapterOptionsInterface interface {
		adapter.ClusterAdapterOptionsInterface

		SetStreamName(string)
		GetRawStreamName() types.Optional[string]
		StreamName() string

		SetMaxLen(int64)
		GetRawMaxLen() types.Optional[int64]
		MaxLen() int64

		SetReadCount(int64)
		GetRawReadCount() types.Optional[int64]
		ReadCount() int64

		SetSessionKeyPrefix(string)
		GetRawSessionKeyPrefix() types.Optional[string]
		SessionKeyPrefix() string
	}

	// RedisStreamsAdapterOptions holds configuration for the Redis Streams adapter.
	//
	// Fields:
	//   - streamName: The name of the Redis stream. Default: "socket.io".
	//   - maxLen: The maximum size of the stream (approximate). Default: 10,000.
	//   - readCount: The number of elements to fetch per XREAD call. Default: 100.
	//   - sessionKeyPrefix: The prefix for session keys in Redis. Default: "sio:session:".
	RedisStreamsAdapterOptions struct {
		adapter.ClusterAdapterOptions

		streamName       types.Optional[string]
		maxLen           types.Optional[int64]
		readCount        types.Optional[int64]
		sessionKeyPrefix types.Optional[string]
	}
)

// DefaultRedisStreamsAdapterOptions returns a new RedisStreamsAdapterOptions with default values.
func DefaultRedisStreamsAdapterOptions() *RedisStreamsAdapterOptions {
	return &RedisStreamsAdapterOptions{}
}

// Assign copies non-nil fields from another RedisStreamsAdapterOptionsInterface.
// This method is useful for merging user-provided options with defaults.
func (s *RedisStreamsAdapterOptions) Assign(data RedisStreamsAdapterOptionsInterface) RedisStreamsAdapterOptionsInterface {
	if data == nil {
		return s
	}

	s.ClusterAdapterOptions.Assign(data)

	if data.GetRawStreamName() != nil {
		s.SetStreamName(data.StreamName())
	}
	if data.GetRawMaxLen() != nil {
		s.SetMaxLen(data.MaxLen())
	}
	if data.GetRawReadCount() != nil {
		s.SetReadCount(data.ReadCount())
	}
	if data.GetRawSessionKeyPrefix() != nil {
		s.SetSessionKeyPrefix(data.SessionKeyPrefix())
	}

	return s
}

// SetStreamName sets the Redis stream name.
func (s *RedisStreamsAdapterOptions) SetStreamName(streamName string) {
	s.streamName = types.NewSome(streamName)
}

// GetRawStreamName returns the raw Optional value for streamName.
func (s *RedisStreamsAdapterOptions) GetRawStreamName() types.Optional[string] {
	return s.streamName
}

// StreamName returns the configured stream name.
// Returns empty string if not set; callers should use DefaultStreamName as fallback.
func (s *RedisStreamsAdapterOptions) StreamName() string {
	if s.streamName == nil {
		return ""
	}
	return s.streamName.Get()
}

// SetMaxLen sets the maximum stream length.
func (s *RedisStreamsAdapterOptions) SetMaxLen(maxLen int64) {
	s.maxLen = types.NewSome(maxLen)
}

// GetRawMaxLen returns the raw Optional value for maxLen.
func (s *RedisStreamsAdapterOptions) GetRawMaxLen() types.Optional[int64] {
	return s.maxLen
}

// MaxLen returns the configured maximum stream length.
// Returns 0 if not set; callers should use DefaultStreamMaxLen as fallback.
func (s *RedisStreamsAdapterOptions) MaxLen() int64 {
	if s.maxLen == nil {
		return 0
	}
	return s.maxLen.Get()
}

// SetReadCount sets the number of elements to fetch per XREAD call.
func (s *RedisStreamsAdapterOptions) SetReadCount(readCount int64) {
	s.readCount = types.NewSome(readCount)
}

// GetRawReadCount returns the raw Optional value for readCount.
func (s *RedisStreamsAdapterOptions) GetRawReadCount() types.Optional[int64] {
	return s.readCount
}

// ReadCount returns the configured read count.
// Returns 0 if not set; callers should use DefaultStreamReadCount as fallback.
func (s *RedisStreamsAdapterOptions) ReadCount() int64 {
	if s.readCount == nil {
		return 0
	}
	return s.readCount.Get()
}

// SetSessionKeyPrefix sets the session key prefix.
func (s *RedisStreamsAdapterOptions) SetSessionKeyPrefix(sessionKeyPrefix string) {
	s.sessionKeyPrefix = types.NewSome(sessionKeyPrefix)
}

// GetRawSessionKeyPrefix returns the raw Optional value for sessionKeyPrefix.
func (s *RedisStreamsAdapterOptions) GetRawSessionKeyPrefix() types.Optional[string] {
	return s.sessionKeyPrefix
}

// SessionKeyPrefix returns the configured session key prefix.
// Returns empty string if not set; callers should use DefaultSessionKeyPrefix as fallback.
func (s *RedisStreamsAdapterOptions) SessionKeyPrefix() string {
	if s.sessionKeyPrefix == nil {
		return ""
	}
	return s.sessionKeyPrefix.Get()
}
