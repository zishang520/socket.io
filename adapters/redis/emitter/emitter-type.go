// Package emitter provides types for broadcasting messages using Redis in Socket.IO.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

type (
	// BroadcastOptions defines options for broadcasting messages to Redis channels.
	BroadcastOptions struct {
		Nsp              string
		BroadcastChannel string
		RequestChannel   string
		Parser           redis.Parser
	}

	// Packet is an alias for redis.RedisPacket.
	Packet = redis.RedisPacket

	// Request is an alias for redis.RedisRequest.
	Request = redis.RedisRequest

	// Response is an alias for redis.RedisResponse.
	Response = redis.RedisResponse
)
