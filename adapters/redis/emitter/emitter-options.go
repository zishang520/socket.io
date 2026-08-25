// Package emitter provides an API for broadcasting messages to Socket.IO servers via Redis
// without requiring a full Socket.IO server instance.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

const (
	// DefaultEmitterKey is the default Redis key prefix for the emitter.
	DefaultEmitterKey = "socket.io"
)

type (
	// EmitterOptionsInterface defines the interface for configuring emitter options.
	// It provides getters and setters for all configurable options.
	EmitterOptionsInterface interface {
		// SetKey sets the Redis key prefix for channel names.
		SetKey(string)
		// GetRawKey returns the raw Optional wrapper for the key setting.
		GetRawKey() types.Optional[string]
		// Key returns the Redis key prefix, or empty string if not set.
		Key() string

		// SetEncoder sets the encoder for outbound messages.
		SetEncoder(redis.Encoder)
		// GetRawEncoder returns the raw Optional wrapper for the encoder setting.
		GetRawEncoder() types.Optional[redis.Encoder]
		// Encoder returns the encoder, or nil if not set.
		Encoder() redis.Encoder

		// SetSharded enables or disables Redis sharded Pub/Sub.
		// When enabled, uses SPUBLISH for Redis Cluster sharded Pub/Sub (Redis 7.0+).
		SetSharded(bool)
		// GetRawSharded returns the raw Optional wrapper for the sharded setting.
		GetRawSharded() types.Optional[bool]
		// Sharded returns whether sharded Pub/Sub is enabled.
		Sharded() bool

		// SetSubscriptionMode sets the subscription mode for sharded Pub/Sub.
		// This should match the adapter's subscriptionMode setting.
		SetSubscriptionMode(redis.SubscriptionMode)
		// GetRawSubscriptionMode returns the raw Optional wrapper for the subscriptionMode setting.
		GetRawSubscriptionMode() types.Optional[redis.SubscriptionMode]
		// SubscriptionMode returns the subscription mode.
		SubscriptionMode() redis.SubscriptionMode
	}

	// EmitterOptions holds configuration options for the Redis emitter.
	// All fields are optional and will use default values if not explicitly set.
	EmitterOptions struct {
		// key is the Redis key prefix used for constructing channel names.
		// Default: "socket.io"
		key types.Optional[string]

		// encoder serializes outbound messages sent to Redis.
		// Default: MessagePack encoder
		encoder types.Optional[redis.Encoder]

		// sharded enables Redis sharded Pub/Sub (SPUBLISH) for Redis Cluster mode.
		// Set to true when using Redis Cluster with sharded Pub/Sub (Redis 7.0+).
		// Default: false
		sharded types.Optional[bool]

		// subscriptionMode controls how room-specific channels are computed.
		// This should match the adapter's subscriptionMode setting.
		// Default: DynamicSubscriptionMode
		subscriptionMode types.Optional[redis.SubscriptionMode]
	}
)

// DefaultEmitterOptions returns raw-empty options; Emitter.Construct applies defaults.
func DefaultEmitterOptions() *EmitterOptions {
	return &EmitterOptions{}
}

// Assign copies non-nil option values from another EmitterOptionsInterface.
// This allows merging configuration from multiple sources.
func (o *EmitterOptions) Assign(data EmitterOptionsInterface) EmitterOptionsInterface {
	if utils.IsNil(data) {
		return o
	}

	if data.GetRawKey() != nil {
		o.SetKey(data.Key())
	}
	if data.GetRawEncoder() != nil {
		o.SetEncoder(data.Encoder())
	}
	if data.GetRawSharded() != nil {
		o.SetSharded(data.Sharded())
	}
	if data.GetRawSubscriptionMode() != nil {
		o.SetSubscriptionMode(data.SubscriptionMode())
	}

	return o
}

// SetKey sets the Redis key prefix for channel names.
func (o *EmitterOptions) SetKey(key string) {
	o.key = types.NewSome(key)
}

// GetRawKey returns the raw Optional wrapper for the key setting.
func (o *EmitterOptions) GetRawKey() types.Optional[string] {
	return o.key
}

// Key returns the Redis key prefix, or empty string if not set.
func (o *EmitterOptions) Key() string {
	if o.key == nil {
		return ""
	}
	return o.key.Get()
}

// SetEncoder sets the encoder for messages sent to Redis.
func (o *EmitterOptions) SetEncoder(encoder redis.Encoder) {
	o.encoder = types.NewSome(encoder)
}

// GetRawEncoder returns the raw Optional wrapper for the encoder setting.
func (o *EmitterOptions) GetRawEncoder() types.Optional[redis.Encoder] {
	return o.encoder
}

// Encoder returns the configured encoder, or nil if not set.
func (o *EmitterOptions) Encoder() redis.Encoder {
	if o.encoder == nil {
		return nil
	}
	return o.encoder.Get()
}

// SetSharded enables or disables Redis sharded Pub/Sub.
// When true, uses SPUBLISH command for Redis Cluster sharded Pub/Sub (Redis 7.0+).
func (o *EmitterOptions) SetSharded(sharded bool) {
	o.sharded = types.NewSome(sharded)
}

// GetRawSharded returns the raw Optional wrapper for the sharded setting.
func (o *EmitterOptions) GetRawSharded() types.Optional[bool] {
	return o.sharded
}

// Sharded returns whether sharded Pub/Sub is enabled.
// Returns false if not set.
func (o *EmitterOptions) Sharded() bool {
	if o.sharded == nil {
		return false
	}
	return o.sharded.Get()
}

// SetSubscriptionMode sets the subscription mode for sharded Pub/Sub.
// This should match the adapter's subscriptionMode setting.
func (o *EmitterOptions) SetSubscriptionMode(mode redis.SubscriptionMode) {
	o.subscriptionMode = types.NewSome(mode)
}

// GetRawSubscriptionMode returns the raw Optional wrapper for the subscriptionMode setting.
func (o *EmitterOptions) GetRawSubscriptionMode() types.Optional[redis.SubscriptionMode] {
	return o.subscriptionMode
}

// SubscriptionMode returns the subscription mode.
// Returns DynamicSubscriptionMode if not set.
func (o *EmitterOptions) SubscriptionMode() redis.SubscriptionMode {
	if o.subscriptionMode == nil {
		return redis.DynamicSubscriptionMode
	}
	return o.subscriptionMode.Get()
}
