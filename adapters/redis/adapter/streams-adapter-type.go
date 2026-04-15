// Package adapter defines types and interfaces for the Redis Streams-based Socket.IO adapter.
// Redis Streams provide message persistence and enable session recovery across server restarts.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

type (
	// RawClusterMessage represents a raw message from the Redis stream.
	// It is a map of string keys to any values, matching the Redis XREAD output format.
	RawClusterMessage map[string]any

	// RedisStreamsAdapter defines the interface for a Redis Streams-based Socket.IO adapter.
	// It extends ClusterAdapter with Redis Streams-specific functionality
	// and PUB/SUB support for ephemeral messages, matching the Node.js implementation.
	RedisStreamsAdapter interface {
		adapter.ClusterAdapter

		// SetRedis configures the Redis client for the adapter.
		SetRedis(*redis.RedisClient)

		// SetOpts sets the configuration options for the streams adapter.
		SetOpts(any)

		// Cleanup registers a cleanup callback to be called when the adapter is closed.
		Cleanup(func())

		// OnRawMessage processes a raw message from the Redis stream.
		OnRawMessage(RawClusterMessage, string) error
	}
)

// Uid returns the UID (unique identifier) from the raw cluster message.
// Returns empty string if the field is missing or not a string.
func (r RawClusterMessage) Uid() string {
	if value, ok := r["uid"].(string); ok {
		return value
	}
	return ""
}

// Nsp returns the namespace from the raw cluster message.
// Returns empty string if the field is missing or not a string.
func (r RawClusterMessage) Nsp() string {
	if value, ok := r["nsp"].(string); ok {
		return value
	}
	return ""
}

// Type returns the message type from the raw cluster message.
// Returns empty string if the field is missing or not a string.
func (r RawClusterMessage) Type() string {
	if value, ok := r["type"].(string); ok {
		return value
	}
	return ""
}

// Data returns the data field from the raw cluster message.
// Returns empty string if the field is missing or not a string.
func (r RawClusterMessage) Data() string {
	if value, ok := r["data"].(string); ok {
		return value
	}
	return ""
}
