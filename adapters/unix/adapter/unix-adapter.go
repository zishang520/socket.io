// Package adapter provides a Unix Domain Socket-based adapter implementation
// for Socket.IO clustering on the same machine.
package adapter

import (
	"sync/atomic"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/unix/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// unixAdapter implements cluster publishing over a shared UnixClient.
type unixAdapter struct {
	adapter.ClusterAdapterWithHeartbeat

	unixClient  *unix.UnixClient
	cleanupFunc atomic.Pointer[types.Callable]
	isClosed    atomic.Bool
}

// MakeUnixAdapter creates an uninitialized Unix adapter.
//
// Deprecated: use UnixAdapterBuilder, which registers the namespace with the
// shared listener lifecycle.
func MakeUnixAdapter() UnixAdapter {
	a := &unixAdapter{
		ClusterAdapterWithHeartbeat: adapter.MakeClusterAdapterWithHeartbeat(),
	}
	a.Prototype(a)
	return a
}

// NewUnixAdapter creates an initialized Unix adapter. Use UnixAdapterBuilder
// when installing it on a server so the shared listener and namespace routing
// are registered.
func NewUnixAdapter(nsp socket.Namespace, client *unix.UnixClient, opts any) UnixAdapter {
	a := MakeUnixAdapter()
	a.SetUnix(client)
	a.SetOpts(opts)
	a.Construct(nsp)
	return a
}

// SetUnix sets the shared Unix client used by the adapter.
func (a *unixAdapter) SetUnix(client *unix.UnixClient) {
	a.unixClient = client
}

// DoPublish publishes a cluster message to all peer Unix listeners.
func (a *unixAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	payload, err := adapter.EncodeClusterMessage(message)
	if err != nil {
		return "", err
	}
	return "", a.unixClient.Broadcast(payload)
}

// DoPublishResponse publishes a response over the shared broadcast transport.
func (a *unixAdapter) DoPublishResponse(_ adapter.ServerId, response *adapter.ClusterResponse) error {
	_, err := a.DoPublish(response)
	return err
}

// Cleanup registers the builder cleanup invoked when the adapter closes.
func (a *unixAdapter) Cleanup(cleanup func()) {
	if cleanup == nil {
		a.cleanupFunc.Store(nil)
		return
	}
	a.cleanupFunc.Store(&cleanup)
	if a.isClosed.Load() {
		if callback := a.cleanupFunc.Swap(nil); callback != nil {
			(*callback)()
		}
	}
}

// Close unregisters this namespace and stops its heartbeat exactly once. The
// shared listener remains owned by UnixClient and is released by UnixClient.Close.
func (a *unixAdapter) Close() {
	if !a.isClosed.CompareAndSwap(false, true) {
		return
	}
	a.ClusterAdapterWithHeartbeat.Close()
	if callback := a.cleanupFunc.Swap(nil); callback != nil {
		(*callback)()
	}
}
