// Package adapter defines types and interfaces for the cache-based Socket.IO adapter.
package adapter

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type (
	// Packet is an alias for cache.CachePacket.
	Packet = cache.CachePacket

	// Request is an alias for cache.CacheRequest.
	Request = cache.CacheRequest

	// Response is an alias for cache.CacheResponse.
	Response = cache.CacheResponse

	// AckRequest is an alias for adapter.ClusterAckRequest used for ack tracking.
	AckRequest = adapter.ClusterAckRequest

	// CacheRequest is the internal request-tracking struct that extends the wire
	// CacheRequest with lifecycle fields.
	CacheRequest struct {
		// Type identifies the message/request type.
		Type adapter.MessageType

		// Resolve is invoked when the request completes successfully.
		Resolve func(*types.Slice[any])

		// Timeout holds the active timeout timer.
		Timeout *atomic.Pointer[utils.Timer]

		// NumSub is the expected number of responses from peer nodes.
		NumSub int64

		// MsgCount tracks how many responses have arrived.
		MsgCount *atomic.Int64

		// Rooms accumulates room information from responses.
		Rooms *types.Set[socket.Room]

		// Sockets accumulates socket information from responses.
		Sockets *types.Slice[*adapter.SocketResponse]

		// Responses accumulates generic response payloads.
		Responses *types.Slice[any]
	}

	// CacheAdapter is the interface for a cache-backed Socket.IO adapter.
	// It extends socket.Adapter with cache-specific configuration methods.
	CacheAdapter interface {
		socket.Adapter

		// SetCache configures the cache client for the adapter.
		SetCache(cache.CacheClient)

		// SetOpts configures adapter options.
		SetOpts(any)

		// Uid returns the unique server identifier for this adapter instance.
		Uid() adapter.ServerId

		// RequestsTimeout returns the configured timeout for inter-node requests.
		RequestsTimeout() time.Duration

		// PublishOnSpecificResponseChannel reports whether responses use a per-node channel.
		PublishOnSpecificResponseChannel() bool

		// Parser returns the codec used for encoding/decoding inter-node messages.
		Parser() cache.Parser

		// AllRooms returns a function that collects all rooms across the cluster.
		AllRooms() func(func(*types.Set[socket.Room], error))
	}
)
