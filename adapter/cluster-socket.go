package adapter

import (
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io/v2/socket"
)

// Expose of subset of the attributes and methods of the Socket struct
type ClusterSocket struct {
	id        socket.SocketId
	handshake *socket.Handshake
	rooms     *types.Set[socket.Room]
	data      any
}

func MakeClusterSocket() *ClusterSocket {
	r := &ClusterSocket{}
	return r
}

func NewClusterSocket(details *SocketResponse) *ClusterSocket {
	r := MakeClusterSocket()

	r.Construct(details)

	return r
}

func (r *ClusterSocket) Id() socket.SocketId {
	return r.id
}

func (r *ClusterSocket) Handshake() *socket.Handshake {
	return r.handshake
}

func (r *ClusterSocket) Rooms() *types.Set[socket.Room] {
	return r.rooms
}

func (r *ClusterSocket) Data() any {
	return r.data
}

func (r *ClusterSocket) Construct(details *SocketResponse) {
	r.id = details.Id
	r.handshake = details.Handshake
	r.rooms = types.NewSet(details.Rooms...)
	r.data = details.Data
}
