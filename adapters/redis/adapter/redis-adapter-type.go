// Package adapter defines types and interfaces for the Redis-based Socket.IO adapter implementation.
package adapter

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type (
	// Packet is an alias for redis.RedisPacket, representing a broadcast packet sent via Redis.
	Packet = redis.RedisPacket

	// Request is an alias for redis.RedisRequest, representing an inter-node request.
	Request = redis.RedisRequest

	// Response is an alias for redis.RedisResponse, representing an inter-node response.
	Response = redis.RedisResponse

	// AckRequest is an alias for adapter.ClusterAckRequest, used for acknowledgement tracking.
	AckRequest = adapter.ClusterAckRequest

	// RedisRequest represents an internal request tracker with state management.
	// It extends the base RedisRequest with fields for tracking request lifecycle.
	RedisRequest struct {
		// Type identifies the message/request type.
		Type adapter.MessageType

		// Resolve is the callback invoked when the request completes successfully.
		Resolve func(*types.Slice[any])

		// Timeout is the timer for request timeout handling.
		Timeout *atomic.Pointer[utils.Timer]

		// NumSub is the number of expected responses from other nodes.
		NumSub int64

		// MsgCount tracks the number of responses received.
		MsgCount *atomic.Int64

		// Rooms accumulates room information from responses.
		Rooms *types.Set[socket.Room]

		// Sockets accumulates socket information from responses.
		Sockets *types.Slice[*adapter.SocketResponse]

		// Responses accumulates generic response data.
		Responses *types.Slice[any]
	}

	// RedisAdapter defines the interface for a Redis-based Socket.IO adapter.
	// It extends the base socket.Adapter with Redis-specific functionality.
	RedisAdapter interface {
		socket.Adapter

		// SetRedis configures the Redis client for the adapter.
		SetRedis(*redis.RedisClient)

		// SetOpts configures adapter options.
		SetOpts(any)

		// Uid returns the unique server identifier for this adapter instance.
		Uid() adapter.ServerId

		// RequestsTimeout returns the configured timeout for inter-node requests.
		RequestsTimeout() time.Duration

		// PublishOnSpecificResponseChannel indicates if responses use node-specific channels.
		PublishOnSpecificResponseChannel() bool

		// Parser returns the parser used for message encoding/decoding.
		Parser() redis.Parser

		// AllRooms returns a function to retrieve all rooms across the cluster.
		AllRooms() func(func(*types.Set[socket.Room], error))
	}
)
