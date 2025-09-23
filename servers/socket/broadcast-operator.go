package socket

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// BroadcastOperator is used to broadcast events to multiple clients.
type BroadcastOperator struct {
	adapter     Adapter
	rooms       *types.Set[Room]
	exceptRooms *types.Set[Room]
	flags       *BroadcastFlags
}

// MakeBroadcastOperator creates a new instance of BroadcastOperator.
func MakeBroadcastOperator() *BroadcastOperator {
	b := &BroadcastOperator{
		rooms:       types.NewSet[Room](),
		exceptRooms: types.NewSet[Room](),
		flags:       &BroadcastFlags{},
	}

	return b
}

// NewBroadcastOperator initializes a BroadcastOperator with the given parameters.
func NewBroadcastOperator(adapter Adapter, rooms *types.Set[Room], exceptRooms *types.Set[Room], flags *BroadcastFlags) *BroadcastOperator {
	b := MakeBroadcastOperator()

	b.Construct(adapter, rooms, exceptRooms, flags)

	return b
}

// Construct initializes the BroadcastOperator with the given parameters.
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

// To targets a room when emitting events. Returns a new BroadcastOperator for chaining.
func (b *BroadcastOperator) To(room ...Room) *BroadcastOperator {
	rooms := types.NewSet(b.rooms.Keys()...)
	rooms.Add(room...)
	return NewBroadcastOperator(b.adapter, rooms, b.exceptRooms, b.flags)
}

// In targets a room when emitting events. Alias of To().
func (b *BroadcastOperator) In(room ...Room) *BroadcastOperator {
	return b.To(room...)
}

// Except excludes a room when emitting events. Returns a new BroadcastOperator for chaining.
func (b *BroadcastOperator) Except(room ...Room) *BroadcastOperator {
	exceptRooms := types.NewSet(b.exceptRooms.Keys()...)
	exceptRooms.Add(room...)
	return NewBroadcastOperator(b.adapter, b.rooms, exceptRooms, b.flags)
}

// Compress sets the compress flag for subsequent event emissions.
func (b *BroadcastOperator) Compress(compress bool) *BroadcastOperator {
	flags := *b.flags
	flags.Compress = &compress
	return NewBroadcastOperator(b.adapter, b.rooms, b.exceptRooms, &flags)
}

// Volatile sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to receive messages.
func (b *BroadcastOperator) Volatile() *BroadcastOperator {
	flags := *b.flags
	flags.Volatile = true
	return NewBroadcastOperator(b.adapter, b.rooms, b.exceptRooms, &flags)
}

// Local sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node.
func (b *BroadcastOperator) Local() *BroadcastOperator {
	flags := *b.flags
	flags.Local = true
	return NewBroadcastOperator(b.adapter, b.rooms, b.exceptRooms, &flags)
}

// Timeout adds a timeout for the next operation.
func (b *BroadcastOperator) Timeout(timeout time.Duration) *BroadcastOperator {
	flags := *b.flags
	flags.Timeout = &timeout
	return NewBroadcastOperator(b.adapter, b.rooms, b.exceptRooms, &flags)
}

// Emit broadcasts an event to all connected clients.
func (b *BroadcastOperator) Emit(ev string, args ...any) error {
	if SOCKET_RESERVED_EVENTS.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
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
				ack(utils.TryCast[[]any](data), nil)
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

// EmitWithAck broadcasts an event and waits for acknowledgements from all clients.
func (b *BroadcastOperator) EmitWithAck(ev string, args ...any) func(Ack) {
	return func(ack Ack) {
		b.Emit(ev, append(args, ack)...)
	}
}

// FetchSockets returns a function to fetch matching socket instances.
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
				} else {
					remoteSockets = append(remoteSockets, NewRemoteSocket(b.adapter, socket))
				}
			}
			callback(remoteSockets, err)
		})
	}
}

// SocketsJoin makes the matching socket instances join the specified rooms.
func (b *BroadcastOperator) SocketsJoin(room ...Room) {
	b.adapter.AddSockets(&BroadcastOptions{
		Rooms:  b.rooms,
		Except: b.exceptRooms,
		Flags:  b.flags,
	}, room)
}

// SocketsLeave makes the matching socket instances leave the specified rooms.
func (b *BroadcastOperator) SocketsLeave(room ...Room) {
	b.adapter.DelSockets(&BroadcastOptions{
		Rooms:  b.rooms,
		Except: b.exceptRooms,
		Flags:  b.flags,
	}, room)
}

// DisconnectSockets makes the matching socket instances disconnect.
func (b *BroadcastOperator) DisconnectSockets(status bool) {
	b.adapter.DisconnectSockets(&BroadcastOptions{
		Rooms:  b.rooms,
		Except: b.exceptRooms,
		Flags:  b.flags,
	}, status)
}
