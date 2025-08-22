// Package adapter provides configuration options for the Redis sharded Pub/Sub adapter for Socket.IO.
package adapter

import "github.com/zishang520/socket.io/v3/pkg/types"

type (
	subscriptionMode string

	// ShardedRedisAdapterOptionsInterface defines the interface for configuring ShardedRedisAdapterOptions.
	ShardedRedisAdapterOptionsInterface interface {
		SetChannelPrefix(string)
		GetRawChannelPrefix() types.Optional[string]
		ChannelPrefix() string

		SetSubscriptionMode(subscriptionMode)
		GetRawSubscriptionMode() types.Optional[subscriptionMode]
		SubscriptionMode() subscriptionMode
	}

	// ShardedRedisAdapterOptions holds configuration for the sharded Redis adapter.
	//
	// channelPrefix: the prefix for the Redis Pub/Sub channels (default: "socket.io").
	// subscriptionMode: impacts the number of Redis Pub/Sub channels (default: DynamicSubscriptionMode).
	ShardedRedisAdapterOptions struct {
		// channelPrefix is the prefix for the Redis Pub/Sub channels.
		channelPrefix types.Optional[string]

		// subscriptionMode determines the subscription mode for channel management.
		subscriptionMode types.Optional[subscriptionMode]
	}
)

const (
	// StaticSubscriptionMode uses 2 channels per namespace.
	StaticSubscriptionMode subscriptionMode = "static"
	// DynamicSubscriptionMode uses 2 + 1 per public room channels per namespace.
	DynamicSubscriptionMode subscriptionMode = "dynamic"
	// DynamicPrivateSubscriptionMode creates separate channels for private rooms as well.
	DynamicPrivateSubscriptionMode subscriptionMode = "dynamic-private"
)

// DefaultShardedRedisAdapterOptions returns a new ShardedRedisAdapterOptions with default values.
func DefaultShardedRedisAdapterOptions() *ShardedRedisAdapterOptions {
	return &ShardedRedisAdapterOptions{}
}

// Assign copies non-nil fields from another ShardedRedisAdapterOptionsInterface.
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

// SetChannelPrefix sets the channel prefix.
func (s *ShardedRedisAdapterOptions) SetChannelPrefix(channelPrefix string) {
	s.channelPrefix = types.NewSome(channelPrefix)
}
func (s *ShardedRedisAdapterOptions) GetRawChannelPrefix() types.Optional[string] {
	return s.channelPrefix
}
func (s *ShardedRedisAdapterOptions) ChannelPrefix() string {
	if s.channelPrefix == nil {
		return ""
	}

	return s.channelPrefix.Get()
}

// SetSubscriptionMode sets the subscription mode.
func (s *ShardedRedisAdapterOptions) SetSubscriptionMode(subscriptionMode subscriptionMode) {
	s.subscriptionMode = types.NewSome(subscriptionMode)
}
func (s *ShardedRedisAdapterOptions) GetRawSubscriptionMode() types.Optional[subscriptionMode] {
	return s.subscriptionMode
}
func (s *ShardedRedisAdapterOptions) SubscriptionMode() subscriptionMode {
	if s.subscriptionMode == nil {
		return ""
	}

	return s.subscriptionMode.Get()
}
