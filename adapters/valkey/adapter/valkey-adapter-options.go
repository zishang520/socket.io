// Package adapter provides configuration options for the Valkey-based Socket.IO adapter.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/valkey/v3/emitter"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// DefaultRequestsTimeout is the default timeout for inter-node requests.
const DefaultRequestsTimeout = 5000 * time.Millisecond

type (
	// ValkeyAdapterOptionsInterface defines the interface for configuring ValkeyAdapterOptions.
	ValkeyAdapterOptionsInterface interface {
		emitter.EmitterOptionsInterface

		SetRequestsTimeout(time.Duration)
		GetRawRequestsTimeout() types.Optional[time.Duration]
		RequestsTimeout() time.Duration

		SetPublishOnSpecificResponseChannel(bool)
		GetRawPublishOnSpecificResponseChannel() types.Optional[bool]
		PublishOnSpecificResponseChannel() bool
	}

	// ValkeyAdapterOptions holds configuration for the Valkey adapter.
	//
	// Fields:
	//   - requestsTimeout: Maximum time to wait for responses to inter-node requests.
	//     Default: 5000ms.
	//   - publishOnSpecificResponseChannel: When true, responses are published to a
	//     channel specific to the requesting node.
	ValkeyAdapterOptions struct {
		emitter.EmitterOptions

		requestsTimeout                  types.Optional[time.Duration]
		publishOnSpecificResponseChannel types.Optional[bool]
	}
)

// DefaultValkeyAdapterOptions returns a new ValkeyAdapterOptions with default values.
func DefaultValkeyAdapterOptions() *ValkeyAdapterOptions {
	return &ValkeyAdapterOptions{}
}

// Assign copies non-nil fields from another ValkeyAdapterOptionsInterface.
func (s *ValkeyAdapterOptions) Assign(data ValkeyAdapterOptionsInterface) ValkeyAdapterOptionsInterface {
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
func (s *ValkeyAdapterOptions) SetRequestsTimeout(requestsTimeout time.Duration) {
	s.requestsTimeout = types.NewSome(requestsTimeout)
}

// GetRawRequestsTimeout returns the raw Optional value for requestsTimeout.
func (s *ValkeyAdapterOptions) GetRawRequestsTimeout() types.Optional[time.Duration] {
	return s.requestsTimeout
}

// RequestsTimeout returns the configured requests timeout.
func (s *ValkeyAdapterOptions) RequestsTimeout() time.Duration {
	if s.requestsTimeout == nil {
		return 0
	}
	return s.requestsTimeout.Get()
}

// SetPublishOnSpecificResponseChannel sets whether responses are published to node-specific channels.
func (s *ValkeyAdapterOptions) SetPublishOnSpecificResponseChannel(v bool) {
	s.publishOnSpecificResponseChannel = types.NewSome(v)
}

// GetRawPublishOnSpecificResponseChannel returns the raw Optional value.
func (s *ValkeyAdapterOptions) GetRawPublishOnSpecificResponseChannel() types.Optional[bool] {
	return s.publishOnSpecificResponseChannel
}

// PublishOnSpecificResponseChannel returns whether responses are published to node-specific channels.
func (s *ValkeyAdapterOptions) PublishOnSpecificResponseChannel() bool {
	if s.publishOnSpecificResponseChannel == nil {
		return false
	}
	return s.publishOnSpecificResponseChannel.Get()
}
