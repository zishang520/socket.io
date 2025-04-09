package types

import (
	"context"

	"github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/servers/engine/v3/events"
)

type RedisClient struct {
	events.EventEmitter

	Client  redis.UniversalClient
	Context context.Context
}

func NewRedisClient(ctx context.Context, redis redis.UniversalClient) *RedisClient {
	return &RedisClient{
		EventEmitter: events.New(),
		Client:       redis,
		Context:      ctx,
	}
}
