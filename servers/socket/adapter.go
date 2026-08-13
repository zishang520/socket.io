package socket

import (
	"fmt"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// AdapterBuilder is a builder for creating Adapter instances.
type AdapterBuilder struct {
}

// adapter is the default implementation of the Adapter interface.
type adapter struct {
	types.EventEmitter

	// Prototype interface, used to implement interface method rewriting
	_proto_ Adapter

	nsp     Namespace
	rooms   *types.Map[Room, *types.Set[SocketId]]
	sids    *types.Map[SocketId, *types.Set[Room]]
	encoder parser.Encoder
}

// New creates a new Adapter for the given Namespace.
func (*AdapterBuilder) New(nsp Namespace) Adapter {
	return NewAdapter(nsp)
}

// MakeAdapter returns a new default Adapter instance.
func MakeAdapter() Adapter {
	a := &adapter{
		EventEmitter: types.NewEventEmitter(),

		rooms: &types.Map[Room, *types.Set[SocketId]]{},
		sids:  &types.Map[SocketId, *types.Set[Room]]{},
	}

	a.Prototype(a)

	return a
}

// NewAdapter creates a new Adapter for the given Namespace.
func NewAdapter(nsp Namespace) Adapter {
	n := MakeAdapter()

	n.Construct(nsp)

	return n
}

// Prototype sets the prototype for the adapter.
func (a *adapter) Prototype(_a Adapter) {
	a._proto_ = _a
}

// Proto returns the prototype of the adapter.
func (a *adapter) Proto() Adapter {
	return a._proto_
}

// Rooms returns the map of rooms and their associated socket IDs.
func (a *adapter) Rooms() *types.Map[Room, *types.Set[SocketId]] {
	return a.rooms
}

// Sids returns the map of socket IDs and their associated rooms.
func (a *adapter) Sids() *types.Map[SocketId, *types.Set[Room]] {
	return a.sids
}

// Nsp returns the namespace associated with the adapter.
func (a *adapter) Nsp() Namespace {
	return a.nsp
}

// Construct initializes the adapter with the given namespace.
func (a *adapter) Construct(nsp Namespace) {
	a.nsp = nsp
	a.encoder = nsp.Server().Encoder()
}

// Init initializes the adapter. To be overridden by custom implementations.
func (a *adapter) Init() {
}

// Close closes the adapter. To be overridden by custom implementations.
func (a *adapter) Close() {
}

// ServerCount returns the number of Socket.IO servers in the cluster.
func (a *adapter) ServerCount() (int64, error) {
	return 1, nil
}

// AddAll adds a socket to a list of rooms.
func (a *adapter) AddAll(id SocketId, rooms *types.Set[Room]) {
	_rooms, ok := a.sids.Load(id)
	if !ok {
		_rooms, _ = a.sids.LoadOrStore(id, types.NewSet[Room]())
	}
	for _, room := range rooms.Keys() {
		_rooms.Add(room)
		ids, ok := a.rooms.Load(room)
		if !ok {
			ids, ok = a.rooms.LoadOrStore(room, types.NewSet[SocketId]())
		}
		if !ok {
			a.Emit("create-room", room)
		}
		if ids.Add(id) {
			a.Emit("join-room", room, id)
		}
	}
}

// Del removes a socket from a room.
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
		if ids.Len() == 0 && a.rooms.CompareAndDelete(room, ids) {
			a.Emit("delete-room", room)
		}
	}
}

// DelAll removes a socket from all rooms it's joined.
func (a *adapter) DelAll(id SocketId) {
	if rooms, ok := a.sids.Load(id); ok {
		for _, room := range rooms.Keys() {
			a._del(room, id)
		}
		a.sids.Delete(id)
	}
}

// Broadcast sends a packet to all matching sockets.
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

// BroadcastWithAck sends a packet and expects multiple acknowledgements.
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
	packet.Id = new(a.nsp.Ids())
	encodedPackets := a._encode(packet, packetOpts)
	var clientCount uint64
	a.apply(opts, func(socket *Socket) {
		// track the total number of acknowledgements that are expected
		clientCount++
		// call the ack callback for each client response
		socket.Acks().Store(*packet.Id, ack)
		if notifyOutgoingListeners := socket.NotifyOutgoingListeners(); notifyOutgoingListeners != nil {
			notifyOutgoingListeners(packet)
		}
		socket.Client().WriteToEngine(encodedPackets, packetOpts)
	})
	clientCountCallback(clientCount)
}

