// Package redis provides Redis client wrapper for Socket.IO Redis adapter.
// This package offers a unified interface for Redis operations with event handling support.
package redis

import (
	"context"
	"errors"

	"github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	// ErrRedisClientRequired is returned when no primary Redis client is provided.
	ErrRedisClientRequired = errors.New("redis: client is required")

	// ErrUnsupportedRedisClient is returned when a go-redis Ring is configured.
	// Socket.IO supports standalone, Sentinel, and Redis Cluster deployments.
	ErrUnsupportedRedisClient = errors.New("redis: *redis.Ring is not supported")
)

// RedisClient wraps a Redis UniversalClient and provides context management
// and event emitting capabilities for the Socket.IO Redis adapter. Supported
// built-in clients are *redis.Client for standalone and Sentinel deployments
// and *redis.ClusterClient for Redis Cluster. *redis.Ring is not supported.
//
// The client supports read/write separation: Client() is used for write
// operations (PUBLISH, XADD, SET, etc.) and Sub() is used for reads and
// subscriptions (SUBSCRIBE, XREAD, XRANGE, etc.). Without a separate
// subscription client, both methods return the primary client.
//
// The client supports error event emission, which allows higher-level components
// to handle Redis-related errors gracefully. Its Redis clients and context are
// immutable after construction.
type RedisClient struct {
	types.EventEmitter

	client    redis.UniversalClient
	subClient redis.UniversalClient
	ctx       context.Context
}

// Client returns the Redis client used for write operations.
func (r *RedisClient) Client() redis.UniversalClient {
	return r.client
}

// Sub returns the Redis client used for read and subscription operations.
func (r *RedisClient) Sub() redis.UniversalClient {
	return r.subClient
}

// Context returns the context controlling Redis operations and subscriptions.
func (r *RedisClient) Context() context.Context {
	return r.ctx
}

func validateClient(client redis.UniversalClient) error {
	if _, unsupported := client.(*redis.Ring); unsupported {
		return ErrUnsupportedRedisClient
	}
	return nil
}

// NewRedisClient creates a new RedisClient with the given context and Redis universal client.
// The same client is used for both read and write operations. Supported built-in
// clients are *redis.Client and *redis.ClusterClient; *redis.Ring is rejected.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of Redis operations.
//     When canceled, all subscriptions and pending operations will be terminated.
//   - client: A Redis UniversalClient instance that handles the actual Redis communication.
//
// Returns:
//   - A pointer to the initialized RedisClient instance, or an error when the
//     configuration is invalid.
//
// Example:
//
//	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
//	redisClient, err := NewRedisClient(context.Background(), client)
func NewRedisClient(ctx context.Context, client redis.UniversalClient) (*RedisClient, error) {
	return NewRedisClientWithSub(ctx, client, nil)
}

// NewRedisClientWithSub creates a new RedisClient with separate clients for
// read/write separation. Supported built-in clients are *redis.Client and
// *redis.ClusterClient; *redis.Ring is rejected for either role.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of Redis operations.
//   - client: The Redis client for write operations (PUBLISH, XADD, SET, etc.)
//     and metadata queries (PUBSUB NUMSUB).
//   - subClient: The Redis client for read/subscribe operations (SUBSCRIBE, XREAD, etc.).
//     Both clients should connect to the same Redis deployment.
//
// Returns:
//   - A pointer to the initialized RedisClient instance, or an error when the
//     configuration is invalid.
//
// Example:
//
//	pubClient := redis.NewClient(&redis.Options{Addr: "master:6379"})
//	subClient := redis.NewClient(&redis.Options{Addr: "replica:6380"})
//	redisClient, err := NewRedisClientWithSub(context.Background(), pubClient, subClient)
func NewRedisClientWithSub(ctx context.Context, client, subClient redis.UniversalClient) (*RedisClient, error) {
	if utils.IsNil(client) {
		return nil, ErrRedisClientRequired
	}
	if err := validateClient(client); err != nil {
		return nil, err
	}
	if utils.IsNil(subClient) {
		subClient = client
	}
	if err := validateClient(subClient); err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}

	return &RedisClient{
		EventEmitter: types.NewEventEmitter(),
		client:       client,
		subClient:    subClient,
		ctx:          ctx,
	}, nil
}
