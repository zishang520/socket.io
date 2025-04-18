// Package adapter provides configuration options for the Redis Streams-based Socket.IO adapter.
package adapter

import "github.com/zishang520/socket.io/adapters/adapter/v3"

type (
	// RedisStreamsAdapterOptionsInterface defines the interface for configuring RedisStreamsAdapterOptions.
	RedisStreamsAdapterOptionsInterface interface {
		adapter.ClusterAdapterOptionsInterface

		SetStreamName(string)
		GetRawStreamName() *string
		StreamName() string

		SetMaxLen(int64)
		GetRawMaxLen() *int64
		MaxLen() int64

		SetReadCount(int64)
		GetRawReadCount() *int64
		ReadCount() int64

		SetSessionKeyPrefix(string)
		GetRawSessionKeyPrefix() *string
		SessionKeyPrefix() string
	}

	// RedisStreamsAdapterOptions holds configuration for the Redis Streams adapter.
	//
	// streamName: the name of the Redis stream (default: "socket.io").
	// maxLen: the maximum size of the stream (default: 10_000).
	// readCount: the number of elements to fetch per XREAD call (default: 100).
	// sessionKeyPrefix: the prefix for session keys (default: "sio:session:").
	RedisStreamsAdapterOptions struct {
		adapter.ClusterAdapterOptions

		streamName       *string
		maxLen           *int64
		readCount        *int64
		sessionKeyPrefix *string
	}
)

// DefaultRedisStreamsAdapterOptions returns a new RedisStreamsAdapterOptions with default values.
func DefaultRedisStreamsAdapterOptions() *RedisStreamsAdapterOptions {
	return &RedisStreamsAdapterOptions{}
}

// Assign copies non-nil fields from another RedisStreamsAdapterOptionsInterface.
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
	s.streamName = &streamName
}

// GetRawStreamName returns the raw stream name pointer.
func (s *RedisStreamsAdapterOptions) GetRawStreamName() *string {
	return s.streamName
}

// StreamName returns the Redis stream name.
func (s *RedisStreamsAdapterOptions) StreamName() string {
	if s.streamName == nil {
		return ""
	}
	return *s.streamName
}

// SetMaxLen sets the maximum stream length.
func (s *RedisStreamsAdapterOptions) SetMaxLen(maxLen int64) {
	s.maxLen = &maxLen
}

// GetRawMaxLen returns the raw maxLen pointer.
func (s *RedisStreamsAdapterOptions) GetRawMaxLen() *int64 {
	return s.maxLen
}

// MaxLen returns the maximum stream length.
func (s *RedisStreamsAdapterOptions) MaxLen() int64 {
	if s.maxLen == nil {
		return 0
	}
	return *s.maxLen
}

// SetReadCount sets the number of elements to fetch per XREAD call.
func (s *RedisStreamsAdapterOptions) SetReadCount(readCount int64) {
	s.readCount = &readCount
}

// GetRawReadCount returns the raw readCount pointer.
func (s *RedisStreamsAdapterOptions) GetRawReadCount() *int64 {
	return s.readCount
}

// ReadCount returns the number of elements to fetch per XREAD call.
func (s *RedisStreamsAdapterOptions) ReadCount() int64 {
	if s.readCount == nil {
		return 0
	}
	return *s.readCount
}

// SetSessionKeyPrefix sets the session key prefix.
func (s *RedisStreamsAdapterOptions) SetSessionKeyPrefix(sessionKeyPrefix string) {
	s.sessionKeyPrefix = &sessionKeyPrefix
}

// GetRawSessionKeyPrefix returns the raw sessionKeyPrefix pointer.
func (s *RedisStreamsAdapterOptions) GetRawSessionKeyPrefix() *string {
	return s.sessionKeyPrefix
}

// SessionKeyPrefix returns the session key prefix.
func (s *RedisStreamsAdapterOptions) SessionKeyPrefix() string {
	if s.sessionKeyPrefix == nil {
		return ""
	}
	return *s.sessionKeyPrefix
}
