// Package emitter provides types and interfaces for broadcasting messages
// to Socket.IO servers using Unix Domain Sockets.
package emitter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/unix/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type (
	// BroadcastOptions contains configuration for broadcasting messages via Unix Domain Sockets.
	// These options determine how messages are routed and encoded.
	BroadcastOptions struct {
		// Nsp is the Socket.IO namespace for the broadcast.
		Nsp string

		// SocketPath is the base path of the Unix Domain Socket.
		SocketPath string
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

	// Packet is an alias for unix.UnixPacket.
	// It represents a Socket.IO packet with routing options.
	Packet = unix.UnixPacket

	// Request is an alias for unix.UnixRequest.
	// It represents an inter-server request message.
	Request = unix.UnixRequest

	// Response is an alias for unix.UnixResponse.
	// It represents an inter-server response message.
	Response = unix.UnixResponse

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
)
