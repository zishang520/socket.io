package socket

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/engine.io/v2/utils"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

type BroadcastOperator struct {
	adapter     Adapter
	rooms       *types.Set[Room]
	exceptRooms *types.Set[Room]
	flags       *BroadcastFlags
}

func MakeBroadcastOperator() *BroadcastOperator {
	b := &BroadcastOperator{
		rooms:       types.NewSet[Room](),
		exceptRooms: types.NewSet[Room](),
		flags:       &BroadcastFlags{},
	}

	return b
}

func NewBroadcastOperator(adapter Adapter, rooms *types.Set[Room], exceptRooms *types.Set[Room], flags *BroadcastFlags) *BroadcastOperator {
	b := MakeBroadcastOperator()

	b.Construct(adapter, rooms, exceptRooms, flags)

	return b
}

func (b *BroadcastOperator) Construct(adapter Adapter, rooms *types.Set[Room], exceptRooms *types.Set[Room], flags *BroadcastFlags) {
	b.adapter = adapter
	if rooms != nil {
		b.rooms = rooms
	}
	if exceptRooms != nil {
		b.exceptRooms = exceptRooms
	}
	if flags != nil {
		b.flags = flags
	}
}

// Targets a room when emitting.
//
//	// the “foo” event will be broadcast to all connected clients in the “room-101” room
//	io.To("room-101").Emit("foo", "bar")
//
//	// with an array of rooms (a client will be notified at most once)
//	io.To("room-101", "room-102").Emit("foo", "bar")
//	io.To([]Room{"room-101", "room-102"}...).Emit("foo", "bar")
//
//	// with multiple chained calls
//	io.To("room-101").To("room-102").Emit("foo", "bar")
//
// Param: Room - a `Room`, or a `Room` slice to expand
//
// Return: a new [BroadcastOperator] instance for chaining
func (b *BroadcastOperator) To(room ...Room) *BroadcastOperator {
	rooms := types.NewSet(b.rooms.Keys()...)
	rooms.Add(room...)
	return NewBroadcastOperator(b.adapter, rooms, b.exceptRooms, b.flags)
}

// Targets a room when emitting. Similar to `to()`, but might feel clearer in some cases:
//
//	// disconnect all clients in the "room-101" room
//	io.In("room-101").DisconnectSockets(false)
//
// Param: Room - a `Room`, or a `Room` slice to expand
//
// Return: a new [BroadcastOperator] instance for chaining
func (b *BroadcastOperator) In(room ...Room) *BroadcastOperator {
	return b.To(room...)
}

// Excludes a room when emitting.
//
//	// the "foo" event will be broadcast to all connected clients, except the ones that are in the "room-101" room
//	io.Except("room-101").Emit("foo", "bar")
//
//	// with an array of rooms
//	io.Except(["room-101", "room-102"]).Emit("foo", "bar")
//	io.Except([]Room{"room-101", "room-102"}...).Emit("foo", "bar")
//
//	// with multiple chained calls
//	io.Except("room-101").Except("room-102").Emit("foo", "bar")
//
// Param: Room - a `Room`, or a `Room` slice to expand
//
// Return: a new [BroadcastOperator] instance for chaining
func (b *BroadcastOperator) Except(room ...Room) *BroadcastOperator {
	exceptRooms := types.NewSet(b.exceptRooms.Keys()...)
	exceptRooms.Add(room...)
	return NewBroadcastOperator(b.adapter, b.rooms, exceptRooms, b.flags)
}

// Sets the compress flag.
//
//	io.Compress(false).Emit("hello")
//
// Param: compress - if `true`, compresses the sending data
//
// Return: a new [BroadcastOperator] instance
func (b *BroadcastOperator) Compress(compress bool) *BroadcastOperator {
	flags := *b.flags
	flags.Compress = compress
	return NewBroadcastOperator(b.adapter, b.rooms, b.exceptRooms, &flags)
}

// Sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to
// receive messages (because of network slowness or other issues, or because they’re connected through long polling
// and is in the middle of a request-response cycle).
//
//	io.Volatile().Emit("hello") // the clients may or may not receive it
func (b *BroadcastOperator) Volatile() *BroadcastOperator {
	flags := *b.flags
	flags.Volatile = true
	return NewBroadcastOperator(b.adapter, b.rooms, b.exceptRooms, &flags)
}

// Sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node.
//
//	// the “foo” event will be broadcast to all connected clients on this node
//	io.Local().Emit("foo", "bar")
//
// Return: a new [BroadcastOperator] instance for chaining
func (b *BroadcastOperator) Local() *BroadcastOperator {
	flags := *b.flags
	flags.Local = true
	return NewBroadcastOperator(b.adapter, b.rooms, b.exceptRooms, &flags)
}

// Adds a timeout in milliseconds for the next operation
//
//	io.Timeout(1000 * time.Millisecond).Emit("some-event", func(args []any, err error) {
//		if err != nil {
//			// some clients did not acknowledge the event in the given delay
//		} else {
//			fmt.Println(args) // one response per client
//		}
//	})
//
// Param: timeout
func (b *BroadcastOperator) Timeout(timeout time.Duration) *BroadcastOperator {
	flags := *b.flags
	flags.Timeout = &timeout
	return NewBroadcastOperator(b.adapter, b.rooms, b.exceptRooms, &flags)
}

// Emits to all clients.
//
//	// the “foo” event will be broadcast to all connected clients
//	io.Emit("foo", "bar")
//
//	// the “foo” event will be broadcast to all connected clients in the “room-101” room
//	io.To("room-101").Emit("foo", "bar")
//
//	// with an acknowledgement expected from all connected clients
//	io.Timeout(1000 * time.Millisecond).Emit("some-event", func(args []any, err error) {
//		if err != nil {
//			// some clients did not acknowledge the event in the given delay
//		} else {
//			fmt.Println(args) // one response per client
//		}
//	})
func (b *BroadcastOperator) Emit(ev string, args ...any) error {
	if SOCKET_RESERVED_EVENTS.Has(ev) {
		return errors.New(fmt.Sprintf(`"%s" is a reserved event name`, ev))
	}
	// set up packet object
	data := append([]any{ev}, args...)
	data_len := len(data)

	packet := &parser.Packet{
		Type: parser.EVENT,
		Data: data,
	}

	ack, withAck := data[data_len-1].(Ack)

	if !withAck {
		b.adapter.Broadcast(packet, &BroadcastOptions{
			Rooms:  b.rooms,
			Except: b.exceptRooms,
			Flags:  b.flags,
		})

		return nil
	}

	packet.Data = data[:data_len-1]

	var timedOut atomic.Bool
	responses := types.NewSlice[any]()
	var timeout time.Duration

	if time := b.flags.Timeout; time != nil {
		timeout = *time
	}

	timer := utils.SetTimeout(func() {
		timedOut.Store(true)
		if b.flags.ExpectSingleResponse {
			ack(nil, errors.New("operation has timed out"))
		} else {
			ack(responses.All(), errors.New("operation has timed out"))
		}
	}, timeout)

	expectedServerCount := int64(-1)
	var actualServerCount atomic.Int64
	var expectedClientCount atomic.Uint64

	checkCompleteness := func() {
		if !timedOut.Load() && expectedServerCount == actualServerCount.Load() && uint64(responses.Len()) == expectedClientCount.Load() {
			utils.ClearTimeout(timer)
			if b.flags.ExpectSingleResponse {
				data, _ := responses.Get(0)
				ack(data.([]any), nil)
			} else {
				ack(responses.All(), nil)
			}
		}
	}

	b.adapter.BroadcastWithAck(packet, &BroadcastOptions{
		Rooms:  b.rooms,
		Except: b.exceptRooms,
		Flags:  b.flags,
	}, func(clientCount uint64) {
		// each Socket.IO server in the cluster sends the number of clients that were notified
		expectedClientCount.Add(clientCount)
		actualServerCount.Add(1)
		checkCompleteness()
	}, func(clientResponse []any, _ error) {
		// each client sends an acknowledgement
		responses.Push(clientResponse...)
		checkCompleteness()
	})
	expectedServerCount = b.adapter.ServerCount()
	checkCompleteness()
	return nil
}

// Emits an event and waits for an acknowledgement from all clients.
//
//	io.Timeout(1000 * time.Millisecond).EmitWithAck("some-event")(func(args []any, err error) {
//		if err == nil {
//			fmt.Println(args) // one response per client
//		} else {
//			// some clients did not acknowledge the event in the given delay
//		}
//	})
//
// Return:  a `func(socket.Ack)` that will be fulfilled when all clients have acknowledged the event
func (b *BroadcastOperator) EmitWithAck(ev string, args ...any) func(Ack) {
	return func(ack Ack) {
		b.Emit(ev, append(args, ack)...)
	}
}

