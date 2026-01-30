// Package emitter provides types and interfaces for broadcasting messages
// to Socket.IO servers using Redis pub/sub.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

type (
	// BroadcastOptions contains configuration for broadcasting messages to Redis channels.
	// These options determine how messages are routed and encoded.
	BroadcastOptions struct {
		// Nsp is the Socket.IO namespace for the broadcast.
		Nsp string

		// BroadcastChannel is the Redis channel used for broadcasting packets to clients.
		// Format: "{key}#{nsp}#" or "{key}#{nsp}#{room}#" for room-specific broadcasts.
		BroadcastChannel string

		// RequestChannel is the Redis channel used for inter-server requests.
		// Format: "{key}-request#{nsp}#"
		RequestChannel string

		// Parser is the encoder/decoder for serializing messages.
		Parser redis.Parser
	}

	// Packet is an alias for redis.RedisPacket.
	// It represents a Socket.IO packet with routing options.
	Packet = redis.RedisPacket

	// Request is an alias for redis.RedisRequest.
	// It represents an inter-server request message.
	Request = redis.RedisRequest

	// Response is an alias for redis.RedisResponse.
	// It represents an inter-server response message.
	Response = redis.RedisResponse
)
