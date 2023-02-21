package socket

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/engine.io/engine"
	"github.com/zishang520/engine.io/events"
	"github.com/zishang520/engine.io/log"
	"github.com/zishang520/engine.io/types"
	"github.com/zishang520/engine.io/utils"
	"github.com/zishang520/socket.io/parser"
)

var (
	SOCKET_RESERVED_EVENTS = types.NewSet("connect", "connect_error", "disconnect", "disconnecting", "newListener", "removeListener")
	socket_log             = log.NewLog("socket.io:socket")
)

type Handshake struct {
	// The headers sent as part of the handshake
	Headers *utils.ParameterBag
	// The date of creation (as string)
	Time string
	// The ip of the client
	Address string
	// Whether the connection is cross-domain
	Xdomain bool
	// Whether the connection is secure
	Secure bool
	// The date of creation (as unix timestamp)
	Issued int64
	// The request URL string
	Url string
	// The query object
	Query *utils.ParameterBag
	// The auth object
	Auth any
}

type Socket struct {
	*StrictEventEmitter

	nsp       *Namespace
	client    *Client
	id        SocketId
	handshake *Handshake

	// Additional information that can be attached to the Socket instance and which will be used in the fetchSockets method
	data    any
	data_mu sync.RWMutex

	connected    bool
	connected_mu sync.RWMutex
	canJoin      bool
	canJoin_mu   sync.RWMutex

	server                *Server
	adapter               Adapter
	acks                  *sync.Map
	fns                   []func([]any, func(error))
	flags                 *BroadcastFlags
	_anyListeners         []events.Listener
	_anyOutgoingListeners []events.Listener

	flags_mu                 sync.RWMutex
	fns_mu                   sync.RWMutex
	_anyListeners_mu         sync.RWMutex
	_anyOutgoingListeners_mu sync.RWMutex
}

func (s *Socket) Nsp() *Namespace {
	return s.nsp
}

func (s *Socket) Id() SocketId {
	return s.id
}

func (s *Socket) Client() *Client {
	return s.client
}

func (s *Socket) Acks() *sync.Map {
	return s.acks
}

func (s *Socket) Handshake() *Handshake {
	return s.handshake
}

func (s *Socket) Connected() bool {
	s.connected_mu.RLock()
	defer s.connected_mu.RUnlock()

	return s.connected
}

func (s *Socket) Data() any {
	s.data_mu.RLock()
	defer s.data_mu.RUnlock()

	return s.data
}

func (s *Socket) SetData(data any) {
	s.data_mu.Lock()
	defer s.data_mu.Unlock()

	s.data = data
}

func NewSocket(nsp *Namespace, client *Client, auth any) *Socket {
	s := &Socket{}
	s.StrictEventEmitter = NewStrictEventEmitter()
	s.nsp = nsp
	s.client = client
	// Additional information that can be attached to the Socket instance and which will be used in the fetchSockets method
	s.data = nil
	s.connected = false
	s.canJoin = true
	s.acks = &sync.Map{}
	s.fns = []func([]any, func(error)){}
	s.flags = &BroadcastFlags{}
	s.server = nsp.Server()
	s.adapter = s.nsp.Adapter()
	if client.conn.Protocol() == 3 {
		if name := nsp.Name(); name != "/" {
			s.id = SocketId(name + "#" + client.id)
		} else {
			s.id = SocketId(client.id)
		}
	} else {
		id, _ := utils.Base64Id().GenerateId()
		s.id = SocketId(id) // don't reuse the Engine.IO id because it's sensitive information
	}
	s.handshake = s.buildHandshake(auth)
	return s
}

// Builds the `handshake` BC object
func (s *Socket) buildHandshake(auth any) *Handshake {
	return &Handshake{
		Headers: s.Request().Headers(),
		Time:    time.Now().Format("2006-01-02 15:04:05"),
		Address: s.Conn().RemoteAddress(),
		Xdomain: s.Request().Headers().Peek("Origin") != "",
		Secure:  s.Request().Secure(),
		Issued:  time.Now().UnixMilli(),
		Url:     s.Request().Request().RequestURI,
		Query:   s.Request().Query(),
		Auth:    auth,
	}
}

