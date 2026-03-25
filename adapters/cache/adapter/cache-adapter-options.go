// Package adapter provides configuration options for the cache-based Socket.IO adapter.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/cache/v3/emitter"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// DefaultRequestsTimeout is the default timeout for inter-node requests.
const DefaultRequestsTimeout = 5000 * time.Millisecond

type (
	// CacheAdapterOptionsInterface is the configuration interface for CacheAdapterOptions.
	// It extends EmitterOptionsInterface with adapter-specific settings.
	CacheAdapterOptionsInterface interface {
		emitter.EmitterOptionsInterface

		SetRequestsTimeout(time.Duration)
		GetRawRequestsTimeout() types.Optional[time.Duration]
		RequestsTimeout() time.Duration

		SetPublishOnSpecificResponseChannel(bool)
		GetRawPublishOnSpecificResponseChannel() types.Optional[bool]
		PublishOnSpecificResponseChannel() bool
	}

	// CacheAdapterOptions holds configuration for the cache adapter.
	//
	//   - requestsTimeout: Maximum wait time for inter-node responses. Default: 5000ms.
	//   - publishOnSpecificResponseChannel: When true, responses are published on a
	//     node-specific channel, reducing unnecessary processing by other nodes.
	CacheAdapterOptions struct {
		emitter.EmitterOptions

		requestsTimeout                  types.Optional[time.Duration]
		publishOnSpecificResponseChannel types.Optional[bool]
	}
)

// DefaultCacheAdapterOptions returns a new CacheAdapterOptions with zero values.
func DefaultCacheAdapterOptions() *CacheAdapterOptions {
	return &CacheAdapterOptions{}
}

// Assign copies non-nil fields from data into s.
func (s *CacheAdapterOptions) Assign(data CacheAdapterOptionsInterface) CacheAdapterOptionsInterface {
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

// SetRequestsTimeout sets the inter-node request timeout.
func (s *CacheAdapterOptions) SetRequestsTimeout(d time.Duration) {
	s.requestsTimeout = types.NewSome(d)
}

// GetRawRequestsTimeout returns the raw Optional for requestsTimeout.
func (s *CacheAdapterOptions) GetRawRequestsTimeout() types.Optional[time.Duration] {
	return s.requestsTimeout
}

// RequestsTimeout returns the configured timeout or zero if unset.
func (s *CacheAdapterOptions) RequestsTimeout() time.Duration {
	if s.requestsTimeout == nil {
		return 0
	}
	return s.requestsTimeout.Get()
}

// SetPublishOnSpecificResponseChannel sets whether to use per-node response channels.
func (s *CacheAdapterOptions) SetPublishOnSpecificResponseChannel(v bool) {
	s.publishOnSpecificResponseChannel = types.NewSome(v)
}

// GetRawPublishOnSpecificResponseChannel returns the raw Optional.
func (s *CacheAdapterOptions) GetRawPublishOnSpecificResponseChannel() types.Optional[bool] {
	return s.publishOnSpecificResponseChannel
}

// PublishOnSpecificResponseChannel returns the configured value or false if unset.
func (s *CacheAdapterOptions) PublishOnSpecificResponseChannel() bool {
	if s.publishOnSpecificResponseChannel == nil {
		return false
	}
	return s.publishOnSpecificResponseChannel.Get()
}
