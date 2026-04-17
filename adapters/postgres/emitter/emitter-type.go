// Package emitter provides types and interfaces for broadcasting messages
// to Socket.IO servers using PostgreSQL LISTEN/NOTIFY.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/postgres/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type (
	// BroadcastOptions contains configuration for broadcasting messages via PostgreSQL channels.
	// These options determine how messages are routed and encoded.
	BroadcastOptions struct {
		// Nsp is the Socket.IO namespace for the broadcast.
		Nsp string

		// BroadcastChannel is the PostgreSQL channel used for all messages.
		// Format: "{key}#{nsp}"
		BroadcastChannel string

		// TableName is the name of the attachment table for large payloads.
		TableName string

		// PayloadThreshold is the byte threshold above which payloads are stored in the attachment table.
		PayloadThreshold int
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
		ServerSideEmit(args ...any) error
	}
	// ClusterMessage is an alias for adapter.ClusterMessage.
	// It is used for cluster communication.
	ClusterMessage = adapter.ClusterMessage

	// BroadcastMessage is an alias for adapter.BroadcastMessage.
	// It is used for broadcasting.
	BroadcastMessage = adapter.BroadcastMessage

	// SocketsJoinLeaveMessage is an alias for adapter.SocketsJoinLeaveMessage.
	// It is used for join/leave operations.
	SocketsJoinLeaveMessage = adapter.SocketsJoinLeaveMessage

	// DisconnectSocketsMessage is an alias for adapter.DisconnectSocketsMessage.
	// It is used for disconnection operations.
	DisconnectSocketsMessage = adapter.DisconnectSocketsMessage

	// ServerSideEmitMessage is an alias for adapter.ServerSideEmitMessage.
	// It is used for server-side emit operations.
	ServerSideEmitMessage = adapter.ServerSideEmitMessage

	// NotificationMessage represents a message received via PostgreSQL LISTEN/NOTIFY.
	NotificationMessage = postgres.NotificationMessage
)
