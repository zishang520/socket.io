// Package emitter provides types and interfaces for broadcasting messages
// to Socket.IO servers using the cache pub/sub layer.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type (
	// BroadcastOptions configures how messages are routed to cache pub/sub channels.
	BroadcastOptions struct {
		// Nsp is the Socket.IO namespace for this broadcast.
		Nsp string

		// BroadcastChannel is the channel used to deliver packets to clients.
		// Format: "{key}#{nsp}#" or "{key}#{nsp}#{room}#" for room-specific routing.
		BroadcastChannel string

		// RequestChannel is the channel used for inter-server requests.
		// Format: "{key}-request#{nsp}#"
		RequestChannel string

		// Parser encodes/decodes inter-node messages.
		Parser cache.Parser

		// Sharded enables sharded pub/sub (SPublish) for cluster mode.
		Sharded bool

		// SubscriptionMode controls room-specific channel routing.
		SubscriptionMode cache.SubscriptionMode
	}

	// BroadcastOperatorInterface is the common interface for all broadcast operators.
	BroadcastOperatorInterface interface {
		To(room ...socket.Room) BroadcastOperatorInterface
		In(room ...socket.Room) BroadcastOperatorInterface
		Except(room ...socket.Room) BroadcastOperatorInterface
		Compress(compress bool) BroadcastOperatorInterface
		Volatile() BroadcastOperatorInterface
		Emit(ev string, args ...any) error
		SocketsJoin(rooms ...socket.Room) error
		SocketsLeave(rooms ...socket.Room) error
		DisconnectSockets(state bool) error
	}

	// Packet is an alias for cache.CachePacket.
	Packet = cache.CachePacket

	// Request is an alias for cache.CacheRequest.
	Request = cache.CacheRequest

	// Response is an alias for cache.CacheResponse.
	Response = cache.CacheResponse

	// ClusterMessage is an alias for adapter.ClusterMessage (used in sharded mode).
	ClusterMessage = adapter.ClusterMessage

	// BroadcastMessage is an alias for adapter.BroadcastMessage.
	BroadcastMessage = adapter.BroadcastMessage

	// SocketsJoinLeaveMessage is an alias for adapter.SocketsJoinLeaveMessage.
	SocketsJoinLeaveMessage = adapter.SocketsJoinLeaveMessage

	// DisconnectSocketsMessage is an alias for adapter.DisconnectSocketsMessage.
	DisconnectSocketsMessage = adapter.DisconnectSocketsMessage

	// ServerSideEmitMessage is an alias for adapter.ServerSideEmitMessage.
	ServerSideEmitMessage = adapter.ServerSideEmitMessage
)
