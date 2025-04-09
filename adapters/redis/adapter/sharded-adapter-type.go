package adapter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3/types"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
)

type ShardedRedisAdapter interface {
	adapter.ClusterAdapter

	SetRedis(*types.RedisClient)
	SetOpts(any)
}
