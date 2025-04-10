package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

type ShardedRedisAdapter interface {
	adapter.ClusterAdapter

	SetRedis(*redis.RedisClient)
	SetOpts(any)
}
