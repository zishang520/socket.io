// Package adapter provides configuration for the Unix Socket.IO adapter.
package adapter

import "github.com/zishang520/socket.io/adapters/adapter/v3"

type (
	// UnixAdapterOptionsInterface is the shared cluster adapter configuration.
	UnixAdapterOptionsInterface = adapter.ClusterAdapterOptionsInterface

	// UnixAdapterOptions contains the shared cluster adapter configuration.
	UnixAdapterOptions = adapter.ClusterAdapterOptions
)

// DefaultUnixAdapterOptions returns empty options. Shared defaults are applied
// by ClusterAdapterWithHeartbeat during construction.
func DefaultUnixAdapterOptions() *UnixAdapterOptions {
	return adapter.DefaultClusterAdapterOptions()
}
