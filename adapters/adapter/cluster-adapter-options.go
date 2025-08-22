package adapter

import (
	"time"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// ClusterAdapterOptionsInterface defines the interface for cluster adapter options.
type (
	ClusterAdapterOptionsInterface interface {
		SetHeartbeatInterval(time.Duration)
		GetRawHeartbeatInterval() types.Optional[time.Duration]
		HeartbeatInterval() time.Duration

		SetHeartbeatTimeout(int64)
		GetRawHeartbeatTimeout() types.Optional[int64]
		HeartbeatTimeout() int64
	}

	// ClusterAdapterOptions holds configuration for cluster adapter heartbeat and timeout.
	ClusterAdapterOptions struct {
		// The number of ms between two heartbeats.
		heartbeatInterval types.Optional[time.Duration]

		// The number of ms without heartbeat before we consider a node down.
		heartbeatTimeout types.Optional[int64]
	}
)

func DefaultClusterAdapterOptions() *ClusterAdapterOptions {
	return &ClusterAdapterOptions{}
}

func (s *ClusterAdapterOptions) Assign(data ClusterAdapterOptionsInterface) ClusterAdapterOptionsInterface {
	if data == nil {
		return s
	}
	if data.GetRawHeartbeatInterval() != nil {
		s.SetHeartbeatInterval(data.HeartbeatInterval())
	}

	if data.GetRawHeartbeatTimeout() != nil {
		s.SetHeartbeatTimeout(data.HeartbeatTimeout())
	}

	return s
}

func (s *ClusterAdapterOptions) SetHeartbeatInterval(heartbeatInterval time.Duration) {
	s.heartbeatInterval = types.NewSome(heartbeatInterval)
}
func (s *ClusterAdapterOptions) GetRawHeartbeatInterval() types.Optional[time.Duration] {
	return s.heartbeatInterval
}
func (s *ClusterAdapterOptions) HeartbeatInterval() time.Duration {
	if s.heartbeatInterval == nil {
		return 0
	}

	return s.heartbeatInterval.Get()
}

func (s *ClusterAdapterOptions) SetHeartbeatTimeout(heartbeatTimeout int64) {
	s.heartbeatTimeout = types.NewSome(heartbeatTimeout)
}
func (s *ClusterAdapterOptions) GetRawHeartbeatTimeout() types.Optional[int64] {
	return s.heartbeatTimeout
}
func (s *ClusterAdapterOptions) HeartbeatTimeout() int64 {
	if s.heartbeatTimeout == nil {
		return 0
	}

	return s.heartbeatTimeout.Get()
}
