// Package adapter provides configuration options for the cache sharded pub/sub adapter.
package adapter

import (
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	// DefaultShardedChannelPrefix is the default channel prefix for the sharded adapter.
	DefaultShardedChannelPrefix = "socket.io"
)

// DefaultShardedSubscriptionMode is the default subscription mode for the sharded adapter.
var DefaultShardedSubscriptionMode = cache.DynamicSubscriptionMode

type (
	// ShardedCacheAdapterOptionsInterface is the configuration interface for the sharded adapter.
	ShardedCacheAdapterOptionsInterface interface {
		SetChannelPrefix(string)
		GetRawChannelPrefix() types.Optional[string]
		ChannelPrefix() string

		SetSubscriptionMode(cache.SubscriptionMode)
		GetRawSubscriptionMode() types.Optional[cache.SubscriptionMode]
		SubscriptionMode() cache.SubscriptionMode
	}

	// ShardedCacheAdapterOptions holds configuration for the sharded cache adapter.
	//
	//   - channelPrefix: Channel prefix for pub/sub. Default: "socket.io".
	//   - subscriptionMode: Channel allocation strategy. Default: DynamicSubscriptionMode.
	ShardedCacheAdapterOptions struct {
		channelPrefix    types.Optional[string]
		subscriptionMode types.Optional[cache.SubscriptionMode]
	}
)

// DefaultShardedCacheAdapterOptions returns a zero-valued ShardedCacheAdapterOptions.
func DefaultShardedCacheAdapterOptions() *ShardedCacheAdapterOptions {
	return &ShardedCacheAdapterOptions{}
}

// Assign copies non-nil fields from data into s.
func (s *ShardedCacheAdapterOptions) Assign(data ShardedCacheAdapterOptionsInterface) ShardedCacheAdapterOptionsInterface {
	if data == nil {
		return s
	}
	if data.GetRawChannelPrefix() != nil {
		s.SetChannelPrefix(data.ChannelPrefix())
	}
	if data.GetRawSubscriptionMode() != nil {
		s.SetSubscriptionMode(data.SubscriptionMode())
	}
	return s
}

// SetChannelPrefix sets the channel prefix.
func (s *ShardedCacheAdapterOptions) SetChannelPrefix(v string) {
	s.channelPrefix = types.NewSome(v)
}

// GetRawChannelPrefix returns the raw Optional for channelPrefix.
func (s *ShardedCacheAdapterOptions) GetRawChannelPrefix() types.Optional[string] {
	return s.channelPrefix
}

// ChannelPrefix returns the configured prefix or empty string if unset.
func (s *ShardedCacheAdapterOptions) ChannelPrefix() string {
	if s.channelPrefix == nil {
		return ""
	}
	return s.channelPrefix.Get()
}

// SetSubscriptionMode sets the subscription mode.
func (s *ShardedCacheAdapterOptions) SetSubscriptionMode(v cache.SubscriptionMode) {
	s.subscriptionMode = types.NewSome(v)
}

// GetRawSubscriptionMode returns the raw Optional for subscriptionMode.
func (s *ShardedCacheAdapterOptions) GetRawSubscriptionMode() types.Optional[cache.SubscriptionMode] {
	return s.subscriptionMode
}

// SubscriptionMode returns the configured mode or empty string if unset.
func (s *ShardedCacheAdapterOptions) SubscriptionMode() cache.SubscriptionMode {
	if s.subscriptionMode == nil {
		return ""
	}
	return s.subscriptionMode.Get()
}
