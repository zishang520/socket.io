// Package emitter provides types and interfaces for broadcasting messages
// to Socket.IO servers using Redis pub/sub.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
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

		// Encoder serializes outbound messages.
		Encoder redis.Encoder

		// SubscriptionMode controls how room-specific channels are computed.
		// This should match the adapter's subscriptionMode setting.
		SubscriptionMode redis.SubscriptionMode
	}

	// BroadcastOperatorInterface defines the fluent broadcast API.
	BroadcastOperatorInterface interface {
		To(room ...socket.Room) BroadcastOperatorInterface
		In(room ...socket.Room) BroadcastOperatorInterface
		Except(room ...socket.Room) BroadcastOperatorInterface
		Compress(compress bool) BroadcastOperatorInterface
		Volatile() BroadcastOperatorInterface
		Emit(ev string, args ...any) error
		SocketsJoin(rooms ...socket.Room) error
		SocketsLeave(rooms ...socket.Room) error
		DisconnectSockets(close bool) error
		ServerSideEmit(args ...any) error
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

	// ClusterMessage is an alias for adapter.ClusterMessage.
	// It is used in sharded mode for cluster communication.
	ClusterMessage = adapter.ClusterMessage

	// BroadcastMessage is an alias for adapter.BroadcastMessage.
	// It is used in sharded mode for broadcasting.
	BroadcastMessage = adapter.BroadcastMessage

	// SocketsJoinLeaveMessage is an alias for adapter.SocketsJoinLeaveMessage.
	// It is used in sharded mode for join/leave operations.
	SocketsJoinLeaveMessage = adapter.SocketsJoinLeaveMessage

	// DisconnectSocketsMessage is an alias for adapter.DisconnectSocketsMessage.
	// It is used in sharded mode for disconnection operations.
	DisconnectSocketsMessage = adapter.DisconnectSocketsMessage

	// ServerSideEmitMessage is an alias for adapter.ServerSideEmitMessage.
	// It is used in sharded mode for server-side emit operations.
	ServerSideEmitMessage = adapter.ServerSideEmitMessage
)
