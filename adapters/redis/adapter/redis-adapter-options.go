// Package adapter provides configuration options for the Redis-based Socket.IO adapter.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/redis/v3/emitter"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Default configuration values for RedisAdapterOptions.
const (
	// DefaultRequestsTimeout is the default timeout for inter-node requests.
	DefaultRequestsTimeout = 5000 * time.Millisecond
)

type (
	// RedisAdapterOptionsInterface defines the interface for configuring RedisAdapterOptions.
	// It extends EmitterOptionsInterface to include adapter-specific settings.
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
	// Fields:
	//   - requestsTimeout: Maximum time to wait for responses to inter-node requests.
	//     Default: 5000ms. After this timeout, the adapter stops waiting for responses.
	//   - publishOnSpecificResponseChannel: When true, responses are published to a
	//     channel specific to the requesting node, reducing unnecessary message processing.
	//     Default: false.
	RedisAdapterOptions struct {
		emitter.EmitterOptions

		requestsTimeout                  types.Optional[time.Duration]
		publishOnSpecificResponseChannel types.Optional[bool]
	}
)

// DefaultRedisAdapterOptions returns a new RedisAdapterOptions with default values.
func DefaultRedisAdapterOptions() *RedisAdapterOptions {
	return &RedisAdapterOptions{}
}

// Assign copies non-nil fields from another RedisAdapterOptionsInterface.
// This method is useful for merging user-provided options with defaults.
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

// SetRequestsTimeout sets the timeout duration for inter-node requests.
func (s *RedisAdapterOptions) SetRequestsTimeout(requestsTimeout time.Duration) {
	s.requestsTimeout = types.NewSome(requestsTimeout)
}

// GetRawRequestsTimeout returns the raw Optional value for requestsTimeout.
// Returns nil if not explicitly set.
func (s *RedisAdapterOptions) GetRawRequestsTimeout() types.Optional[time.Duration] {
	return s.requestsTimeout
}

// RequestsTimeout returns the configured requests timeout.
// Returns 0 if not explicitly set; callers should use DefaultRequestsTimeout as fallback.
func (s *RedisAdapterOptions) RequestsTimeout() time.Duration {
	if s.requestsTimeout == nil {
		return 0
	}
	return s.requestsTimeout.Get()
}

// SetPublishOnSpecificResponseChannel sets whether responses should be published
// to a node-specific channel.
func (s *RedisAdapterOptions) SetPublishOnSpecificResponseChannel(publishOnSpecificResponseChannel bool) {
	s.publishOnSpecificResponseChannel = types.NewSome(publishOnSpecificResponseChannel)
}

// GetRawPublishOnSpecificResponseChannel returns the raw Optional value.
// Returns nil if not explicitly set.
func (s *RedisAdapterOptions) GetRawPublishOnSpecificResponseChannel() types.Optional[bool] {
	return s.publishOnSpecificResponseChannel
}

// PublishOnSpecificResponseChannel returns whether responses should be published
// to a node-specific channel. Returns false if not explicitly set.
func (s *RedisAdapterOptions) PublishOnSpecificResponseChannel() bool {
	if s.publishOnSpecificResponseChannel == nil {
		return false
	}
	return s.publishOnSpecificResponseChannel.Get()
}
