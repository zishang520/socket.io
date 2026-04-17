// Package emitter provides types and interfaces for broadcasting messages
// to Socket.IO servers using Unix Domain Sockets.
package emitter

import (
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
)