// Emits to this client.
func (s *Socket) Emit(ev string, args ...any) error {
	if SOCKET_RESERVED_EVENTS.Has(ev) {
		return errors.New(fmt.Sprintf(`"%s" is a reserved event name`, ev))
	}
	data := append([]any{ev}, args...)
	data_len := len(data)
	packet := &parser.Packet{
		Type: parser.EVENT,
		Data: data,
	}
	// access last argument to see if it's an ACK callback
	if fn, ok := data[data_len-1].(func(...any)); ok {
		id := s.nsp.Ids()
		socket_log.Debug("emitting packet with ack id %d", id)
		packet.Data = data[:data_len-1]
		s.registerAckCallback(id, fn)
		packet.Id = &id
	}
	s.flags_mu.Lock()
	flags := *s.flags
	s.flags = &BroadcastFlags{}
	s.flags_mu.Unlock()
	s.notifyOutgoingListeners(packet)
	s.packet(packet, &flags)
	return nil
}

func (s *Socket) registerAckCallback(id uint64, ack func(...any)) {
	s.flags_mu.RLock()
	timeout := s.flags.Timeout
	s.flags_mu.RUnlock()
	if timeout == nil {
		s.acks.Store(id, ack)
		return
	}
	timer := utils.SetTimeOut(func() {
		socket_log.Debug("event with ack id %d has timed out after %d ms", id, *timeout/time.Millisecond)
		s.acks.Delete(id)
		ack(errors.New("operation has timed out"))
	}, *timeout)
	s.acks.Store(id, func(args ...any) {
		utils.ClearTimeout(timer)
		ack(append([]any{nil}, args...)...)
	})
}

// Targets a room when broadcasting.
func (s *Socket) To(room ...Room) *BroadcastOperator {
	return s.newBroadcastOperator().To(room...)
}

// Targets a room when broadcasting.
func (s *Socket) In(room ...Room) *BroadcastOperator {
	return s.newBroadcastOperator().In(room...)
}

// Excludes a room when broadcasting.
func (s *Socket) Except(room ...Room) *BroadcastOperator {
	return s.newBroadcastOperator().Except(room...)
}

// Sends a `message` event.
func (s *Socket) Send(args ...any) *Socket {
	s.Emit("message", args...)
	return s
}

// Sends a `message` event.
func (s *Socket) Write(args ...any) *Socket {
	s.Emit("message", args...)
	return s
}

// Writes a packet.
func (s *Socket) packet(packet *parser.Packet, opts *BroadcastFlags) {
	packet.Nsp = s.nsp.Name()
	if opts == nil {
		opts = &BroadcastFlags{}
	}
	opts.Compress = false != opts.Compress
	s.client._packet(packet, &opts.WriteOptions)
}

// Joins a room.
func (s *Socket) Join(rooms ...Room) {
	s.canJoin_mu.Lock()
	if !s.canJoin {
		defer s.canJoin_mu.Unlock()
		return
	}
	s.canJoin_mu.Unlock()

	socket_log.Debug("join room %s", rooms)
	s.adapter.AddAll(s.id, types.NewSet(rooms...))
}

// Leaves a room.
func (s *Socket) Leave(room Room) {
	socket_log.Debug("leave room %s", room)
	s.adapter.Del(s.id, room)
}

// Leave all rooms.
func (s *Socket) leaveAll() {
	s.adapter.DelAll(s.id)
}

// Called by `Namespace` upon successful
// middleware execution (ie authorization).
// Socket is added to namespace array before
// call to join, so adapters can access it.
func (s *Socket) _onconnect() {
	socket_log.Debug("socket connected - writing packet")

	s.connected_mu.Lock()
	s.connected = true
	s.connected_mu.Unlock()

	s.Join(Room(s.id))
	if s.Conn().Protocol() == 3 {
		s.packet(&parser.Packet{
			Type: parser.CONNECT,
		}, nil)
	} else {
		s.packet(&parser.Packet{
			Type: parser.CONNECT,
			Data: map[string]any{
				"sid": s.id,
			},
		}, nil)
	}
}

// Called with each packet. Called by `Client`.
func (s *Socket) _onpacket(packet *parser.Packet) {
	socket_log.Debug("got packet %v", packet)
	switch packet.Type {
	case parser.EVENT:
		s.onevent(packet)
		break
	case parser.BINARY_EVENT:
		s.onevent(packet)
		break
	case parser.ACK:
		s.onack(packet)
		break
	case parser.BINARY_ACK:
		s.onack(packet)
		break
	case parser.DISCONNECT:
		s.ondisconnect()
		break
	}
}

