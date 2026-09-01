// Package postgres defines constants for PostgreSQL-based message types used in the Socket.IO adapter.
// These message types are used for inter-node communication in a clustered Socket.IO environment.
package postgres

import (
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
)

const (
	// DefaultOperationTimeout bounds publish, attachment fetch, and listener update operations.
	DefaultOperationTimeout = 5 * time.Second

	EMITTER_UID = adapter.EMITTER_UID

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
	ADAPTER_CLOSE             = adapter.ADAPTER_CLOSE
)
