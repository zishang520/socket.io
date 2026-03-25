// Package emitter provides an API for broadcasting messages to Socket.IO servers
// via the cache pub/sub layer without requiring a full Socket.IO server instance.
package emitter

import (
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	// DefaultEmitterKey is the default key prefix used for channel names.
	DefaultEmitterKey = "socket.io"
)

type (
	// EmitterOptionsInterface is the configuration interface for emitter options.
	EmitterOptionsInterface interface {
		SetKey(string)
		GetRawKey() types.Optional[string]
		Key() string

		SetParser(cache.Parser)
		GetRawParser() types.Optional[cache.Parser]
		Parser() cache.Parser

		SetSharded(bool)
		GetRawSharded() types.Optional[bool]
		Sharded() bool

		SetSubscriptionMode(cache.SubscriptionMode)
		GetRawSubscriptionMode() types.Optional[cache.SubscriptionMode]
		SubscriptionMode() cache.SubscriptionMode
	}

	// EmitterOptions holds configuration options for the cache emitter.
	EmitterOptions struct {
		key              types.Optional[string]
		parser           types.Optional[cache.Parser]
		sharded          types.Optional[bool]
		subscriptionMode types.Optional[cache.SubscriptionMode]
	}
)

// DefaultEmitterOptions returns a zero-valued EmitterOptions.
func DefaultEmitterOptions() *EmitterOptions {
	return &EmitterOptions{}
}

// Assign copies non-nil option values from data into o.
func (o *EmitterOptions) Assign(data EmitterOptionsInterface) EmitterOptionsInterface {
	if data == nil {
		return o
	}
	if data.GetRawKey() != nil {
		o.SetKey(data.Key())
	}
	if data.Parser() != nil {
		o.SetParser(data.Parser())
	}
	if data.GetRawSharded() != nil {
		o.SetSharded(data.Sharded())
	}
	if data.GetRawSubscriptionMode() != nil {
		o.SetSubscriptionMode(data.SubscriptionMode())
	}
	return o
}

func (o *EmitterOptions) SetKey(key string) { o.key = types.NewSome(key) }
func (o *EmitterOptions) GetRawKey() types.Optional[string] { return o.key }
func (o *EmitterOptions) Key() string {
	if o.key == nil {
		return ""
	}
	return o.key.Get()
}

func (o *EmitterOptions) SetParser(parser cache.Parser) { o.parser = types.NewSome(parser) }
func (o *EmitterOptions) GetRawParser() types.Optional[cache.Parser] { return o.parser }
func (o *EmitterOptions) Parser() cache.Parser {
	if o.parser == nil {
		return nil
	}
	return o.parser.Get()
}

func (o *EmitterOptions) SetSharded(sharded bool) { o.sharded = types.NewSome(sharded) }
func (o *EmitterOptions) GetRawSharded() types.Optional[bool] { return o.sharded }
func (o *EmitterOptions) Sharded() bool {
	if o.sharded == nil {
		return false
	}
	return o.sharded.Get()
}

func (o *EmitterOptions) SetSubscriptionMode(mode cache.SubscriptionMode) {
	o.subscriptionMode = types.NewSome(mode)
}
func (o *EmitterOptions) GetRawSubscriptionMode() types.Optional[cache.SubscriptionMode] {
	return o.subscriptionMode
}
func (o *EmitterOptions) SubscriptionMode() cache.SubscriptionMode {
	if o.subscriptionMode == nil {
		return cache.DynamicSubscriptionMode
	}
	return o.subscriptionMode.Get()
}
