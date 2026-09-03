// Package adapter defines types and interfaces for the Valkey Streams-based Socket.IO adapter.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
)

type (
	// RawClusterMessage represents the flat field-value shape read from a Valkey stream.
	RawClusterMessage = valkey.RawClusterMessage

	// ValkeyStreamsAdapter defines the interface for a Valkey Streams-based Socket.IO adapter.
	// It extends ClusterAdapter with Streams-specific persistence and Pub/Sub transport.
	ValkeyStreamsAdapter interface {
		adapter.ClusterAdapter

		SetValkey(*valkey.ValkeyClient)
		SetOpts(any)
		OnRawMessage(RawClusterMessage, string) error
	}
)