// Called upon event packet.
func (s *Socket) onevent(packet *parser.Packet) {
	args := packet.Data.([]any)
	socket_log.Debug("emitting event %v", args)
	if nil != packet.Id {
		socket_log.Debug("attaching ack callback to event")
		args = append(args, s.ack(*packet.Id))
	}
	s._anyListeners_mu.RLock()
	if s._anyListeners != nil && len(s._anyListeners) > 0 {
		listeners := append([]events.Listener{}, s._anyListeners[:]...)
		s._anyListeners_mu.RUnlock()
		for _, listener := range listeners {
			listener(args...)
		}
	} else {
		s._anyListeners_mu.RUnlock()
	}
	s.dispatch(args)
}

// Produces an ack callback to emit with an event.
func (s *Socket) ack(id uint64) func(...any) {
	sent := int32(0)
	return func(args ...any) {
		// prevent double callbacks
		if atomic.CompareAndSwapInt32(&sent, 0, 1) {
			socket_log.Debug("sending ack %v", args)
			s.packet(&parser.Packet{
				Id:   &id,
				Type: parser.ACK,
				Data: args,
			}, nil)
		}
	}
}

// Called upon ack packet.
func (s *Socket) onack(packet *parser.Packet) {
	if packet.Id != nil {
		if ack, ok := s.acks.Load(*packet.Id); ok {
			socket_log.Debug("calling ack %d with %v", *packet.Id, packet.Data)
			(ack.(func(...any)))(packet.Data.([]any)...)
			s.acks.Delete(*packet.Id)
		} else {
			socket_log.Debug("bad ack %d", *packet.Id)
		}
	} else {
		socket_log.Debug("bad ack nil")
	}
}

// Called upon client disconnect packet.
func (s *Socket) ondisconnect() {
	socket_log.Debug("got disconnect packet")
	s._onclose("client namespace disconnect")
}

// Handles a client error.
func (s *Socket) _onerror(err any) {
	if s.ListenerCount("error") > 0 {
		s.EmitReserved("error", err)
	} else {
		utils.Log().Error("Missing error handler on `socket`.")
		utils.Log().Error("%v", err)
	}
}

// Called upon closing. Called by `Client`.
func (s *Socket) _onclose(reason any) *Socket {
	if !s.Connected() {
		return s
	}

	socket_log.Debug("closing socket - reason %v", reason)
	s.EmitReserved("disconnecting", reason)
	s._cleanup()
	s.nsp._remove(s)
	s.client._remove(s)
	s.connected_mu.Lock()
	s.connected = false
	s.connected_mu.Unlock()
	s.EmitReserved("disconnect", reason)
	return nil
}

// Makes the socket leave all the rooms it was part of and prevents it from joining any other room
func (s *Socket) _cleanup() {
	s.leaveAll()
	s.canJoin_mu.Lock()
	s.canJoin = false
	s.canJoin_mu.Unlock()

}

// Produces an `error` packet.
func (s *Socket) _error(err any) {
	s.packet(&parser.Packet{
		Type: parser.CONNECT_ERROR,
		Data: err,
	}, nil)
}

// Disconnects this client.
func (s *Socket) Disconnect(status bool) *Socket {
	if !s.Connected() {
		return s
	}
	if status {
		s.client._disconnect()
	} else {
		s.packet(&parser.Packet{
			Type: parser.DISCONNECT,
		}, nil)
		s._onclose("server namespace disconnect")
	}
	return s
}

// Sets the compress flag.
func (s *Socket) Compress(compress bool) *Socket {
	s.flags_mu.Lock()
	s.flags.Compress = compress
	s.flags_mu.Unlock()
	return s
}

// Sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to
// receive messages (because of network slowness or other issues, or because they’re connected through long polling
// and is in the middle of a request-response cycle).
func (s *Socket) Volatile() *Socket {
	s.flags_mu.Lock()
	s.flags.Volatile = true
	s.flags_mu.Unlock()
	return s
}

// Sets a modifier for a subsequent event emission that the event data will only be broadcast to every sockets but the
// sender.
func (s *Socket) Broadcast() *BroadcastOperator {
	return s.newBroadcastOperator()

}

// Sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node.
func (s *Socket) Local() *BroadcastOperator {
	return s.newBroadcastOperator().Local()
}

// Sets a modifier for a subsequent event emission that the callback will be called with an error when the
// given number of milliseconds have elapsed without an acknowledgement from the client:
//
// ```
//
//	socket.Timeout(5000 * time.Millisecond).Emit("my-event", func(args ...any) {
//	  if args[0] != nil {
//	    // the client did not acknowledge the event in the given delay
//	  }
//	})
//
// ```
func (s *Socket) Timeout(timeout time.Duration) *Socket {
	s.flags_mu.Lock()
	s.flags.Timeout = &timeout
	s.flags_mu.Unlock()
	return s
}

// Dispatch incoming event to socket listeners.
func (s *Socket) dispatch(event []any) {
	socket_log.Debug("dispatching an event %v", event)
	s.run(event, func(err error) {
		if err != nil {
			s._onerror(err)
			return
		}
		if s.Connected() {
			s.EmitUntyped(event[0].(string), event[1:]...)
		} else {
			socket_log.Debug("ignore packet received after disconnection")
		}
	})
}

// Sets up socket middleware.
func (s *Socket) Use(fn func([]any, func(error))) *Socket {
	s.fns_mu.Lock()
	defer s.fns_mu.Unlock()

	s.fns = append(s.fns, fn)
	return s
}

// Executes the middleware for an incoming event.
func (s *Socket) run(event []any, fn func(err error)) {
	s.fns_mu.RLock()
	fns := append([]func([]any, func(error)){}, s.fns...)
	s.fns_mu.RUnlock()
	if length := len(fns); length > 0 {
		var run func(i int)
		run = func(i int) {
			fns[i](event, func(err error) {
				// upon error, short-circuit
				if err != nil {
					go fn(err)
					return
				}
				// if no middleware left, summon callback
				if i >= length-1 {
					go fn(nil)
					return
				}
				// go on to next
				run(i + 1)
			})
		}
		run(0)
	} else {
		go fn(nil)
	}
}

// Whether the socket is currently disconnected
func (s *Socket) Disconnected() bool {
	return !s.Connected()
}

// A reference to the request that originated the underlying Engine.IO Socket.
func (s *Socket) Request() *types.HttpContext {
	return s.client.Request()
}

// A reference to the underlying Client transport connection (Engine.IO Socket object).
func (s *Socket) Conn() engine.Socket {
	return s.client.conn
}

func (s *Socket) Rooms() *types.Set[Room] {
	if rooms := s.adapter.SocketRooms(s.id); rooms != nil {
		return rooms
	}
	return types.NewSet[Room]()
}

// Adds a listener that will be fired when any event is received. The event name is passed as the first argument to
// the callback.
func (s *Socket) OnAny(listener events.Listener) *Socket {
	s._anyListeners_mu.Lock()
	defer s._anyListeners_mu.Unlock()

	if s._anyListeners == nil {
		s._anyListeners = []events.Listener{}
	}
	s._anyListeners = append(s._anyListeners, listener)
	return s
}

// Adds a listener that will be fired when any event is received. The event name is passed as the first argument to
// the callback. The listener is added to the beginning of the listeners array.
func (s *Socket) PrependAny(listener events.Listener) *Socket {
	s._anyListeners_mu.Lock()
	defer s._anyListeners_mu.Unlock()

	if s._anyListeners == nil {
		s._anyListeners = []events.Listener{}
	}
	s._anyListeners = append([]events.Listener{listener}, s._anyListeners...)
	return s
}

// Removes the listener that will be fired when any event is received.
func (s *Socket) OffAny(listener events.Listener) *Socket {
	s._anyListeners_mu.Lock()
	defer s._anyListeners_mu.Unlock()

	if len(s._anyListeners) == 0 {
		return s
	}
	if listener != nil {
		listenerPointer := reflect.ValueOf(listener).Pointer()
		for i, _listener := range s._anyListeners {
			if listenerPointer == reflect.ValueOf(_listener).Pointer() {
				s._anyListeners = append(s._anyListeners[:i], s._anyListeners[i+1:]...)
				return s
			}
		}
	} else {
		s._anyListeners = []events.Listener{}
	}
	return s
}

