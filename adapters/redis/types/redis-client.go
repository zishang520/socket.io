package types

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type RedisClient struct {
	types.EventEmitter

	Client  redis.UniversalClient
	Context context.Context
}

func NewRedisClient(ctx context.Context, redis redis.UniversalClient) *RedisClient {
	return &RedisClient{
		EventEmitter: types.NewEventEmitter(),
		Client:       redis,
		Context:      ctx,
	}
}
