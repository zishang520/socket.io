// Package adapter provides configuration options for the Redis sharded Pub/Sub adapter for Socket.IO.
// The sharded adapter leverages Redis 7.0's sharded Pub/Sub for improved scalability.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Default configuration values for ShardedRedisAdapterOptions.
const (
	DefaultShardedChannelPrefix = "socket.io"
)

// DefaultShardedSubscriptionMode is the default subscription mode for the sharded adapter.
var DefaultShardedSubscriptionMode = redis.DynamicSubscriptionMode

type (
	// ShardedRedisAdapterOptionsInterface defines the interface for configuring ShardedRedisAdapterOptions.
	ShardedRedisAdapterOptionsInterface interface {
		SetChannelPrefix(string)
		GetRawChannelPrefix() types.Optional[string]
		ChannelPrefix() string

		SetSubscriptionMode(redis.SubscriptionMode)
		GetRawSubscriptionMode() types.Optional[redis.SubscriptionMode]
		SubscriptionMode() redis.SubscriptionMode
	}

	// ShardedRedisAdapterOptions holds configuration for the sharded Redis adapter.
	//
	// Fields:
	//   - channelPrefix: The prefix for Redis Pub/Sub channels. Default: "socket.io".
	//   - subscriptionMode: Determines the channel management strategy. Default: DynamicSubscriptionMode.
	ShardedRedisAdapterOptions struct {
		channelPrefix    types.Optional[string]
		subscriptionMode types.Optional[redis.SubscriptionMode]
	}
)

// DefaultShardedRedisAdapterOptions returns a new ShardedRedisAdapterOptions with default values.
func DefaultShardedRedisAdapterOptions() *ShardedRedisAdapterOptions {
	return &ShardedRedisAdapterOptions{}
}

// Assign copies non-nil fields from another ShardedRedisAdapterOptionsInterface.
// This method is useful for merging user-provided options with defaults.
func (s *ShardedRedisAdapterOptions) Assign(data ShardedRedisAdapterOptionsInterface) ShardedRedisAdapterOptionsInterface {
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

// SetChannelPrefix sets the channel prefix for Redis Pub/Sub.
func (s *ShardedRedisAdapterOptions) SetChannelPrefix(channelPrefix string) {
	s.channelPrefix = types.NewSome(channelPrefix)
}

// GetRawChannelPrefix returns the raw Optional value for channelPrefix.
func (s *ShardedRedisAdapterOptions) GetRawChannelPrefix() types.Optional[string] {
	return s.channelPrefix
}

// ChannelPrefix returns the configured channel prefix.
// Returns empty string if not set; callers should use DefaultShardedChannelPrefix as fallback.
func (s *ShardedRedisAdapterOptions) ChannelPrefix() string {
	if s.channelPrefix == nil {
		return ""
	}
	return s.channelPrefix.Get()
}

// SetSubscriptionMode sets the subscription mode for channel management.
func (s *ShardedRedisAdapterOptions) SetSubscriptionMode(subscriptionMode redis.SubscriptionMode) {
	s.subscriptionMode = types.NewSome(subscriptionMode)
}

// GetRawSubscriptionMode returns the raw Optional value for subscriptionMode.
func (s *ShardedRedisAdapterOptions) GetRawSubscriptionMode() types.Optional[redis.SubscriptionMode] {
	return s.subscriptionMode
}

// SubscriptionMode returns the configured subscription mode.
// Returns empty string if not set; callers should use DefaultShardedSubscriptionMode as fallback.
func (s *ShardedRedisAdapterOptions) SubscriptionMode() redis.SubscriptionMode {
	if s.subscriptionMode == nil {
		return ""
	}
	return s.subscriptionMode.Get()
}
