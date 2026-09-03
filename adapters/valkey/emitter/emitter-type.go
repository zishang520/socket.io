// Package emitter provides types and interfaces for broadcasting messages
// to Socket.IO servers using Valkey Pub/Sub.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type (
	// BroadcastOptions contains the immutable routing and encoding options of an operator.
	BroadcastOptions struct {
		Nsp              string
		BroadcastChannel string
		RequestChannel   string
		Encoder          valkey.Encoder
		SubscriptionMode valkey.SubscriptionMode
	}

	// BroadcastOperatorInterface defines the fluent broadcast API shared by all emitter transports.
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

	Packet                   = valkey.ValkeyPacket
	Request                  = valkey.ValkeyRequest
	Response                 = valkey.ValkeyResponse
	ClusterMessage           = adapter.ClusterMessage
	BroadcastMessage         = adapter.BroadcastMessage
	SocketsJoinLeaveMessage  = adapter.SocketsJoinLeaveMessage
	DisconnectSocketsMessage = adapter.DisconnectSocketsMessage
	ServerSideEmitMessage    = adapter.ServerSideEmitMessage
)
