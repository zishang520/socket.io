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
	// Packet is an alias for redis.RedisPacket, representing a packet sent via Redis.
	Packet = redis.RedisPacket

	// Request is an alias for redis.RedisRequest, representing a request sent via Redis.
	Request = redis.RedisRequest

	// RedisRequest extends the base RedisRequest with additional fields for internal request tracking.
	RedisRequest struct {
		Type      adapter.MessageType                   // The type of the message/request.
		Resolve   func(*types.Slice[any])               // Callback to resolve the request.
		Timeout   *atomic.Pointer[utils.Timer]          // Timeout for the request.
		NumSub    int64                                 // Number of expected responses.
		MsgCount  *atomic.Int64                         // Counter for received messages.
		Rooms     *types.Set[socket.Room]               // Set of rooms involved in the request.
		Sockets   *types.Slice[*adapter.SocketResponse] // Slice of socket responses.
		Responses *types.Slice[any]                     // Slice of generic responses.
	}

	// Response is an alias for redis.RedisResponse, representing a response sent via Redis.
	Response = redis.RedisResponse

	// AckRequest is an alias for adapter.ClusterAckRequest, used for acknowledgement tracking.
	AckRequest = adapter.ClusterAckRequest

	// RedisAdapter defines the interface for a Redis-based Socket.IO adapter.
	RedisAdapter interface {
		socket.Adapter

		SetRedis(*redis.RedisClient)
		SetOpts(any)

		Uid() adapter.ServerId
		RequestsTimeout() time.Duration
		PublishOnSpecificResponseChannel() bool
		Parser() redis.Parser

		AllRooms() func(func(*types.Set[socket.Room], error))
	}
)
