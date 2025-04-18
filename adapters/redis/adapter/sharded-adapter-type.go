// Package adapter defines the interface for the Redis sharded Pub/Sub adapter for Socket.IO.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

// ShardedRedisAdapter defines the interface for a sharded Redis-based Socket.IO adapter.
type ShardedRedisAdapter interface {
	adapter.ClusterAdapter

	SetRedis(*redis.RedisClient)
	SetOpts(any)
}
