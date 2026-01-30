// Package adapter defines the interface for the Redis sharded Pub/Sub adapter for Socket.IO.
// This adapter leverages Redis 7.0's sharded Pub/Sub for improved scalability in clustered environments.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

// ShardedRedisAdapter defines the interface for a sharded Redis-based Socket.IO adapter.
// It extends ClusterAdapter with Redis-specific configuration methods.
type ShardedRedisAdapter interface {
	adapter.ClusterAdapter

	// SetRedis configures the Redis client for the adapter.
	SetRedis(*redis.RedisClient)

	// SetOpts configures adapter options.
	SetOpts(any)
}
