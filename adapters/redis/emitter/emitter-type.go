package emitter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

type (
	BroadcastOptions struct {
		Nsp              string
		BroadcastChannel string
		RequestChannel   string
		Parser           redis.Parser
	}

	Packet = redis.RedisPacket

	Request = redis.RedisRequest

	Response = redis.RedisResponse
)
