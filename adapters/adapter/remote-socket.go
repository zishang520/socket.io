package adapter

import (
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// Expose of subset of the attributes and methods of the Socket struct
type RemoteSocket struct {
	id        socket.SocketId
	handshake *socket.Handshake
	rooms     *types.Set[socket.Room]
	data      any
}

func MakeRemoteSocket() *RemoteSocket {
	r := &RemoteSocket{}
	return r
}

func NewRemoteSocket(details *SocketResponse) *RemoteSocket {
	r := MakeRemoteSocket()

	r.Construct(details)

	return r
}

func (r *RemoteSocket) Id() socket.SocketId {
	return r.id
}

func (r *RemoteSocket) Handshake() *socket.Handshake {
	return r.handshake
}

func (r *RemoteSocket) Rooms() *types.Set[socket.Room] {
	return r.rooms
}

func (r *RemoteSocket) Data() any {
	return r.data
}

func (r *RemoteSocket) Construct(details *SocketResponse) {
	r.id = details.Id
	r.handshake = details.Handshake
	r.rooms = types.NewSet(details.Rooms...)
	r.data = details.Data
}
