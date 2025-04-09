package emitter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3/types"
)

type (
	BroadcastOptions struct {
		Nsp              string
		BroadcastChannel string
		RequestChannel   string
		Parser           types.Parser
	}

	Packet = types.RedisPacket

	Request = types.RedisRequest

	Response = types.RedisResponse
)
