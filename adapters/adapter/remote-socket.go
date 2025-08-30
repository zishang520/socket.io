package adapter

import (
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// RemoteSocket exposes a subset of the attributes and methods of the Socket struct.
type RemoteSocket struct {
	id        socket.SocketId
	handshake *socket.Handshake
	rooms     *types.Set[socket.Room]
	data      any
}

// MakeRemoteSocket creates a new empty RemoteSocket instance.
func MakeRemoteSocket() *RemoteSocket {
	return &RemoteSocket{}
}

// NewRemoteSocket creates a new RemoteSocket from the given SocketResponse details.
func NewRemoteSocket(details *SocketResponse) *RemoteSocket {
	r := MakeRemoteSocket()

	r.Construct(details)

	return r
}

// Id returns the socket ID of the remote socket.
func (r *RemoteSocket) Id() socket.SocketId {
	return r.id
}

// Handshake returns the handshake information of the remote socket.
func (r *RemoteSocket) Handshake() *socket.Handshake {
	return r.handshake
}

// Rooms returns the set of rooms the remote socket has joined.
func (r *RemoteSocket) Rooms() *types.Set[socket.Room] {
	return r.rooms
}

// Data returns the custom data associated with the remote socket.
func (r *RemoteSocket) Data() any {
	return r.data
}

// Construct initializes the RemoteSocket from the given SocketResponse details.
func (r *RemoteSocket) Construct(details *SocketResponse) {
	r.id = details.Id
	r.handshake = details.Handshake
	r.rooms = types.NewSet(details.Rooms...)
	r.data = details.Data
}
