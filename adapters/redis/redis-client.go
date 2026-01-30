// Package redis provides Redis client wrapper for Socket.IO Redis adapter.
// This package offers a unified interface for Redis operations with event handling support.
package redis

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// RedisClient wraps a Redis UniversalClient and provides context management
// and event emitting capabilities for the Socket.IO Redis adapter.
//
// The client supports error event emission, which allows higher-level components
// to handle Redis-related errors gracefully.
type RedisClient struct {
	types.EventEmitter

	// Client is the underlying Redis universal client.
	// It supports both standalone and cluster Redis deployments.
	Client redis.UniversalClient

	// Context is the context used for Redis operations.
	// This context controls the lifecycle of Redis subscriptions and operations.
	Context context.Context
}

// NewRedisClient creates a new RedisClient with the given context and Redis universal client.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of Redis operations.
//     When cancelled, all subscriptions and pending operations will be terminated.
//   - client: A Redis UniversalClient instance that handles the actual Redis communication.
//
// Returns:
//   - A pointer to the initialized RedisClient instance.
//
// Example:
//
//	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	redisClient := NewRedisClient(context.Background(), client)
func NewRedisClient(ctx context.Context, client redis.UniversalClient) *RedisClient {
	if ctx == nil {
		ctx = context.Background()
	}

	return &RedisClient{
		EventEmitter: types.NewEventEmitter(),
		Client:       client,
		Context:      ctx,
	}
}
