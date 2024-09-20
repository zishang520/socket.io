package socket

import (
	"fmt"
	"sync/atomic"

	_types "github.com/zishang520/engine.io-go-parser/types"
	"github.com/zishang520/engine.io/v2/events"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

type (
	AdapterBuilder struct {
	}

	adapter struct {
		events.EventEmitter

		// Prototype interface, used to implement interface method rewriting
		_proto_ Adapter

		nsp     Namespace
		rooms   *types.Map[Room, *types.Set[SocketId]]
		sids    *types.Map[SocketId, *types.Set[Room]]
		encoder parser.Encoder
	}
)

func (*AdapterBuilder) New(nsp Namespace) Adapter {
	return NewAdapter(nsp)
}

func MakeAdapter() Adapter {
	a := &adapter{
		EventEmitter: events.New(),

		rooms: &types.Map[Room, *types.Set[SocketId]]{},
		sids:  &types.Map[SocketId, *types.Set[Room]]{},
	}

	a.Prototype(a)

	return a
}

func NewAdapter(nsp Namespace) Adapter {
	n := MakeAdapter()

	n.Construct(nsp)

	return n
}

func (a *adapter) Prototype(_a Adapter) {
	a._proto_ = _a
}

func (a *adapter) Proto() Adapter {
	return a._proto_
}

func (a *adapter) Rooms() *types.Map[Room, *types.Set[SocketId]] {
	return a.rooms
}

func (a *adapter) Sids() *types.Map[SocketId, *types.Set[Room]] {
	return a.sids
}

func (a *adapter) Nsp() Namespace {
	return a.nsp
}

func (a *adapter) Construct(nsp Namespace) {
	a.nsp = nsp
	a.encoder = nsp.Server().Encoder()
}

// To be overridden
func (a *adapter) Init() {
}

// To be overridden
func (a *adapter) Close() {
}

// Returns the number of Socket.IO servers in the cluster
func (a *adapter) ServerCount() int64 {
	return 1
}

// Adds a socket to a list of room.
func (a *adapter) AddAll(id SocketId, rooms *types.Set[Room]) {
	_rooms, _ := a.sids.LoadOrStore(id, types.NewSet[Room]())
	for _, room := range rooms.Keys() {
		_rooms.Add(room)
		ids, ok := a.rooms.LoadOrStore(room, types.NewSet[SocketId]())
		if !ok {
			a.Emit("create-room", room)
		}
		if !ids.Has(id) {
			ids.Add(id)
			a.Emit("join-room", room, id)
		}
	}
}

// Removes a socket from a room.
func (a *adapter) Del(id SocketId, room Room) {
	if rooms, ok := a.sids.Load(id); ok {
		rooms.Delete(room)
	}
	a._del(room, id)
}

func (a *adapter) _del(room Room, id SocketId) {
	if ids, ok := a.rooms.Load(room); ok {
		if ids.Delete(id) {
			a.Emit("leave-room", room, id)
		}
		if ids.Len() == 0 {
			if _, ok := a.rooms.LoadAndDelete(room); ok {
				a.Emit("delete-room", room)
			}
		}
	}
}

// Removes a socket from all rooms it's joined.
func (a *adapter) DelAll(id SocketId) {
	if rooms, ok := a.sids.Load(id); ok {
		for _, room := range rooms.Keys() {
			a._del(room, id)
		}
		a.sids.Delete(id)
	}
}

// Broadcasts a packet.
//
// Options:
//   - `Flags` {*BroadcastFlags} flags for this packet
//   - `Except` {*types.Set[Room]} sids that should be excluded
//   - `Rooms` {*types.Set[Room]} list of rooms to broadcast to
func (a *adapter) Broadcast(packet *parser.Packet, opts *BroadcastOptions) {
	flags := &BroadcastFlags{}
	if opts != nil && opts.Flags != nil {
		flags = opts.Flags
	}

	packetOpts := &WriteOptions{}
	packetOpts.PreEncoded = true
	packetOpts.Volatile = flags.Volatile
	packetOpts.Compress = flags.Compress

	packet.Nsp = a.nsp.Name()
	encodedPackets := a._encode(packet, packetOpts)
	a.apply(opts, func(socket *Socket) {
		if notifyOutgoingListeners := socket.NotifyOutgoingListeners(); notifyOutgoingListeners != nil {
			notifyOutgoingListeners(packet)
		}
		socket.Client().WriteToEngine(encodedPackets, packetOpts)
	})
}

// Broadcasts a packet and expects multiple acknowledgements.
//
// Options:
//   - `Flags` {*BroadcastFlags} flags for this packet
//   - `Except` {*types.Set[Room]} sids that should be excluded
//   - `Rooms` {*types.Set[Room]} list of rooms to broadcast to
func (a *adapter) BroadcastWithAck(packet *parser.Packet, opts *BroadcastOptions, clientCountCallback func(uint64), ack Ack) {
	flags := &BroadcastFlags{}
	if opts != nil && opts.Flags != nil {
		flags = opts.Flags
	}

	packetOpts := &WriteOptions{}
	packetOpts.PreEncoded = true
	packetOpts.Volatile = flags.Volatile
	packetOpts.Compress = flags.Compress

	packet.Nsp = a.nsp.Name()
	// we can use the same id for each packet, since the _ids counter is common (no duplicate)
	id := a.nsp.Ids()
	packet.Id = &id
	encodedPackets := a._encode(packet, packetOpts)
	var clientCount atomic.Uint64
	a.apply(opts, func(socket *Socket) {
		// track the total number of acknowledgements that are expected
		clientCount.Add(1)
		// call the ack callback for each client response
		socket.Acks().Store(*packet.Id, ack)
		if notifyOutgoingListeners := socket.NotifyOutgoingListeners(); notifyOutgoingListeners != nil {
			notifyOutgoingListeners(packet)
		}
		socket.Client().WriteToEngine(encodedPackets, packetOpts)
	})
	clientCountCallback(clientCount.Load())
}

func (a *adapter) _encode(packet *parser.Packet, packetOpts *WriteOptions) []_types.BufferInterface {
	encodedPackets := a.encoder.Encode(packet)

	if len(encodedPackets) == 1 {
		if p, ok := encodedPackets[0].(*_types.StringBuffer); ok {
			// "4" being the "message" packet type in the Engine.IO protocol
			data := _types.NewStringBufferString("4")
			data.Write(p.Bytes())
			// see https://github.com/websockets/ws/issues/617#issuecomment-283002469
			packetOpts.WsPreEncodedFrame = data
		}
	}

	return encodedPackets
}

// Gets a list of sockets by sid.
func (a *adapter) Sockets(rooms *types.Set[Room]) *types.Set[SocketId] {
	sids := types.NewSet[SocketId]()
	a.apply(&BroadcastOptions{Rooms: rooms}, func(socket *Socket) {
		sids.Add(socket.Id())
	})
	return sids
}

// Gets the list of rooms a given socket has joined.
func (a *adapter) SocketRooms(id SocketId) *types.Set[Room] {
	if rooms, ok := a.sids.Load(id); ok {
		return rooms
	}
	return nil
}

// Returns the matching socket instances
func (a *adapter) FetchSockets(opts *BroadcastOptions) func(func([]SocketDetails, error)) {
	return func(callback func([]SocketDetails, error)) {
		sockets := []SocketDetails{}
		a.apply(opts, func(socket *Socket) {
			sockets = append(sockets, socket)
		})
		callback(sockets, nil)
	}
}

// Makes the matching socket instances join the specified rooms
func (a *adapter) AddSockets(opts *BroadcastOptions, rooms []Room) {
	a.apply(opts, func(socket *Socket) {
		socket.Join(rooms...)
	})
}

// Makes the matching socket instances leave the specified rooms
func (a *adapter) DelSockets(opts *BroadcastOptions, rooms []Room) {
	a.apply(opts, func(socket *Socket) {
		for _, room := range rooms {
			socket.Leave(room)
		}
	})
}

// Makes the matching socket instances disconnect
func (a *adapter) DisconnectSockets(opts *BroadcastOptions, status bool) {
	a.apply(opts, func(socket *Socket) {
		socket.Disconnect(status)
	})
}

func (a *adapter) apply(opts *BroadcastOptions, callback func(*Socket)) {
	if opts == nil {
		opts = &BroadcastOptions{
			Rooms:  types.NewSet[Room](),
			Except: types.NewSet[Room](),
		}
	}

	rooms := opts.Rooms
	except := a.computeExceptSids(opts.Except)

	if rooms != nil && rooms.Len() > 0 {
		ids := types.NewSet[SocketId]()
		for _, room := range rooms.Keys() {
			if _ids, ok := a.rooms.Load(room); ok {
				for _, id := range _ids.Keys() {
					if ids.Has(id) || except.Has(id) {
						continue
					}
					if socket, ok := a.nsp.Sockets().Load(id); ok {
						callback(socket)
						ids.Add(id)
					}
				}
			}
		}
	} else {
		a.sids.Range(func(id SocketId, _ *types.Set[Room]) bool {
			if except.Has(id) {
				return true
			}
			if socket, ok := a.nsp.Sockets().Load(id); ok {
				callback(socket)
			}
			return true
		})
	}
}

func (a *adapter) computeExceptSids(exceptRooms *types.Set[Room]) *types.Set[SocketId] {
	exceptSids := types.NewSet[SocketId]()
	if exceptRooms != nil && exceptRooms.Len() > 0 {
		for _, room := range exceptRooms.Keys() {
			if ids, ok := a.rooms.Load(room); ok {
				exceptSids.Add(ids.Keys()...)
			}
		}
	}
	return exceptSids
}

// Send a packet to the other Socket.IO servers in the cluster
func (a *adapter) ServerSideEmit(packet []any) error {
	return fmt.Errorf(`this adapter does not support the ServerSideEmit() functionality`)
}

// Save the client session in order to restore it upon reconnection.
func (a *adapter) PersistSession(session *SessionToPersist) {}

// Restore the session and find the packets that were missed by the client.
func (a *adapter) RestoreSession(pid PrivateSessionId, offset string) (*Session, error) {
	return nil, nil
}
