// Package emitter provides an API for broadcasting messages to Socket.IO servers via Valkey
// without requiring a full Socket.IO server instance.
package emitter

import (
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
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

		SetParser(valkey.Parser)
		GetRawParser() types.Optional[valkey.Parser]
		Parser() valkey.Parser

		SetSharded(bool)
		GetRawSharded() types.Optional[bool]
		Sharded() bool

		SetSubscriptionMode(valkey.SubscriptionMode)
		GetRawSubscriptionMode() types.Optional[valkey.SubscriptionMode]
		SubscriptionMode() valkey.SubscriptionMode
	}

	// EmitterOptions holds configuration options for the Valkey emitter.
	EmitterOptions struct {
		key              types.Optional[string]
		parser           types.Optional[valkey.Parser]
		sharded          types.Optional[bool]
		subscriptionMode types.Optional[valkey.SubscriptionMode]
	}
)

// DefaultEmitterOptions creates a new EmitterOptions instance with default values.
func DefaultEmitterOptions() *EmitterOptions {
	return &EmitterOptions{}
}

// Assign copies non-nil option values from another EmitterOptionsInterface.
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

func (o *EmitterOptions) SetKey(key string)     { o.key = types.NewSome(key) }
func (o *EmitterOptions) GetRawKey() types.Optional[string] { return o.key }
func (o *EmitterOptions) Key() string {
	if o.key == nil {
		return ""
	}
	return o.key.Get()
}

func (o *EmitterOptions) SetParser(parser valkey.Parser) { o.parser = types.NewSome(parser) }
func (o *EmitterOptions) GetRawParser() types.Optional[valkey.Parser] { return o.parser }
func (o *EmitterOptions) Parser() valkey.Parser {
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
