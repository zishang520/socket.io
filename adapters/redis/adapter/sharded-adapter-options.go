package adapter

type (
	subscriptionMode string

	ShardedRedisAdapterOptionsInterface interface {
		SetChannelPrefix(string)
		GetRawChannelPrefix() *string
		ChannelPrefix() string

		SetSubscriptionMode(subscriptionMode)
		GetRawSubscriptionMode() *subscriptionMode
		SubscriptionMode() subscriptionMode
	}

	ShardedRedisAdapterOptions struct {
		// The prefix for the Redis Pub/Sub channels.
		//
		// Default: "socket.io"
		channelPrefix *string

		// The subscription mode impacts the number of Redis Pub/Sub channels:
		//
		// - [StaticSubscriptionMode]: 2 channels per namespace
		//
		// Useful when used with dynamic namespaces.
		//
		// - [DynamicSubscriptionMode]: (2 + 1 per public room) channels per namespace
		//
		// The default value, useful when some rooms have a low number of clients (so only a few Socket.IO servers are notified).
		//
		// Only public rooms (i.e. not related to a particular Socket ID) are taken in account, because:
		// - a lot of connected clients would mean a lot of subscription/unsubscription
		// - the Socket ID attribute is ephemeral
		//
		// - [DynamicPrivateSubscriptionMode]
		//
		// Like [DynamicPrivateSubscriptionMode] but creates separate channels for private rooms as well. Useful when there is lots of 1:1 communication
		// via socket.Emit() calls.
		//
		// Default: [DynamicSubscriptionMode]
		subscriptionMode *subscriptionMode
	}
)

const (
	StaticSubscriptionMode         subscriptionMode = "static"
	DynamicSubscriptionMode        subscriptionMode = "dynamic"
	DynamicPrivateSubscriptionMode subscriptionMode = "dynamic-private"
)

func DefaultShardedRedisAdapterOptions() *ShardedRedisAdapterOptions {
	return &ShardedRedisAdapterOptions{}
}

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

func (s *ShardedRedisAdapterOptions) SetChannelPrefix(channelPrefix string) {
	s.channelPrefix = &channelPrefix
}
func (s *ShardedRedisAdapterOptions) GetRawChannelPrefix() *string {
	return s.channelPrefix
}
func (s *ShardedRedisAdapterOptions) ChannelPrefix() string {
	if s.channelPrefix == nil {
		return ""
	}

	return *s.channelPrefix
}

func (s *ShardedRedisAdapterOptions) SetSubscriptionMode(subscriptionMode subscriptionMode) {
	s.subscriptionMode = &subscriptionMode
}
func (s *ShardedRedisAdapterOptions) GetRawSubscriptionMode() *subscriptionMode {
	return s.subscriptionMode
}
func (s *ShardedRedisAdapterOptions) SubscriptionMode() subscriptionMode {
	if s.subscriptionMode == nil {
		return ""
	}

	return *s.subscriptionMode
}
