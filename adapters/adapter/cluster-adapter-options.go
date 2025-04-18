package adapter

import (
	"time"
)

// ClusterAdapterOptionsInterface defines the interface for cluster adapter options.
type (
	ClusterAdapterOptionsInterface interface {
		SetHeartbeatInterval(time.Duration)
		GetRawHeartbeatInterval() *time.Duration
		HeartbeatInterval() time.Duration

		SetHeartbeatTimeout(int64)
		GetRawHeartbeatTimeout() *int64
		HeartbeatTimeout() int64
	}

	// ClusterAdapterOptions holds configuration for cluster adapter heartbeat and timeout.
	ClusterAdapterOptions struct {
		// The number of ms between two heartbeats.
		//
		// Default: 5_000 * time.Millisecond
		heartbeatInterval *time.Duration

		// The number of ms without heartbeat before we consider a node down.
		//
		// Default: 10_000
		heartbeatTimeout *int64
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
	s.heartbeatInterval = &heartbeatInterval
}
func (s *ClusterAdapterOptions) GetRawHeartbeatInterval() *time.Duration {
	return s.heartbeatInterval
}
func (s *ClusterAdapterOptions) HeartbeatInterval() time.Duration {
	if s.heartbeatInterval == nil {
		return 0
	}

	return *s.heartbeatInterval
}

func (s *ClusterAdapterOptions) SetHeartbeatTimeout(heartbeatTimeout int64) {
	s.heartbeatTimeout = &heartbeatTimeout
}
func (s *ClusterAdapterOptions) GetRawHeartbeatTimeout() *int64 {
	return s.heartbeatTimeout
}
func (s *ClusterAdapterOptions) HeartbeatTimeout() int64 {
	if s.heartbeatTimeout == nil {
		return 0
	}

	return *s.heartbeatTimeout
}
