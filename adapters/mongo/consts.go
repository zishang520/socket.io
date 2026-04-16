// Package mongo defines constants for MongoDB-based message types used in the Socket.IO adapter.
// These message types are used for inter-node communication in a clustered Socket.IO environment.
package mongo

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
)

// Message types for Socket.IO MongoDB adapter inter-node communication.
// These constants define the different operations that can be performed
// across multiple Socket.IO server nodes using MongoDB as the message broker.
const (
	// SOCKETS requests a list of socket IDs from other nodes.
	SOCKETS adapter.MessageType = iota

	// ALL_ROOMS requests a list of all rooms from other nodes.
	ALL_ROOMS

	// REMOTE_JOIN instructs other nodes to join a socket to specified rooms.
	REMOTE_JOIN

	// REMOTE_LEAVE instructs other nodes to remove a socket from specified rooms.
	REMOTE_LEAVE

	// REMOTE_DISCONNECT instructs other nodes to disconnect a specific socket.
	REMOTE_DISCONNECT

	// REMOTE_FETCH requests detailed socket information from other nodes.
	REMOTE_FETCH

	// SERVER_SIDE_EMIT sends a server-side event to all other nodes.
	SERVER_SIDE_EMIT

	// BROADCAST broadcasts a message to all clients matching specified criteria.
	BROADCAST

	// BROADCAST_CLIENT_COUNT reports the number of matching clients from a node.
	BROADCAST_CLIENT_COUNT

	// BROADCAST_ACK sends a broadcast acknowledgment from a client.
	BROADCAST_ACK
)
