// Package emitter provides types and interfaces for broadcasting messages
// to Socket.IO servers using Valkey pub/sub.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type (
	// BroadcastOptions contains configuration for broadcasting messages to Valkey channels.
	BroadcastOptions struct {
		Nsp              string
		BroadcastChannel string
		RequestChannel   string
		Parser           valkey.Parser
		Sharded          bool
		SubscriptionMode valkey.SubscriptionMode
	}

	// BroadcastOperatorInterface defines the common interface for broadcast operators.
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

	// Packet is an alias for valkey.ValkeyPacket.
	Packet = valkey.ValkeyPacket

	// Request is an alias for valkey.ValkeyRequest.
	Request = valkey.ValkeyRequest

	// Response is an alias for valkey.ValkeyResponse.
	Response = valkey.ValkeyResponse

	// ClusterMessage is an alias for adapter.ClusterMessage.
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
