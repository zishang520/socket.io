// Package adapter provides configuration options for the Redis Streams-based Socket.IO adapter.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	// RedisStreamsAdapterOptionsInterface defines the interface for configuring RedisStreamsAdapterOptions.
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
	// streamName: the name of the Redis stream (default: "socket.io").
	// maxLen: the maximum size of the stream (default: 10_000).
	// readCount: the number of elements to fetch per XREAD call (default: 100).
	// sessionKeyPrefix: the prefix for session keys (default: "sio:session:").
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
func (s *RedisStreamsAdapterOptions) GetRawStreamName() types.Optional[string] {
	return s.streamName
}
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
func (s *RedisStreamsAdapterOptions) GetRawMaxLen() types.Optional[int64] {
	return s.maxLen
}
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
func (s *RedisStreamsAdapterOptions) GetRawReadCount() types.Optional[int64] {
	return s.readCount
}
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
func (s *RedisStreamsAdapterOptions) GetRawSessionKeyPrefix() types.Optional[string] {
	return s.sessionKeyPrefix
}
func (s *RedisStreamsAdapterOptions) SessionKeyPrefix() string {
	if s.sessionKeyPrefix == nil {
		return ""
	}

	return s.sessionKeyPrefix.Get()
}
