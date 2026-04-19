// Package adapter provides configuration options for the Valkey Streams-based Socket.IO adapter.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	DefaultStreamName          = "socket.io"
	DefaultStreamMaxLen        = 10_000
	DefaultStreamReadCount     = 100
	DefaultStreamCount         = 1
	DefaultStreamChannelPrefix = "socket.io"
	DefaultBlockTimeInMs       = 5_000
	DefaultSessionKeyPrefix    = "sio:session:"
)

type (
	// ValkeyStreamsAdapterOptionsInterface defines the interface for configuring ValkeyStreamsAdapterOptions.
	ValkeyStreamsAdapterOptionsInterface interface {
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

	// ValkeyStreamsAdapterOptions holds configuration for the Valkey Streams adapter.
	ValkeyStreamsAdapterOptions struct {
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

// DefaultValkeyStreamsAdapterOptions returns a new ValkeyStreamsAdapterOptions with default values.
func DefaultValkeyStreamsAdapterOptions() *ValkeyStreamsAdapterOptions {
	return &ValkeyStreamsAdapterOptions{}
}

// Assign copies non-nil fields from another ValkeyStreamsAdapterOptionsInterface.
func (s *ValkeyStreamsAdapterOptions) Assign(data ValkeyStreamsAdapterOptionsInterface) ValkeyStreamsAdapterOptionsInterface {
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

func (s *ValkeyStreamsAdapterOptions) SetStreamName(v string) { s.streamName = types.NewSome(v) }
func (s *ValkeyStreamsAdapterOptions) GetRawStreamName() types.Optional[string] {
	return s.streamName
}
func (s *ValkeyStreamsAdapterOptions) StreamName() string {
	if s.streamName == nil {
		return ""
	}
	return s.streamName.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetStreamCount(v int) { s.streamCount = types.NewSome(v) }
func (s *ValkeyStreamsAdapterOptions) GetRawStreamCount() types.Optional[int] {
	return s.streamCount
}
func (s *ValkeyStreamsAdapterOptions) StreamCount() int {
	if s.streamCount == nil {
		return 0
	}
	return s.streamCount.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetChannelPrefix(v string) {
	s.channelPrefix = types.NewSome(v)
}
func (s *ValkeyStreamsAdapterOptions) GetRawChannelPrefix() types.Optional[string] {
	return s.channelPrefix
}
func (s *ValkeyStreamsAdapterOptions) ChannelPrefix() string {
	if s.channelPrefix == nil {
		return ""
	}
	return s.channelPrefix.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetUseShardedPubSub(v bool) {
	s.useShardedPubSub = types.NewSome(v)
}
func (s *ValkeyStreamsAdapterOptions) GetRawUseShardedPubSub() types.Optional[bool] {
	return s.useShardedPubSub
}
func (s *ValkeyStreamsAdapterOptions) UseShardedPubSub() bool {
	if s.useShardedPubSub == nil {
		return false
	}
	return s.useShardedPubSub.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetMaxLen(v int64)                   { s.maxLen = types.NewSome(v) }
func (s *ValkeyStreamsAdapterOptions) GetRawMaxLen() types.Optional[int64] { return s.maxLen }
func (s *ValkeyStreamsAdapterOptions) MaxLen() int64 {
	if s.maxLen == nil {
		return 0
	}
	return s.maxLen.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetReadCount(v int64)                   { s.readCount = types.NewSome(v) }
func (s *ValkeyStreamsAdapterOptions) GetRawReadCount() types.Optional[int64] { return s.readCount }
func (s *ValkeyStreamsAdapterOptions) ReadCount() int64 {
	if s.readCount == nil {
		return 0
	}
	return s.readCount.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetBlockTimeInMs(v int64) {
	s.blockTimeInMs = types.NewSome(v)
}
func (s *ValkeyStreamsAdapterOptions) GetRawBlockTimeInMs() types.Optional[int64] {
	return s.blockTimeInMs
}
func (s *ValkeyStreamsAdapterOptions) BlockTimeInMs() int64 {
	if s.blockTimeInMs == nil {
		return 0
	}
	return s.blockTimeInMs.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetSessionKeyPrefix(v string) {
	s.sessionKeyPrefix = types.NewSome(v)
}
func (s *ValkeyStreamsAdapterOptions) GetRawSessionKeyPrefix() types.Optional[string] {
	return s.sessionKeyPrefix
}
func (s *ValkeyStreamsAdapterOptions) SessionKeyPrefix() string {
	if s.sessionKeyPrefix == nil {
		return ""
	}
	return s.sessionKeyPrefix.Get()
}

func (s *ValkeyStreamsAdapterOptions) SetOnlyPlaintext(v bool) {
	s.onlyPlaintext = types.NewSome(v)
}
func (s *ValkeyStreamsAdapterOptions) GetRawOnlyPlaintext() types.Optional[bool] {
	return s.onlyPlaintext
}
func (s *ValkeyStreamsAdapterOptions) OnlyPlaintext() bool {
	if s.onlyPlaintext == nil {
		return false
	}
	return s.onlyPlaintext.Get()
}
