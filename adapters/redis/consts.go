// Package redis defines constants for Redis-based message types used in the Socket.IO adapter.
package redis

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
)

const (
	// SOCKETS represents a message type for socket operations.
	SOCKETS adapter.MessageType = iota
	// ALL_ROOMS represents a message type for all rooms operations.
	ALL_ROOMS
	// REMOTE_JOIN represents a message type for remote join operations.
	REMOTE_JOIN
	// REMOTE_LEAVE represents a message type for remote leave operations.
	REMOTE_LEAVE
	// REMOTE_DISCONNECT represents a message type for remote disconnect operations.
	REMOTE_DISCONNECT
	// REMOTE_FETCH represents a message type for remote fetch operations.
	REMOTE_FETCH
	// SERVER_SIDE_EMIT represents a message type for server-side emit operations.
	SERVER_SIDE_EMIT
	// BROADCAST represents a message type for broadcast operations.
	BROADCAST
	// BROADCAST_CLIENT_COUNT represents a message type for broadcast client count operations.
	BROADCAST_CLIENT_COUNT
	// BROADCAST_ACK represents a message type for broadcast acknowledgement operations.
	BROADCAST_ACK
)
