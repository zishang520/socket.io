// Package adapter defines types and interfaces for the Redis Streams-based Socket.IO adapter.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

type (
	// RawClusterMessage represents a raw message from the Redis stream.
	RawClusterMessage map[string]any

	// RedisStreamsAdapter defines the interface for a Redis Streams-based Socket.IO adapter.
	RedisStreamsAdapter interface {
		adapter.ClusterAdapterWithHeartbeat

		SetRedis(*redis.RedisClient)
		Cleanup(func())
		OnRawMessage(RawClusterMessage, string) error
	}
)

// Uid returns the UID from the raw cluster message.
func (r RawClusterMessage) Uid() string {
	if value, ok := r["uid"].(string); ok {
		return value
	}
	return ""
}

// Nsp returns the namespace from the raw cluster message.
func (r RawClusterMessage) Nsp() string {
	if value, ok := r["nsp"].(string); ok {
		return value
	}
	return ""
}

// Type returns the message type from the raw cluster message.
func (r RawClusterMessage) Type() string {
	if value, ok := r["type"].(string); ok {
		return value
	}
	return ""
}

// Data returns the data field from the raw cluster message.
func (r RawClusterMessage) Data() string {
	if value, ok := r["data"].(string); ok {
		return value
	}
	return ""
}
