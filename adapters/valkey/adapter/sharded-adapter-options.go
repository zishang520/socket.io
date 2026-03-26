// Package adapter provides configuration options for the Valkey sharded Pub/Sub adapter for Socket.IO.
package adapter

import (
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	// DefaultShardedChannelPrefix is the default channel prefix for sharded Valkey adapter.
	DefaultShardedChannelPrefix = "socket.io"
)

// DefaultShardedSubscriptionMode is the default subscription mode for the sharded adapter.
var DefaultShardedSubscriptionMode = valkey.DynamicSubscriptionMode

type (
	// ShardedValkeyAdapterOptionsInterface defines the interface for configuring ShardedValkeyAdapterOptions.
	ShardedValkeyAdapterOptionsInterface interface {
		SetChannelPrefix(string)
		GetRawChannelPrefix() types.Optional[string]
		ChannelPrefix() string

		SetSubscriptionMode(valkey.SubscriptionMode)
		GetRawSubscriptionMode() types.Optional[valkey.SubscriptionMode]
		SubscriptionMode() valkey.SubscriptionMode
	}

	// ShardedValkeyAdapterOptions holds configuration for the sharded Valkey adapter.
	ShardedValkeyAdapterOptions struct {
		channelPrefix    types.Optional[string]
		subscriptionMode types.Optional[valkey.SubscriptionMode]
	}
)

// DefaultShardedValkeyAdapterOptions returns a new ShardedValkeyAdapterOptions with default values.
func DefaultShardedValkeyAdapterOptions() *ShardedValkeyAdapterOptions {
	return &ShardedValkeyAdapterOptions{}
}

// Assign copies non-nil fields from another ShardedValkeyAdapterOptionsInterface.
func (s *ShardedValkeyAdapterOptions) Assign(data ShardedValkeyAdapterOptionsInterface) ShardedValkeyAdapterOptionsInterface {
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

func (s *ShardedValkeyAdapterOptions) SetChannelPrefix(v string) { s.channelPrefix = types.NewSome(v) }
func (s *ShardedValkeyAdapterOptions) GetRawChannelPrefix() types.Optional[string] {
	return s.channelPrefix
}
func (s *ShardedValkeyAdapterOptions) ChannelPrefix() string {
	if s.channelPrefix == nil {
		return ""
	}
	return s.channelPrefix.Get()
}

func (s *ShardedValkeyAdapterOptions) SetSubscriptionMode(v valkey.SubscriptionMode) {
	s.subscriptionMode = types.NewSome(v)
}
func (s *ShardedValkeyAdapterOptions) GetRawSubscriptionMode() types.Optional[valkey.SubscriptionMode] {
	return s.subscriptionMode
}
func (s *ShardedValkeyAdapterOptions) SubscriptionMode() valkey.SubscriptionMode {
	if s.subscriptionMode == nil {
		return ""
	}
	return s.subscriptionMode.Get()
}
