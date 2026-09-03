// Package emitter provides an API for broadcasting messages to Socket.IO servers via Valkey
// without requiring a full Socket.IO server instance.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

const (
	// DefaultEmitterKey is the default Valkey key prefix for the emitter.
	DefaultEmitterKey = "socket.io"
)

type (
	// EmitterOptionsInterface defines the interface for configuring emitter options.
	EmitterOptionsInterface interface {
		SetKey(string)
		GetRawKey() types.Optional[string]
		Key() string

		SetEncoder(valkey.Encoder)
		GetRawEncoder() types.Optional[valkey.Encoder]
		Encoder() valkey.Encoder

		SetSharded(bool)
		GetRawSharded() types.Optional[bool]
		Sharded() bool

		SetSubscriptionMode(valkey.SubscriptionMode)
		GetRawSubscriptionMode() types.Optional[valkey.SubscriptionMode]
		SubscriptionMode() valkey.SubscriptionMode
	}

	// EmitterOptions holds optional Valkey emitter settings.
	EmitterOptions struct {
		key              types.Optional[string]
		encoder          types.Optional[valkey.Encoder]
		sharded          types.Optional[bool]
		subscriptionMode types.Optional[valkey.SubscriptionMode]
	}
)

// DefaultEmitterOptions returns raw-empty options; Emitter.Construct applies defaults.
func DefaultEmitterOptions() *EmitterOptions {
	return new(EmitterOptions)
}

// Assign copies explicitly set values from data.
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

func (o *EmitterOptions) SetKey(key string)                 { o.key = types.NewSome(key) }
func (o *EmitterOptions) GetRawKey() types.Optional[string] { return o.key }
func (o *EmitterOptions) Key() string {
	if o.key == nil {
		return ""
	}
	return o.key.Get()
}

func (o *EmitterOptions) SetEncoder(encoder valkey.Encoder) {
	o.encoder = types.NewSome(encoder)
}
func (o *EmitterOptions) GetRawEncoder() types.Optional[valkey.Encoder] { return o.encoder }
func (o *EmitterOptions) Encoder() valkey.Encoder {
	if o.encoder == nil {
		return nil
	}
	return o.encoder.Get()
}

func (o *EmitterOptions) SetSharded(sharded bool)             { o.sharded = types.NewSome(sharded) }
func (o *EmitterOptions) GetRawSharded() types.Optional[bool] { return o.sharded }
func (o *EmitterOptions) Sharded() bool {
	if o.sharded == nil {
		return false
	}
	return o.sharded.Get()
}

func (o *EmitterOptions) SetSubscriptionMode(mode valkey.SubscriptionMode) {
	o.subscriptionMode = types.NewSome(mode)
}
func (o *EmitterOptions) GetRawSubscriptionMode() types.Optional[valkey.SubscriptionMode] {
	return o.subscriptionMode
}
func (o *EmitterOptions) SubscriptionMode() valkey.SubscriptionMode {
	if o.subscriptionMode == nil {
		return valkey.DynamicSubscriptionMode
	}
	return o.subscriptionMode.Get()
}
