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
	Packet = redis.RedisPacket

	Request = redis.RedisRequest

	RedisRequest struct {
		Type      adapter.MessageType
		Resolve   func(*types.Slice[any])
		Timeout   *atomic.Pointer[utils.Timer]
		NumSub    int64
		MsgCount  *atomic.Int64
		Rooms     *types.Set[socket.Room]
		Sockets   *types.Slice[*adapter.SocketResponse]
		Responses *types.Slice[any]
	}

	Response = redis.RedisResponse

	AckRequest = adapter.ClusterAckRequest

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
