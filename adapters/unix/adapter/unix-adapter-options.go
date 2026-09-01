// Package adapter provides configuration for the Unix Socket.IO adapter.
package adapter

import baseadapter "github.com/zishang520/socket.io/adapters/adapter/v3"

type (
	// UnixAdapterOptionsInterface is the shared cluster adapter configuration.
	UnixAdapterOptionsInterface = baseadapter.ClusterAdapterOptionsInterface

	// UnixAdapterOptions contains the shared cluster adapter configuration.
	UnixAdapterOptions = baseadapter.ClusterAdapterOptions
)

// DefaultUnixAdapterOptions returns empty options. Shared defaults are applied
// by ClusterAdapterWithHeartbeat during construction.
func DefaultUnixAdapterOptions() *UnixAdapterOptions {
	return baseadapter.DefaultClusterAdapterOptions()
}
