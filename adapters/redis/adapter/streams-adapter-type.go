package adapter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3/types"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
)

type (
	RawClusterMessage map[string]any

	RedisStreamsAdapter interface {
		adapter.ClusterAdapterWithHeartbeat

		SetRedis(*types.RedisClient)
		Cleanup(func())
		OnRawMessage(RawClusterMessage, string) error
	}
)

func (r RawClusterMessage) Uid() string {
	if value, ok := r["uid"].(string); ok {
		return value
	}
	return ""
}

func (r RawClusterMessage) Nsp() string {
	if value, ok := r["nsp"].(string); ok {
		return value
	}
	return ""
}

func (r RawClusterMessage) Type() string {
	if value, ok := r["type"].(string); ok {
		return value
	}
	return ""
}

func (r RawClusterMessage) Data() string {
	if value, ok := r["data"].(string); ok {
		return value
	}
	return ""
}
