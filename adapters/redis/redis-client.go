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
// The client supports read/write separation: Client is used for write operations
// (PUBLISH, XADD, SET, etc.) and SubClient is used for read/subscribe operations
// (SUBSCRIBE, XREAD, XRANGE, etc.). If SubClient is nil, Client is used for both.
//
// The client supports error event emission, which allows higher-level components
// to handle Redis-related errors gracefully.
type RedisClient struct {
	types.EventEmitter

	// Client is the underlying Redis universal client used for write operations
	// (PUBLISH, XADD, SET, etc.) and metadata queries (PUBSUB NUMSUB).
	// It supports both standalone and cluster Redis deployments.
	Client redis.UniversalClient

	// SubClient is an optional separate Redis universal client used for
	// read/subscribe operations (SUBSCRIBE, PSUBSCRIBE, SSUBSCRIBE, XREAD,
	// XRANGE, etc.). When nil, Client is used for all operations.
	//
	// Using a separate client for subscriptions prevents blocking read operations
	// from starving the write connection pool, and allows routing reads to
	// Redis replicas for improved scalability.
	SubClient redis.UniversalClient

	// Context is the context used for Redis operations.
	// This context controls the lifecycle of Redis subscriptions and operations.
	Context context.Context
}

// Sub returns the Redis client to use for read/subscribe operations.
// If SubClient is set, it is returned; otherwise Client is used as the fallback.
func (r *RedisClient) Sub() redis.UniversalClient {
	if r.SubClient != nil {
		return r.SubClient
	}
	return r.Client
}

// NewRedisClient creates a new RedisClient with the given context and Redis universal client.
// The same client is used for both read and write operations.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of Redis operations.
//     When canceled, all subscriptions and pending operations will be terminated.
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

// NewRedisClientWithSub creates a new RedisClient with separate clients for read/write separation.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of Redis operations.
//   - client: The Redis client for write operations (PUBLISH, XADD, SET, etc.)
//     and metadata queries (PUBSUB NUMSUB).
//   - subClient: The Redis client for read/subscribe operations (SUBSCRIBE, XREAD, etc.).
//     Both clients should connect to the same Redis deployment.
//
// Example:
//
//	pubClient := redis.NewClient(&redis.Options{Addr: "master:6379"})
//	subClient := redis.NewClient(&redis.Options{Addr: "replica:6380"})
//	redisClient := NewRedisClientWithSub(context.Background(), pubClient, subClient)
func NewRedisClientWithSub(ctx context.Context, client, subClient redis.UniversalClient) *RedisClient {
	if ctx == nil {
		ctx = context.Background()
	}

	return &RedisClient{
		EventEmitter: types.NewEventEmitter(),
		Client:       client,
		SubClient:    subClient,
		Context:      ctx,
	}
}
