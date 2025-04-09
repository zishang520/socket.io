package adapter

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/servers/engine/v3/utils"
	"github.com/zishang520/socket.io/adapters/redis/v3/types"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type (
	Packet = types.RedisPacket

	Request = types.RedisRequest

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

	Response = types.RedisResponse

	AckRequest = adapter.ClusterAckRequest

	RedisAdapter interface {
		socket.Adapter

		SetRedis(*types.RedisClient)
		SetOpts(any)

		Uid() adapter.ServerId
		RequestsTimeout() time.Duration
		PublishOnSpecificResponseChannel() bool
		Parser() types.Parser

		AllRooms() func(func(*types.Set[socket.Room], error))
	}
)
