// Package mongo provides the MongoDB transport for Socket.IO clustering.
package mongo

import "github.com/zishang520/socket.io/adapters/adapter/v3"

const (
	INITIAL_HEARTBEAT         = adapter.INITIAL_HEARTBEAT
	HEARTBEAT                 = adapter.HEARTBEAT
	BROADCAST                 = adapter.BROADCAST
	SOCKETS_JOIN              = adapter.SOCKETS_JOIN
	SOCKETS_LEAVE             = adapter.SOCKETS_LEAVE
	DISCONNECT_SOCKETS        = adapter.DISCONNECT_SOCKETS
	FETCH_SOCKETS             = adapter.FETCH_SOCKETS
	FETCH_SOCKETS_RESPONSE    = adapter.FETCH_SOCKETS_RESPONSE
	SERVER_SIDE_EMIT          = adapter.SERVER_SIDE_EMIT
	SERVER_SIDE_EMIT_RESPONSE = adapter.SERVER_SIDE_EMIT_RESPONSE
	BROADCAST_CLIENT_COUNT    = adapter.BROADCAST_CLIENT_COUNT
	BROADCAST_ACK             = adapter.BROADCAST_ACK

	SESSION EventType = 13
)

const EMITTER_UID = adapter.EMITTER_UID
