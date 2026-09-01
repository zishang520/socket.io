// Package emitter provides types and interfaces for broadcasting messages
// to Socket.IO servers using Unix Domain Sockets.
package emitter

import (
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type (
	// BroadcastOptions contains configuration for broadcasting messages via Unix Domain Sockets.
	// The namespace routes the message to the matching adapter.
	BroadcastOptions struct {
		// Nsp is the Socket.IO namespace for the broadcast.
		Nsp string
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