// Returns an array of listeners that are listening for any event that is specified. This array can be manipulated,
// e.g. to remove listeners.
func (s *Socket) ListenersAny() []events.Listener {
	s._anyListeners_mu.Lock()
	defer s._anyListeners_mu.Unlock()

	if s._anyListeners == nil {
		s._anyListeners = []events.Listener{}
	}
	return s._anyListeners
}

// Adds a listener that will be fired when any event is emitted. The event name is passed as the first argument to the
// callback.
//
// <pre><code>
//
//	socket.OnAnyOutgoing(events.Listener {
//	  fmt.Println(args)
//	})
//
// </pre></code>
func (s *Socket) OnAnyOutgoing(listener events.Listener) *Socket {
	s._anyOutgoingListeners_mu.Lock()
	defer s._anyOutgoingListeners_mu.Unlock()

	if s._anyOutgoingListeners == nil {
		s._anyOutgoingListeners = []events.Listener{}
	}
	s._anyOutgoingListeners = append(s._anyOutgoingListeners, listener)
	return s
}

// Adds a listener that will be fired when any event is emitted. The event name is passed as the first argument to the
// callback. The listener is added to the beginning of the listeners array.
//
// <pre><code>
//
//	socket.PrependAnyOutgoing(events.Listener {
//	  fmt.Println(args)
//	})
//
// </pre></code>
func (s *Socket) PrependAnyOutgoing(listener events.Listener) *Socket {
	s._anyOutgoingListeners_mu.Lock()
	defer s._anyOutgoingListeners_mu.Unlock()

	if s._anyOutgoingListeners == nil {
		s._anyOutgoingListeners = []events.Listener{}
	}
	s._anyOutgoingListeners = append([]events.Listener{listener}, s._anyOutgoingListeners...)
	return s
}

// Removes the listener that will be fired when any event is emitted.
//
// <pre><code>
//
//	handler := func(args ...any) {
//	  fmt.Println(args)
//	}
//
// socket.OnAnyOutgoing(handler)
//
// then later
// socket.OffAnyOutgoing(handler)
//
// </pre></code>
func (s *Socket) OffAnyOutgoing(listener events.Listener) *Socket {
	s._anyOutgoingListeners_mu.Lock()
	defer s._anyOutgoingListeners_mu.Unlock()

	if s._anyOutgoingListeners == nil {
		return s
	}
	if listener != nil {
		listenerPointer := reflect.ValueOf(listener).Pointer()
		for i, _listener := range s._anyOutgoingListeners {
			if listenerPointer == reflect.ValueOf(_listener).Pointer() {
				s._anyOutgoingListeners = append(s._anyOutgoingListeners[:i], s._anyOutgoingListeners[i+1:]...)
				return s
			}
		}
	} else {
		s._anyOutgoingListeners = []events.Listener{}
	}
	return s
}

// Returns an array of listeners that are listening for any event that is specified. This array can be manipulated,
// e.g. to remove listeners.
func (s *Socket) ListenersAnyOutgoing() []events.Listener {
	s._anyOutgoingListeners_mu.Lock()
	defer s._anyOutgoingListeners_mu.Unlock()

	if s._anyOutgoingListeners == nil {
		s._anyOutgoingListeners = []events.Listener{}
	}
	return s._anyOutgoingListeners
}

// Notify the listeners for each packet sent (emit or broadcast)
func (s *Socket) notifyOutgoingListeners(packet *parser.Packet) {
	s._anyOutgoingListeners_mu.RLock()
	if s._anyOutgoingListeners != nil && len(s._anyOutgoingListeners) > 0 {
		listeners := append([]events.Listener{}, s._anyOutgoingListeners[:]...)
		s._anyOutgoingListeners_mu.RUnlock()
		for _, listener := range listeners {
			if args, ok := packet.Data.([]any); ok {
				listener(args...)
			} else {
				listener(packet.Data)
			}
		}
	} else {
		s._anyOutgoingListeners_mu.RUnlock()
	}
}
func (s *Socket) NotifyOutgoingListeners() func(*parser.Packet) {
	return s.notifyOutgoingListeners
}

func (s *Socket) newBroadcastOperator() *BroadcastOperator {
	s.flags_mu.Lock()
	flags := *s.flags
	s.flags = &BroadcastFlags{}
	s.flags_mu.Unlock()
	return NewBroadcastOperator(s.adapter, types.NewSet[Room](), types.NewSet[Room](Room(s.id)), &flags)
}
