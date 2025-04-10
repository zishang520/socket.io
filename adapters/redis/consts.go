package redis

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
)

const (
	SOCKETS adapter.MessageType = iota
	ALL_ROOMS
	REMOTE_JOIN
	REMOTE_LEAVE
	REMOTE_DISCONNECT
	REMOTE_FETCH
	SERVER_SIDE_EMIT
	BROADCAST
	BROADCAST_CLIENT_COUNT
	BROADCAST_ACK
)
