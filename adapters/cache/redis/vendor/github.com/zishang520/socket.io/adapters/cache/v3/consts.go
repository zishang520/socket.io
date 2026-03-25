// Package cache defines message-type constants used for inter-node communication
// in a clustered Socket.IO environment backed by a cache pub/sub layer.
package cache

import "github.com/zishang520/socket.io/adapters/adapter/v3"

// Inter-node message types for the cache adapter.
const (
	// SOCKETS requests the list of socket IDs from other nodes.
	SOCKETS adapter.MessageType = iota

	// ALL_ROOMS requests the list of all rooms from other nodes.
	ALL_ROOMS

	// REMOTE_JOIN instructs other nodes to join a socket to the given rooms.
	REMOTE_JOIN

	// REMOTE_LEAVE instructs other nodes to remove a socket from the given rooms.
	REMOTE_LEAVE

	// REMOTE_DISCONNECT instructs other nodes to disconnect a specific socket.
	REMOTE_DISCONNECT

	// REMOTE_FETCH requests detailed socket information from other nodes.
	REMOTE_FETCH

	// SERVER_SIDE_EMIT broadcasts a server-side event to all other nodes.
	SERVER_SIDE_EMIT

	// BROADCAST sends a Socket.IO packet to clients across all nodes.
	BROADCAST

	// BROADCAST_CLIENT_COUNT reports how many clients will receive a broadcast.
	BROADCAST_CLIENT_COUNT

	// BROADCAST_ACK carries acknowledgement responses for broadcast operations.
	BROADCAST_ACK
)
