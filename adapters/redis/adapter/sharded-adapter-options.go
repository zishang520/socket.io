// Package adapter provides configuration options for the Redis sharded Pub/Sub adapter for Socket.IO.
package adapter

type (
	subscriptionMode string

	// ShardedRedisAdapterOptionsInterface defines the interface for configuring ShardedRedisAdapterOptions.
	ShardedRedisAdapterOptionsInterface interface {
		SetChannelPrefix(string)
		GetRawChannelPrefix() *string
		ChannelPrefix() string

		SetSubscriptionMode(subscriptionMode)
		GetRawSubscriptionMode() *subscriptionMode
		SubscriptionMode() subscriptionMode
	}

	// ShardedRedisAdapterOptions holds configuration for the sharded Redis adapter.
	//
	// channelPrefix: the prefix for the Redis Pub/Sub channels (default: "socket.io").
	// subscriptionMode: impacts the number of Redis Pub/Sub channels (default: DynamicSubscriptionMode).
	ShardedRedisAdapterOptions struct {
		// channelPrefix is the prefix for the Redis Pub/Sub channels.
		channelPrefix *string

		// subscriptionMode determines the subscription mode for channel management.
		subscriptionMode *subscriptionMode
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
	s.channelPrefix = &channelPrefix
}

// GetRawChannelPrefix returns the raw channel prefix pointer.
func (s *ShardedRedisAdapterOptions) GetRawChannelPrefix() *string {
	return s.channelPrefix
}

// ChannelPrefix returns the channel prefix.
func (s *ShardedRedisAdapterOptions) ChannelPrefix() string {
	if s.channelPrefix == nil {
		return ""
	}

	return *s.channelPrefix
}

// SetSubscriptionMode sets the subscription mode.
func (s *ShardedRedisAdapterOptions) SetSubscriptionMode(subscriptionMode subscriptionMode) {
	s.subscriptionMode = &subscriptionMode
}

// GetRawSubscriptionMode returns the raw subscription mode pointer.
func (s *ShardedRedisAdapterOptions) GetRawSubscriptionMode() *subscriptionMode {
	return s.subscriptionMode
}

// SubscriptionMode returns the subscription mode.
func (s *ShardedRedisAdapterOptions) SubscriptionMode() subscriptionMode {
	if s.subscriptionMode == nil {
		return ""
	}

	return *s.subscriptionMode
}
