// Package adapter provides configuration options for the Redis-based Socket.IO adapter.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/redis/v3/emitter"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	// RedisAdapterOptionsInterface defines the interface for configuring RedisAdapterOptions.
	RedisAdapterOptionsInterface interface {
		emitter.EmitterOptionsInterface

		SetRequestsTimeout(time.Duration)
		GetRawRequestsTimeout() types.Optional[time.Duration]
		RequestsTimeout() time.Duration

		SetPublishOnSpecificResponseChannel(bool)
		GetRawPublishOnSpecificResponseChannel() types.Optional[bool]
		PublishOnSpecificResponseChannel() bool
	}

	// RedisAdapterOptions holds configuration for the Redis adapter.
	//
	// requestsTimeout: after this timeout the adapter will stop waiting for responses to a request (default: 5000ms).
	// publishOnSpecificResponseChannel: whether to publish a response to the channel specific to the requesting node (default: false).
	RedisAdapterOptions struct {
		emitter.EmitterOptions

		// requestsTimeout is the duration to wait for responses to a request.
		requestsTimeout types.Optional[time.Duration]

		// publishOnSpecificResponseChannel determines if responses are published to a node-specific channel.
		publishOnSpecificResponseChannel types.Optional[bool]
	}
)

// DefaultRedisAdapterOptions returns a new RedisAdapterOptions with default values.
func DefaultRedisAdapterOptions() *RedisAdapterOptions {
	return &RedisAdapterOptions{}
}

// Assign copies non-nil fields from another RedisAdapterOptionsInterface.
func (s *RedisAdapterOptions) Assign(data RedisAdapterOptionsInterface) RedisAdapterOptionsInterface {
	if data == nil {
		return s
	}

	s.EmitterOptions.Assign(data)

	if data.GetRawRequestsTimeout() != nil {
		s.SetRequestsTimeout(data.RequestsTimeout())
	}
	if data.GetRawPublishOnSpecificResponseChannel() != nil {
		s.SetPublishOnSpecificResponseChannel(data.PublishOnSpecificResponseChannel())
	}

	return s
}

// SetRequestsTimeout sets the requests timeout duration.
func (s *RedisAdapterOptions) SetRequestsTimeout(requestsTimeout time.Duration) {
	s.requestsTimeout = types.NewSome(requestsTimeout)
}
func (s *RedisAdapterOptions) GetRawRequestsTimeout() types.Optional[time.Duration] {
	return s.requestsTimeout
}
func (s *RedisAdapterOptions) RequestsTimeout() time.Duration {
	if s.requestsTimeout == nil {
		return 0
	}

	return s.requestsTimeout.Get()
}

// SetPublishOnSpecificResponseChannel sets whether to publish responses to a node-specific channel.
func (s *RedisAdapterOptions) SetPublishOnSpecificResponseChannel(publishOnSpecificResponseChannel bool) {
	s.publishOnSpecificResponseChannel = types.NewSome(publishOnSpecificResponseChannel)
}
func (s *RedisAdapterOptions) GetRawPublishOnSpecificResponseChannel() types.Optional[bool] {
	return s.publishOnSpecificResponseChannel
}
func (s *RedisAdapterOptions) PublishOnSpecificResponseChannel() bool {
	if s.publishOnSpecificResponseChannel == nil {
		return false
	}

	return s.publishOnSpecificResponseChannel.Get()
}
