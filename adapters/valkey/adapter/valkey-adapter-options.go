// Package adapter provides configuration options for the Valkey-based Socket.IO adapter.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

const (
	// DefaultRequestsTimeout is the default timeout for inter-node requests.
	DefaultRequestsTimeout = 5000 * time.Millisecond
)

type (
	// ValkeyAdapterOptionsInterface defines the classic Valkey adapter settings.
	ValkeyAdapterOptionsInterface interface {
		SetKey(string)
		GetRawKey() types.Optional[string]
		Key() string

		SetParser(valkey.Parser)
		GetRawParser() types.Optional[valkey.Parser]
		Parser() valkey.Parser

		SetRequestsTimeout(time.Duration)
		GetRawRequestsTimeout() types.Optional[time.Duration]
		RequestsTimeout() time.Duration

		SetPublishOnSpecificResponseChannel(bool)
		GetRawPublishOnSpecificResponseChannel() types.Optional[bool]
		PublishOnSpecificResponseChannel() bool
	}

	// ValkeyAdapterOptions holds configuration for the classic Valkey adapter.
	ValkeyAdapterOptions struct {
		key                              types.Optional[string]
		parser                           types.Optional[valkey.Parser]
		requestsTimeout                  types.Optional[time.Duration]
		publishOnSpecificResponseChannel types.Optional[bool]
	}
)

// DefaultValkeyAdapterOptions returns raw-empty options; ValkeyAdapter.Construct applies defaults.
func DefaultValkeyAdapterOptions() *ValkeyAdapterOptions {
	return &ValkeyAdapterOptions{}
}

// Assign copies explicitly configured values from data.
func (s *ValkeyAdapterOptions) Assign(data ValkeyAdapterOptionsInterface) ValkeyAdapterOptionsInterface {
	if utils.IsNil(data) {
		return s
	}
	if data.GetRawKey() != nil {
		s.SetKey(data.Key())
	}
	if data.GetRawParser() != nil {
		s.SetParser(data.Parser())
	}
	if data.GetRawRequestsTimeout() != nil {
		s.SetRequestsTimeout(data.RequestsTimeout())
	}
	if data.GetRawPublishOnSpecificResponseChannel() != nil {
		s.SetPublishOnSpecificResponseChannel(data.PublishOnSpecificResponseChannel())
	}
	return s
}

func (s *ValkeyAdapterOptions) SetKey(key string) {
	s.key = types.NewSome(key)
}

func (s *ValkeyAdapterOptions) GetRawKey() types.Optional[string] {
	return s.key
}

func (s *ValkeyAdapterOptions) Key() string {
	if s.key == nil {
		return ""
	}
	return s.key.Get()
}

func (s *ValkeyAdapterOptions) SetParser(parser valkey.Parser) {
	s.parser = types.NewSome(parser)
}

func (s *ValkeyAdapterOptions) GetRawParser() types.Optional[valkey.Parser] {
	return s.parser
}

func (s *ValkeyAdapterOptions) Parser() valkey.Parser {
	if s.parser == nil {
		return nil
	}
	return s.parser.Get()
}

func (s *ValkeyAdapterOptions) SetRequestsTimeout(requestsTimeout time.Duration) {
	s.requestsTimeout = types.NewSome(requestsTimeout)
}

func (s *ValkeyAdapterOptions) GetRawRequestsTimeout() types.Optional[time.Duration] {
	return s.requestsTimeout
}

func (s *ValkeyAdapterOptions) RequestsTimeout() time.Duration {
	if s.requestsTimeout == nil {
		return 0
	}
	return s.requestsTimeout.Get()
}

func (s *ValkeyAdapterOptions) SetPublishOnSpecificResponseChannel(value bool) {
	s.publishOnSpecificResponseChannel = types.NewSome(value)
}

func (s *ValkeyAdapterOptions) GetRawPublishOnSpecificResponseChannel() types.Optional[bool] {
	return s.publishOnSpecificResponseChannel
}

func (s *ValkeyAdapterOptions) PublishOnSpecificResponseChannel() bool {
	if s.publishOnSpecificResponseChannel == nil {
		return false
	}
	return s.publishOnSpecificResponseChannel.Get()
}
