// Package redis provides Redis client wrapper for Socket.IO Redis adapter.
package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// RedisClient wraps a Redis UniversalClient and provides context and event emitting capabilities.
type RedisClient struct {
	// EventEmitter provides event handling for the Redis client.
	types.EventEmitter

	// Client is the underlying Redis universal client.
	Client redis.UniversalClient
	// Context is the context used for Redis operations.
	Context context.Context
}

// NewRedisClient creates a new RedisClient with the given context and Redis universal client.
func NewRedisClient(ctx context.Context, redis redis.UniversalClient) *RedisClient {
	return &RedisClient{
		EventEmitter: types.NewEventEmitter(),
		Client:       redis,
		Context:      ctx,
	}
}
