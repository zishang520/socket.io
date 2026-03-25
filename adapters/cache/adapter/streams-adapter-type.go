// Package adapter defines types and interfaces for the cache Streams-based adapter.
// Streams provide message persistence and enable session recovery across server restarts.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
)

type (
	// RawClusterMessage is a raw stream entry value map, as returned by XRead.
	RawClusterMessage map[string]any

	// CacheStreamsAdapter is the interface for a streams-based Socket.IO cluster adapter.
	CacheStreamsAdapter interface {
		adapter.ClusterAdapterWithHeartbeat

		// SetCache configures the cache client for the adapter.
		SetCache(cache.CacheClient)

		// Cleanup registers a callback invoked when the adapter is closed.
		Cleanup(func())

		// OnRawMessage processes a raw stream entry and its entry ID (offset).
		OnRawMessage(RawClusterMessage, string) error
	}
)

// Uid returns the server UID from the raw message.
func (r RawClusterMessage) Uid() string {
	if v, ok := r["uid"].(string); ok {
		return v
	}
	return ""
}

// Nsp returns the namespace from the raw message.
func (r RawClusterMessage) Nsp() string {
	if v, ok := r["nsp"].(string); ok {
		return v
	}
	return ""
}

// Type returns the message type string from the raw message.
func (r RawClusterMessage) Type() string {
	if v, ok := r["type"].(string); ok {
		return v
	}
	return ""
}

// Data returns the data field from the raw message.
func (r RawClusterMessage) Data() string {
	if v, ok := r["data"].(string); ok {
		return v
	}
	return ""
}
