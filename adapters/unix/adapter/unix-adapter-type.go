// Package adapter defines types and interfaces for the Unix Domain Socket-based Socket.IO adapter implementation.
// It uses Unix Domain Sockets for inter-node communication in a clustered Socket.IO environment.
package adapter

import (
	"encoding/json"
	"sync/atomic"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/unix/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	// UnixAdapter defines the interface for a Unix Domain Socket-based Socket.IO adapter.
	// It extends ClusterAdapterWithHeartbeat with Unix socket-specific functionality.
	UnixAdapter interface {
		adapter.ClusterAdapterWithHeartbeat

		// SetUnix configures the Unix Domain Socket client for the adapter.
		SetUnix(*unix.UnixClient)

		// Cleanup registers a cleanup callback to be called when the adapter is closed.
		Cleanup(func())

		// SetChannel sets the channel prefix for this adapter.
		SetChannel(string)

		// OnRawMessage processes a raw message payload received from Unix Domain Socket.
		OnRawMessage([]byte)
	}

	// UnixMessage represents a message received via Unix Domain Socket.
	// It contains the full cluster message payload along with routing metadata.
	UnixMessage struct {
		Uid     adapter.ServerId    `json:"uid,omitempty"`
		Type    adapter.MessageType `json:"type,omitempty"`
		Data    any                 `json:"data,omitempty"`
		Nsp     string              `json:"nsp,omitempty"`
		Channel string              `json:"channel,omitempty"`
	}
)

// UnixAdapterBuilder creates Unix Domain Socket adapters for Socket.IO namespaces.
// It manages the shared listener connection and message loop across all namespace adapters.
type UnixAdapterBuilder struct {
	// Unix is the Unix Domain Socket client used for communication.
	Unix *unix.UnixClient
	// Opts contains configuration options for the adapter.
	Opts UnixAdapterOptionsInterface

	namespaceToAdapters types.Map[string, UnixAdapter]
	listening           atomic.Bool
}

// New creates a new UnixAdapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (ub *UnixAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	options := DefaultUnixAdapterOptions()
	options.Assign(ub.Opts)

	// Apply defaults
	if options.GetRawKey() == nil {
		options.SetKey(DefaultChannelPrefix)
	}
	if options.GetRawHeartbeatInterval() == nil {
		options.SetHeartbeatInterval(DefaultHeartbeatInterval)
	}
	if options.GetRawHeartbeatTimeout() == nil {
		options.SetHeartbeatTimeout(DefaultHeartbeatTimeout)
	}

	channel := options.Key() + "#" + nsp.Name()

	adapterInstance := NewUnixAdapter(nsp, ub.Unix, options)
	adapterInstance.SetChannel(channel)

	ub.namespaceToAdapters.Store(nsp.Name(), adapterInstance)

	// Start listening if not already
	if ub.listening.CompareAndSwap(false, true) {
		// Create a unique listener path for this server node
		listenerPath := ub.Unix.SocketPath + "." + string(adapterInstance.(adapter.ClusterAdapter).Uid())
		if err := ub.Unix.Listen(listenerPath); err != nil {
			ub.Unix.Emit("error", err)
		}

		go ub.startListening(options)
	}

	// Register cleanup callback
	adapterInstance.Cleanup(func() {
		ub.namespaceToAdapters.Delete(nsp.Name())
	})

	return adapterInstance
}

// startListening continuously reads from the Unix Domain Socket and dispatches messages
// to the appropriate namespace adapter.
func (ub *UnixAdapterBuilder) startListening(options *UnixAdapterOptions) {
	buf := make([]byte, 65536) // 64KB buffer for Unix Domain Socket messages

	for {
		n, _, err := ub.Unix.ReadMessage(buf)
		if err != nil {
			if ub.Unix.Context.Err() != nil {
				return // Context canceled, stop listening
			}
			ub.Unix.Emit("error", err)
			continue
		}

		if n == 0 {
			continue
		}

		// Make a copy of the message data
		data := make([]byte, n)
		copy(data, buf[:n])

		// Dispatch to all namespace adapters — the adapter will filter by channel/nsp
		ub.dispatchMessage(data, options)
	}
}

// dispatchMessage sends a raw message payload to the correct adapter based on namespace.
func (ub *UnixAdapterBuilder) dispatchMessage(data []byte, options *UnixAdapterOptions) {
	// Peek at the message to determine the target namespace
	// The message is JSON-encoded and contains a "nsp" field
	var peek struct {
		Nsp string `json:"nsp,omitempty"`
	}

	// Try to extract the nsp from the message for targeted dispatch.
	// If we can't parse it, broadcast to all adapters and let them filter.
	if err := json.Unmarshal(data, &peek); err == nil && peek.Nsp != "" {
		if adapterInstance, ok := ub.namespaceToAdapters.Load(peek.Nsp); ok {
			adapterInstance.OnRawMessage(data)
		}
		return
	}

	// Fallback: dispatch to all namespace adapters
	ub.namespaceToAdapters.Range(func(_ string, adapterInstance UnixAdapter) bool {
		adapterInstance.OnRawMessage(data)
		return true
	})
}
