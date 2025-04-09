package socket

import (
	"time"

	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

type (
	SocketDetails interface {
		Id() SocketId
		Handshake() *Handshake
		Rooms() *types.Set[Room]
		Data() any
	}

	// Expose of subset of the attributes and methods of the Socket struct
	RemoteSocket struct {
		id        SocketId
		handshake *Handshake
		rooms     *types.Set[Room]
		data      any

		operator *BroadcastOperator
	}
)

func MakeRemoteSocket() *RemoteSocket {
	r := &RemoteSocket{}
	return r
}

func NewRemoteSocket(adapter Adapter, details SocketDetails) *RemoteSocket {
	r := MakeRemoteSocket()

	r.Construct(adapter, details)

	return r
}

func (r *RemoteSocket) Id() SocketId {
	return r.id
}

func (r *RemoteSocket) Handshake() *Handshake {
	return r.handshake
}

func (r *RemoteSocket) Rooms() *types.Set[Room] {
	return r.rooms
}

func (r *RemoteSocket) Data() any {
	return r.data
}

func (r *RemoteSocket) Construct(adapter Adapter, details SocketDetails) {
	r.id = details.Id()
	r.handshake = details.Handshake()
	r.rooms = types.NewSet(details.Rooms().Keys()...)
	r.data = details.Data()
	r.operator = NewBroadcastOperator(adapter, types.NewSet(Room(r.id)), types.NewSet[Room](), &BroadcastFlags{
		ExpectSingleResponse: true, // so that remoteSocket.Emit() with acknowledgement behaves like socket.Emit()
	})
}

// Adds a timeout in milliseconds for the next operation.
//
//	io.FetchSockets()(func(sockets []*RemoteSocket, _ error){
//
//		for _, socket := range sockets {
//			if (someCondition) {
//				socket.Timeout(1000 * time.Millisecond).Emit("some-event", func(args []any, err error) {
//					if err != nil {
//						// the client did not acknowledge the event in the given delay
//					}
//				})
//			}
//		}
//
//	})
//	// Note: if possible, using a room instead of looping over all sockets is preferable
//
//	io.Timeout(1000 * time.Millisecond).To(someConditionRoom).Emit("some-event", func(args []any, err error) {
//		// ...
//	})
//
// Param: time.Duration - timeout
func (r *RemoteSocket) Timeout(timeout time.Duration) *BroadcastOperator {
	return r.operator.Timeout(timeout)
}

func (r *RemoteSocket) Emit(ev string, args ...any) error {
	return r.operator.Emit(ev, args...)
}

// Joins a room.
//
// Param: Room - a [Room], or a [Room] slice to expand
func (r *RemoteSocket) Join(room ...Room) {
	r.operator.SocketsJoin(room...)
}

// Leaves a room.
//
// Param: Room - a [Room], or a [Room] slice to expand
func (r *RemoteSocket) Leave(room ...Room) {
	r.operator.SocketsLeave(room...)
}

// Disconnects this client.
//
// Param: close - if `true`, closes the underlying connection
func (r *RemoteSocket) Disconnect(status bool) *RemoteSocket {
	r.operator.DisconnectSockets(status)
	return r
}
