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
	DefaultStreamCount      = 1
	DefaultChannelPrefix    = "socket.io"
	DefaultBlockTimeInMs    = 5_000
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

		SetStreamCount(int)
		GetRawStreamCount() types.Optional[int]
		StreamCount() int

		SetChannelPrefix(string)
		GetRawChannelPrefix() types.Optional[string]
		ChannelPrefix() string

		SetUseShardedPubSub(bool)
		GetRawUseShardedPubSub() types.Optional[bool]
		UseShardedPubSub() bool

		SetMaxLen(int64)
		GetRawMaxLen() types.Optional[int64]
		MaxLen() int64

		SetReadCount(int64)
		GetRawReadCount() types.Optional[int64]
		ReadCount() int64

		SetBlockTimeInMs(int64)
		GetRawBlockTimeInMs() types.Optional[int64]
		BlockTimeInMs() int64

		SetSessionKeyPrefix(string)
		GetRawSessionKeyPrefix() types.Optional[string]
		SessionKeyPrefix() string

		SetOnlyPlaintext(bool)
		GetRawOnlyPlaintext() types.Optional[bool]
		OnlyPlaintext() bool
	}

	// RedisStreamsAdapterOptions holds configuration for the Redis Streams adapter.
	RedisStreamsAdapterOptions struct {
		adapter.ClusterAdapterOptions

		streamName       types.Optional[string]
		streamCount      types.Optional[int]
		channelPrefix    types.Optional[string]
		useShardedPubSub types.Optional[bool]
		maxLen           types.Optional[int64]
		readCount        types.Optional[int64]
		blockTimeInMs    types.Optional[int64]
		sessionKeyPrefix types.Optional[string]
		onlyPlaintext    types.Optional[bool]
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
	if data.GetRawStreamCount() != nil {
		s.SetStreamCount(data.StreamCount())
	}
	if data.GetRawChannelPrefix() != nil {
		s.SetChannelPrefix(data.ChannelPrefix())
	}
	if data.GetRawUseShardedPubSub() != nil {
		s.SetUseShardedPubSub(data.UseShardedPubSub())
	}
	if data.GetRawMaxLen() != nil {
		s.SetMaxLen(data.MaxLen())
	}
	if data.GetRawReadCount() != nil {
		s.SetReadCount(data.ReadCount())
	}
	if data.GetRawBlockTimeInMs() != nil {
		s.SetBlockTimeInMs(data.BlockTimeInMs())
	}
	if data.GetRawSessionKeyPrefix() != nil {
		s.SetSessionKeyPrefix(data.SessionKeyPrefix())
	}
	if data.GetRawOnlyPlaintext() != nil {
		s.SetOnlyPlaintext(data.OnlyPlaintext())
	}

	return s
}

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

func (s *RedisStreamsAdapterOptions) SetStreamCount(streamCount int) {
	s.streamCount = types.NewSome(streamCount)
}
func (s *RedisStreamsAdapterOptions) GetRawStreamCount() types.Optional[int] {
	return s.streamCount
}
func (s *RedisStreamsAdapterOptions) StreamCount() int {
	if s.streamCount == nil {
		return 0
	}
	return s.streamCount.Get()
}

func (s *RedisStreamsAdapterOptions) SetChannelPrefix(channelPrefix string) {
	s.channelPrefix = types.NewSome(channelPrefix)
}
func (s *RedisStreamsAdapterOptions) GetRawChannelPrefix() types.Optional[string] {
	return s.channelPrefix
}
func (s *RedisStreamsAdapterOptions) ChannelPrefix() string {
	if s.channelPrefix == nil {
		return ""
	}
	return s.channelPrefix.Get()
}

func (s *RedisStreamsAdapterOptions) SetUseShardedPubSub(useShardedPubSub bool) {
	s.useShardedPubSub = types.NewSome(useShardedPubSub)
}
func (s *RedisStreamsAdapterOptions) GetRawUseShardedPubSub() types.Optional[bool] {
	return s.useShardedPubSub
}
func (s *RedisStreamsAdapterOptions) UseShardedPubSub() bool {
	if s.useShardedPubSub == nil {
		return false
	}
	return s.useShardedPubSub.Get()
}

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

func (s *RedisStreamsAdapterOptions) SetBlockTimeInMs(blockTimeInMs int64) {
	s.blockTimeInMs = types.NewSome(blockTimeInMs)
}
func (s *RedisStreamsAdapterOptions) GetRawBlockTimeInMs() types.Optional[int64] {
	return s.blockTimeInMs
}
func (s *RedisStreamsAdapterOptions) BlockTimeInMs() int64 {
	if s.blockTimeInMs == nil {
		return 0
	}
	return s.blockTimeInMs.Get()
}

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

func (s *RedisStreamsAdapterOptions) SetOnlyPlaintext(onlyPlaintext bool) {
	s.onlyPlaintext = types.NewSome(onlyPlaintext)
}
func (s *RedisStreamsAdapterOptions) GetRawOnlyPlaintext() types.Optional[bool] {
	return s.onlyPlaintext
}
func (s *RedisStreamsAdapterOptions) OnlyPlaintext() bool {
	if s.onlyPlaintext == nil {
		return false
	}
	return s.onlyPlaintext.Get()
}