func (a *adapter) _encode(packet *parser.Packet, packetOpts *WriteOptions) []types.BufferInterface {
	encodedPackets := a.encoder.Encode(packet)

	if len(encodedPackets) == 1 {
		if p, ok := encodedPackets[0].(*types.StringBuffer); ok {
			// "4" being the "message" packet type in the Engine.IO protocol
			payload := p.Bytes()
			data := make([]byte, len(payload)+1)
			data[0] = '4'
			copy(data[1:], payload)
			// see https://github.com/websockets/ws/issues/617#issuecomment-283002469
			packetOpts.WsPreEncodedFrame = newBroadcastFrame(types.NewStringBuffer(data))
		}
	}

	return encodedPackets
}

// Sockets returns a set of socket IDs matching the given rooms.
func (a *adapter) Sockets(rooms *types.Set[Room]) *types.Set[SocketId] {
	sids := types.NewSet[SocketId]()
	a.apply(&BroadcastOptions{Rooms: rooms}, func(socket *Socket) {
		sids.Add(socket.Id())
	})
	return sids
}

// SocketRooms returns the list of rooms a given socket has joined.
func (a *adapter) SocketRooms(id SocketId) *types.Set[Room] {
	if rooms, ok := a.sids.Load(id); ok {
		return rooms
	}
	return nil
}

// FetchSockets returns a function to fetch matching socket instances.
func (a *adapter) FetchSockets(opts *BroadcastOptions) func(func([]SocketDetails, error)) {
	return func(callback func([]SocketDetails, error)) {
		sockets := []SocketDetails{}
		a.apply(opts, func(socket *Socket) {
			sockets = append(sockets, socket)
		})
		callback(sockets, nil)
	}
}

// AddSockets makes the matching socket instances join the specified rooms.
func (a *adapter) AddSockets(opts *BroadcastOptions, rooms []Room) {
	a.apply(opts, func(socket *Socket) {
		socket.Join(rooms...)
	})
}

// DelSockets makes the matching socket instances leave the specified rooms.
func (a *adapter) DelSockets(opts *BroadcastOptions, rooms []Room) {
	a.apply(opts, func(socket *Socket) {
		for _, room := range rooms {
			socket.Leave(room)
		}
	})
}

// DisconnectSockets makes the matching socket instances disconnect.
func (a *adapter) DisconnectSockets(opts *BroadcastOptions, status bool) {
	a.apply(opts, func(socket *Socket) {
		socket.Disconnect(status)
	})
}

func (a *adapter) apply(opts *BroadcastOptions, callback func(*Socket)) {
	var roomKeys []Room
	var except *types.Set[SocketId]
	if opts != nil {
		except = a.computeExceptSids(opts.Except)
		if opts.Rooms != nil {
			roomKeys = opts.Rooms.Keys()
		}
	}

	if len(roomKeys) == 0 {
		a.sids.Range(func(id SocketId, _ *types.Set[Room]) bool {
			if except != nil && except.Has(id) {
				return true
			}
			if socket, ok := a.nsp.Sockets().Load(id); ok && socket.Connected() {
				callback(socket)
			}
			return true
		})
		return
	}

	var seen *types.Set[SocketId]
	if len(roomKeys) > 1 {
		seen = types.NewSet[SocketId]()
	}
	for _, room := range roomKeys {
		roomIds, ok := a.rooms.Load(room)
		if !ok {
			continue
		}
		for _, id := range roomIds.Keys() {
			if seen != nil && seen.Has(id) {
				continue
			}
			if except != nil && except.Has(id) {
				continue
			}
			socket, ok := a.nsp.Sockets().Load(id)
			if !ok {
				continue
			}
			if socket.Connected() {
				callback(socket)
			}
			if seen != nil {
				seen.Add(id)
			}
		}
	}
}

func (a *adapter) computeExceptSids(exceptRooms *types.Set[Room]) *types.Set[SocketId] {
	if exceptRooms == nil {
		return nil
	}
	roomKeys := exceptRooms.Keys()
	if len(roomKeys) == 0 {
		return nil
	}

	exceptSids := types.NewSet[SocketId]()
	for _, room := range roomKeys {
		ids, ok := a.rooms.Load(room)
		if !ok {
			continue
		}
		exceptSids.Add(ids.Keys()...)
	}
	return exceptSids
}

// ServerSideEmit sends a packet to the other Socket.IO servers in the cluster.
func (a *adapter) ServerSideEmit(packet []any) error {
	return fmt.Errorf(`this adapter does not support the ServerSideEmit() functionality`)
}

// PersistSession saves the client session to restore it upon reconnection.
func (a *adapter) PersistSession(session *SessionToPersist) {
}

// RestoreSession restores the session and finds the packets missed by the client.
func (a *adapter) RestoreSession(pid PrivateSessionId, offset string) (*Session, error) {
	return nil, nil
}