// Gets a list of clients.
//
// Deprecated: this method will be removed in the next major release, please use [Server#ServerSideEmit] or [FetchSockets] instead.
func (b *BroadcastOperator) AllSockets() (*types.Set[SocketId], error) {
	if b.adapter == nil {
		return nil, errors.New("No adapter for this namespace, are you trying to get the list of clients of a dynamic namespace?")
	}
	return b.adapter.Sockets(b.rooms), nil
}

// Returns the matching socket instances. This method works across a cluster of several Socket.IO servers.
//
// Note: this method also works within a cluster of multiple Socket.IO servers, with a compatible [Adapter].
//
//	io.FetchSockets()(func(sockets []*RemoteSocket, _ error){
//		// return all Socket instances
//	})
//
//	// return all Socket instances in the "room1" room
//	io.In("room1").FetchSockets()(func(sockets []*RemoteSocket, _ error){
//
//		for _, socket := range sockets {
//			fmt.Println(socket.Id())
//			fmt.Println(socket.Handshake())
//			fmt.Println(socket.Rooms())
//			fmt.Println(socket.Data())
//
//			socket.Emit("hello")
//			socket.Join("room1")
//			socket.Leave("room2")
//			socket.Disconnect()
//		}
//
//	})
func (b *BroadcastOperator) FetchSockets() func(func([]*RemoteSocket, error)) {
	return func(callback func([]*RemoteSocket, error)) {
		b.adapter.FetchSockets(&BroadcastOptions{
			Rooms:  b.rooms,
			Except: b.exceptRooms,
			Flags:  b.flags,
		})(func(sockets []SocketDetails, err error) {
			remoteSockets := []*RemoteSocket{}
			for _, socket := range sockets {
				if s, ok := socket.(*RemoteSocket); ok {
					remoteSockets = append(remoteSockets, s)
				} else if sd, sd_ok := socket.(SocketDetails); sd_ok {
					remoteSockets = append(remoteSockets, NewRemoteSocket(b.adapter, sd))
				}
			}
			callback(remoteSockets, err)
		})
	}
}

// Makes the matching socket instances join the specified rooms.
//
// Note: this method also works within a cluster of multiple Socket.IO servers, with a compatible [Adapter].
//
//	// make all socket instances join the "room1" room
//	io.SocketsJoin("room1")
//
//	// make all socket instances in the "room1" room join the "room2" and "room3" rooms
//	io.In("room1").SocketsJoin([]Room{"room2", "room3"}...)
//
// Param: Room - a `Room`, or a `Room` slice to expand
func (b *BroadcastOperator) SocketsJoin(room ...Room) {
	b.adapter.AddSockets(&BroadcastOptions{
		Rooms:  b.rooms,
		Except: b.exceptRooms,
		Flags:  b.flags,
	}, room)
}

// Makes the matching socket instances leave the specified rooms.
//
// Note: this method also works within a cluster of multiple Socket.IO servers, with a compatible [Adapter].
//
//	// make all socket instances leave the "room1" room
//	io.SocketsLeave("room1")
//
//	// make all socket instances in the "room1" room leave the "room2" and "room3" rooms
//	io.In("room1").SocketsLeave([]Room{"room2", "room3"}...)
//
// Param: Room - a `Room`, or a `Room` slice to expand
func (b *BroadcastOperator) SocketsLeave(room ...Room) {
	b.adapter.DelSockets(&BroadcastOptions{
		Rooms:  b.rooms,
		Except: b.exceptRooms,
		Flags:  b.flags,
	}, room)
}

// Makes the matching socket instances disconnect.
//
// Note: this method also works within a cluster of multiple Socket.IO servers, with a compatible [Adapter].
//
//	// make all socket instances disconnect (the connections might be kept alive for other namespaces)
//	io.DisconnectSockets(false)
//
//	// make all socket instances in the "room1" room disconnect and close the underlying connections
//	io.In("room1").DisconnectSockets(true)
//
// Param: close - whether to close the underlying connection
func (b *BroadcastOperator) DisconnectSockets(status bool) {
	b.adapter.DisconnectSockets(&BroadcastOptions{
		Rooms:  b.rooms,
		Except: b.exceptRooms,
		Flags:  b.flags,
	}, status)
}
