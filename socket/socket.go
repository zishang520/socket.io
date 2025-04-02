package socket

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/engine.io/v2/engine"
	"github.com/zishang520/engine.io/v2/events"
	"github.com/zishang520/engine.io/v2/log"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/engine.io/v2/utils"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

var (
	socket_log                     = log.NewLog("socket.io:socket")
	SOCKET_RESERVED_EVENTS         = types.NewSet("connect", "connect_error", "disconnect", "disconnecting", "newListener", "removeListener")
	RECOVERABLE_DISCONNECT_REASONS = types.NewSet("transport error", "transport close", "forced close", "ping timeout", "server shutting down", "forced server close")
)

type (
	Ack = func([]any, error)

	SocketMiddleware = func([]any, func(error))

	Handshake struct {
		// The headers sent as part of the handshake
		Headers map[string][]string `json:"headers" msgpack:"headers"`
		// The date of creation (as string)
		Time string `json:"time" msgpack:"time"`
		// The ip of the client
		Address string `json:"address" msgpack:"address"`
		// Whether the connection is cross-domain
		Xdomain bool `json:"xdomain" msgpack:"xdomain"`
		// Whether the connection is secure
		Secure bool `json:"secure" msgpack:"secure"`
		// The date of creation (as unix timestamp)
		Issued int64 `json:"issued" msgpack:"issued"`
		// The request URL string
		Url string `json:"url" msgpack:"url"`
		// The query object
		Query map[string][]string `json:"query" msgpack:"query"`
		// The auth object
		Auth any `json:"auth" msgpack:"auth"`
	}

	// This is the main object for interacting with a client.
	//
	// A Socket belongs to a given [Namespace] and uses an underlying [Client] to communicate.
	//
	// Within each [Namespace], you can also define arbitrary channels (called "rooms") that the [Socket] can
	// join and leave. That provides a convenient way to broadcast to a group of socket instances.
	//
	//	io.On("connection", func(args ...any) {
	//		socket := args[0].(*socket.Socket)
	//
	//		utils.Log().Info(`socket %s connected`, socket.Id())
	//
	//		// send an event to the client
	//		socket.Emit("foo", "bar")
	//
	//		socket.On("foobar", func(...any) {
	//			// an event was received from the client
	//		})
	//
	//		// join the room named "room1"
	//		socket.Join("room1")
	//
	//		// broadcast to everyone in the room named "room1"
	//		io.to("room1").Emit("hello")
	//
	//		// upon disconnection
	//		socket.On("disconnect", func(reason ...any) {
	//			utils.Log().Info(`socket %s disconnected due to %s`, socket.Id(), reason[0])
	//		})
	//	})
	Socket struct {
		*StrictEventEmitter

		nsp    Namespace
		client *Client

		// An unique identifier for the session.
		id SocketId
		// Whether the connection state was recovered after a temporary disconnection. In that case, any missed packets will
		// be transmitted to the client, the data attribute and the rooms will be restored.
		recovered bool
		// The handshake details.
		handshake *Handshake

		// Additional information that can be attached to the Socket instance and which will be used in the
		// [Server.fetchSockets()] method.
		data atomic.Pointer[any]

		// Whether the socket is currently connected or not.
		//
		//	io.Use(func(socket *socket.Socket, next func(*ExtendedError)) {
		//		fmt.Println(socket.Connected()) // false
		//		next(nil)
		//	})
		//
		//	io.On("connection", func(args ...any) {
		//		socket := args[0].(*socket.Socket)
		//		fmt.Println(socket.Connected()) // true
		//	})
		connected atomic.Bool

		// The session ID, which must not be shared (unlike [id]).
		pid PrivateSessionId

		// TODO: remove this unused reference
		server                *Server
		adapter               Adapter
		acks                  *types.Map[uint64, Ack]
		fns                   *types.Slice[SocketMiddleware]
		flags                 atomic.Pointer[BroadcastFlags]
		_anyListeners         *types.Slice[events.Listener]
		_anyOutgoingListeners *types.Slice[events.Listener]

		canJoin atomic.Bool
	}
)

func MakeSocket() *Socket {
	s := &Socket{
		StrictEventEmitter: NewStrictEventEmitter(),

		// Initialize default value
		acks:                  &types.Map[uint64, Ack]{},
		fns:                   types.NewSlice[SocketMiddleware](),
		_anyListeners:         types.NewSlice[events.Listener](),
		_anyOutgoingListeners: types.NewSlice[events.Listener](),
	}
	s.flags.Store(&BroadcastFlags{})
	s.canJoin.Store(true)

	return s
}

func NewSocket(nsp Namespace, client *Client, auth any, previousSession *Session) *Socket {
	s := MakeSocket()

	s.Construct(nsp, client, auth, previousSession)

	return s
}

// An unique identifier for the session.
func (s *Socket) Id() SocketId {
	return s.id
}

// Whether the connection state was recovered after a temporary disconnection. In that case, any missed packets will
// be transmitted to the client, the data attribute and the rooms will be restored.
func (s *Socket) Recovered() bool {
	return s.recovered
}

// The handshake details.
func (s *Socket) Handshake() *Handshake {
	return s.handshake
}

// Additional information that can be attached to the Socket instance and which will be used in the
// [Server.fetchSockets()] method.
func (s *Socket) SetData(data any) {
	s.data.Store(&data)
}
func (s *Socket) Data() any {
	if data := s.data.Load(); data != nil {
		return *data
	}
	return nil
}

// Whether the socket is currently connected or not.
//
//	io.Use(func(socket *socket.Socket, next func(*ExtendedError)) {
//		fmt.Println(socket.Connected()) // false
//		next(nil)
//	})
//
//	io.On("connection", func(args ...any) {
//		socket := args[0].(*socket.Socket)
//		fmt.Println(socket.Connected()) // true
//	})
func (s *Socket) Connected() bool {
	return s.connected.Load()
}

func (s *Socket) Acks() *types.Map[uint64, Ack] {
	return s.acks
}

func (s *Socket) Nsp() Namespace {
	return s.nsp
}

func (s *Socket) Client() *Client {
	return s.client
}

func (s *Socket) Construct(nsp Namespace, client *Client, auth any, previousSession *Session) {
	s.nsp = nsp
	s.client = client

	s.server = nsp.Server()
	s.adapter = s.nsp.Adapter()
	if previousSession != nil {
		s.id = previousSession.Sid
		s.pid = previousSession.Pid
		for _, room := range previousSession.Rooms.Keys() {
			s.Join(room)
		}
		s.SetData(previousSession.Data)
		for _, packet := range previousSession.MissedPackets {
			s.packet(&parser.Packet{
				Type: parser.EVENT,
				Data: packet,
			}, nil)
		}
		s.recovered = true
	} else {
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
		if s.server.Opts().GetRawConnectionStateRecovery() != nil {
			id, _ := utils.Base64Id().GenerateId()
			s.pid = PrivateSessionId(id)
		}
	}
	s.handshake = s.buildHandshake(auth)

	// prevents crash when the socket receives an "error" event without listener
	//
	// Golang defines the error by itself. It seems that this logic is not needed?
	s.On("error", func(...any) {})
}

// Builds the `handshake` BC object
func (s *Socket) buildHandshake(auth any) *Handshake {
	return &Handshake{
		Headers: s.Request().Headers().All(),
		Time:    time.Now().Format("2006-01-02 15:04:05"),
		Address: s.Conn().RemoteAddress(),
		Xdomain: s.Request().Headers().Peek("Origin") != "",
		Secure:  s.Request().Secure(),
		Issued:  time.Now().UnixMilli(),
		Url:     s.Request().Request().RequestURI,
		Query:   s.Request().Query().All(),
		Auth:    auth,
	}
}

// Emits to this client.
//
//	io.On("connection", func(args ...any) {
//		socket := args[0].(*socket.Socket)
//		socket.Emit("hello", "world")
//
//		// all serializable datastructures are supported (no need to call json.Marshal, But the map can only be of `map[string]any` type, currently does not support other types of maps)
//		socket.Emit("hello", 1, "2", map[string]any{"3": []string{"4"}, "5": types.NewBytesBuffer([]byte{6})})
//
//		// with an acknowledgement from the client
//		socket.Emit("hello", "world", func(args []any, err error) {
//			// ...
//		})
//	})
func (s *Socket) Emit(ev string, args ...any) error {
	if SOCKET_RESERVED_EVENTS.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
	}
	data := append([]any{ev}, args...)
	data_len := len(data)
	packet := &parser.Packet{
		Type: parser.EVENT,
		Data: data,
	}
	// access last argument to see if it's an ACK callback
	if fn, ok := data[data_len-1].(Ack); ok {
		id := s.nsp.Ids()
		socket_log.Debug("emitting packet with ack id %d", id)
		packet.Data = data[:data_len-1]
		s.registerAckCallback(id, fn)
		packet.Id = &id
	}
	flags := *s.flags.Load()
	s.flags.Store(&BroadcastFlags{})

	if s.nsp.Server().Opts().GetRawConnectionStateRecovery() != nil {
		// this ensures the packet is stored and can be transmitted upon reconnection
		s.adapter.Broadcast(packet, &BroadcastOptions{
			Rooms:  types.NewSet(Room(s.id)),
			Except: types.NewSet[Room](),
			Flags:  &flags,
		})
	} else {
		s.notifyOutgoingListeners(packet)
		s.packet(packet, &flags)
	}

	return nil
}

// Emits an event and waits for an acknowledgement
//
//	io.On("connection", func(args ...any) => {
//		client := args[0].(*socket.Socket)
//		// without timeout
//		client.EmitWithAck("hello", "world")(func(args []any, err error) {
//			if err == nil {
//				fmt.Println(args) // one response per client
//			} else {
//				// some clients did not acknowledge the event in the given delay
//			}
//		})
//
//		// with a specific timeout
//		client.Timeout(1000 * time.Millisecond).EmitWithAck("hello", "world")(func(args []any, err error) {
//			if err == nil {
//				fmt.Println(args) // one response per client
//			} else {
//				// some clients did not acknowledge the event in the given delay
//			}
//		})
//	})
//
// Return:  a `func(socket.Ack)` that will be fulfilled when all clients have acknowledged the event
func (s *Socket) EmitWithAck(ev string, args ...any) func(Ack) {
	return func(ack Ack) {
		s.Emit(ev, append(args, ack)...)
	}
}

func (s *Socket) registerAckCallback(id uint64, ack Ack) {
	timeout := s.flags.Load().Timeout
	if timeout == nil {
		s.acks.Store(id, ack)
		return
	}
	timer := utils.SetTimeout(func() {
		socket_log.Debug("event with ack id %d has timed out after %d ms", id, *timeout/time.Millisecond)
		s.acks.Delete(id)
		ack(nil, errors.New("operation has timed out"))
	}, *timeout)
	s.acks.Store(id, func(args []any, _ error) {
		utils.ClearTimeout(timer)
		ack(args, nil)
	})
}

// Targets a room when broadcasting.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		// the “foo” event will be broadcast to all connected clients in the “room-101” room, except this socket
//		socket.To("room-101").Emit("foo", "bar")
//
//		// the code above is equivalent to:
//		io.To("room-101").Except(Room(socket.Id())).Emit("foo", "bar")
//
//		// with an array of rooms (a client will be notified at most once)
//		socket.To([]Room{"room-101", "room-102"}...).Emit("foo", "bar")
//
//		// with multiple chained calls
//		socket.To("room-101").To("room-102").Emit("foo", "bar")
//	})
//
// Param: Room - a `Room`, or a `Room` slice to expand
//
// Return: a new [BroadcastOperator] instance for chaining
func (s *Socket) To(room ...Room) *BroadcastOperator {
	return s.newBroadcastOperator().To(room...)
}

// Targets a room when broadcasting. Similar to `to()`, but might feel clearer in some cases:
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		// disconnect all clients in the "room-101" room, except this socket
//		socket.In("room-101").DisconnectSockets(false)
//	});
//
// Param: Room - a `Room`, or a `Room` slice to expand
//
// Return: a new [BroadcastOperator] instance for chaining
func (s *Socket) In(room ...Room) *BroadcastOperator {
	return s.newBroadcastOperator().In(room...)
}

// Excludes a room when broadcasting.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		// the "foo" event will be broadcast to all connected clients, except the ones that are in the "room-101" room
//		// and this socket
//		socket.Except("room-101").Emit("foo", "bar")
//
//		// with an array of rooms
//		socket.Except([]Room{"room-101", "room-102"}...).Emit("foo", "bar")
//
//		// with multiple chained calls
//		socket.Except("room-101").Except("room-102").Emit("foo", "bar")
//	})
//
// Param: Room - a `Room`, or a `Room` slice to expand
//
// Return: a new [BroadcastOperator] instance for chaining
func (s *Socket) Except(room ...Room) *BroadcastOperator {
	return s.newBroadcastOperator().Except(room...)
}

// Sends a `message` event.
//
// This method mimics the WebSocket.send() method.
//
// See: https://developer.mozilla.org/en-US/docs/Web/API/WebSocket/send
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		socket.Send("hello");
//
//		// this is equivalent to
//		socket.Emit("message", "hello");
//	});
func (s *Socket) Send(args ...any) *Socket {
	s.Emit("message", args...)
	return s
}

// Sends a `message` event. Alias of Send.
func (s *Socket) Write(args ...any) *Socket {
	s.Emit("message", args...)
	return s
}

// Writes a packet.
//
// Param:  packet - packet struct
//
// Param:  opts - options
func (s *Socket) packet(packet *parser.Packet, opts *BroadcastFlags) {
	packet.Nsp = s.nsp.Name()
	if opts == nil {
		opts = &BroadcastFlags{}
	}
	s.client._packet(packet, &opts.WriteOptions)
}

// Joins a room.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		// join a single room
//		socket.Join("room1")
//
//		// join multiple rooms
//		socket.Join([]Room{"room-101", "room-102"}...)
//	})
//
// Param: Room - a `Room`, or a `Room` slice to expand
func (s *Socket) Join(rooms ...Room) {
	if !s.canJoin.Load() {
		return
	}

	socket_log.Debug("join room %s", rooms)
	s.adapter.AddAll(s.id, types.NewSet(rooms...))
}

// Leaves a room.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		// leave a single room
//		socket.Leave("room1")
//
//		// leave multiple rooms
//		socket.Leave("room-101")
//		socket.Leave("room-102")
//	})
//
// Param: Room - a `Room`, or a `Room` slice to expand
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

	s.connected.Store(true)

	s.Join(Room(s.id))
	if s.Conn().Protocol() == 3 {
		s.packet(&parser.Packet{
			Type: parser.CONNECT,
		}, nil)
	} else {
		s.packet(&parser.Packet{
			Type: parser.CONNECT,
			Data: &struct {
				Sid SocketId         `json:"sid" msgpack:"sid"`
				Pid PrivateSessionId `json:"pid,omitempty" msgpack:"pid,omitempty"`
			}{
				Sid: s.id,
				Pid: s.pid,
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
	case parser.BINARY_EVENT:
		s.onevent(packet)
	case parser.ACK:
		s.onack(packet)
	case parser.BINARY_ACK:
		s.onack(packet)
	case parser.DISCONNECT:
		s.ondisconnect()
	}
}

// Called upon event packet.
//
// Param:  packet - packet struct
func (s *Socket) onevent(packet *parser.Packet) {
	args := packet.Data.([]any)
	socket_log.Debug("emitting event %v", args)
	if nil != packet.Id {
		socket_log.Debug("attaching ack callback to event")
		args = append(args, s.ack(*packet.Id))
	}
	for _, listener := range s._anyListeners.All() {
		listener(args...)
	}
	s.dispatch(args)
}

// Produces an ack callback to emit with an event.
//
// Param: id - packet id
func (s *Socket) ack(id uint64) Ack {
	sent := &sync.Once{}
	return func(args []any, _ error) {
		// prevent double callbacks
		sent.Do(func() {
			socket_log.Debug("sending ack %v", args)
			s.packet(&parser.Packet{
				Id:   &id,
				Type: parser.ACK,
				Data: args,
			}, nil)
		})
	}
}

// Called upon ack packet.
func (s *Socket) onack(packet *parser.Packet) {
	if packet.Id != nil {
		if ack, ok := s.acks.Load(*packet.Id); ok {
			socket_log.Debug("calling ack %d with %v", *packet.Id, packet.Data)
			ack(packet.Data.([]any), nil)
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
	// FIXME the meaning of the "error" event is overloaded:
	//  - it can be sent by the client (`socket.emit("error")`)
	//  - it can be emitted when the connection encounters an error (an invalid packet for example)
	//  - it can be emitted when a packet is rejected in a middleware (`socket.use()`)
	s.EmitReserved("error", err)
}

// Called upon closing. Called by `Client`.
//
// Param: reason
// Param: description
func (s *Socket) _onclose(args ...any) *Socket {
	if !s.Connected() {
		return s
	}
	socket_log.Debug("closing socket - reason %v", args[0])
	s.EmitReserved("disconnecting", args...)

	if s.server.Opts().GetRawConnectionStateRecovery() != nil && RECOVERABLE_DISCONNECT_REASONS.Has(args[0].(string)) {
		socket_log.Debug("connection state recovery is enabled for sid %s", s.id)
		s.adapter.PersistSession(&SessionToPersist{
			Sid:   s.id,
			Pid:   s.pid,
			Rooms: types.NewSet(s.Rooms().Keys()...),
			Data:  s.Data(),
		})
	}
	s._cleanup()
	s.client._remove(s)
	s.connected.Store(false)
	s.EmitReserved("disconnect", args...)
	return nil
}

// Makes the socket leave all the rooms it was part of and prevents it from joining any other room
func (s *Socket) _cleanup() {
	s.leaveAll()
	s.nsp.Remove(s)
	s.canJoin.Store(false)
}

// Produces an `error` packet.
func (s *Socket) _error(err any) {
	s.packet(&parser.Packet{
		Type: parser.CONNECT_ERROR,
		Data: err,
	}, nil)
}

// Disconnects this client.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		// disconnect this socket (the connection might be kept alive for other namespaces)
//		socket.Disconnect(false)
//
//		// disconnect this socket and close the underlying connection
//		socket.Disconnect(true)
//	})
//
// Param: status - if `true`, closes the underlying connection
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
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		socket.Compress(false).Emit("hello")
//	})
//
// Param: compress - if `true`, compresses the sending data
func (s *Socket) Compress(compress bool) *Socket {
	s.flags.Load().Compress = compress
	return s
}

// Sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to
// receive messages (because of network slowness or other issues, or because they’re connected through long polling
// and is in the middle of a request-response cycle).
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		socket.Volatile().Emit("hello") // the client may or may not receive it
//	})
func (s *Socket) Volatile() *Socket {
	s.flags.Load().Volatile = true
	return s
}

// Sets a modifier for a subsequent event emission that the event data will only be broadcast to every sockets but the
// sender.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		// the “foo” event will be broadcast to all connected clients, except this socket
//		socket.Broadcast().Emit("foo", "bar")
//	})
//
// Return: a new [BroadcastOperator] instance for chaining
func (s *Socket) Broadcast() *BroadcastOperator {
	return s.newBroadcastOperator()
}

// Sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		// the “foo” event will be broadcast to all connected clients on this node, except this socket
//		socket.Local().Emit("foo", "bar")
//	})
//
// Return: a new [BroadcastOperator] instance for chaining
func (s *Socket) Local() *BroadcastOperator {
	return s.newBroadcastOperator().Local()
}

// Sets a modifier for a subsequent event emission that the callback will be called with an error when the
// given number of milliseconds have elapsed without an acknowledgement from the client:
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		socket.Timeout(1000 * time.Millisecond).Emit("my-event", func(args []any, err error) {
//			if err != nil {
//				// the client did not acknowledge the event in the given delay
//			}
//		})
//	})
func (s *Socket) Timeout(timeout time.Duration) *Socket {
	s.flags.Load().Timeout = &timeout
	return s
}

// Dispatch incoming event to socket listeners.
func (s *Socket) dispatch(event []any) {
	socket_log.Debug("dispatching an event %v", event)
	s.run(event, func(err error) {
		go func(err error) {
			if err != nil {
				s._onerror(err)
				return
			}
			if s.Connected() {
				s.EmitUntyped(event[0].(string), event[1:]...)
			} else {
				socket_log.Debug("ignore packet received after disconnection")
			}
		}(err)
	})
}

// Sets up socket middleware.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		socket.Use(func(events []any, next func(error)) {
//			if isUnauthorized(events[0]) {
//				next(error.New("unauthorized event"))
//				return
//			}
//			// do not forget to call next
//			next(nil)
//		})
//
//		socket.On("error", func(errs ...any) {
//			if err, ok := errs[0].(error); ok && err.Error() == "unauthorized event" {
//				socket.Disconnect(false)
//			}
//		});
//	});
//
// Param: fn - middleware function (event, next)
func (s *Socket) Use(fn SocketMiddleware) *Socket {
	s.fns.Push(fn)
	return s
}

// Executes the middleware for an incoming event.
//
// Pparam: event - event that will get emitted
//
// Pparam: fn - last fn call in the middleware
func (s *Socket) run(event []any, fn func(error)) {
	fns := s.fns.All()
	if length := len(fns); length > 0 {
		var run func(i int)
		run = func(i int) {
			fns[i](event, func(err error) {
				// upon error, short-circuit
				if err != nil {
					fn(err)
					return
				}
				// if no middleware left, summon callback
				if i >= length-1 {
					fn(nil)
					return
				}
				// go on to next
				run(i + 1)
			})
		}
		run(0)
	} else {
		fn(nil)
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

// A reference to the underlying Client transport connection (Engine.IO Socket interface).
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		fmt.Println(socket.Conn().Transport().Name()) // prints "polling" or "websocket" or "webtransport"
//
//		socket.Conn().Once("upgrade", func(...any) {
//			fmt.Println(socket.Conn().Transport().Name()) // prints "websocket"
//		})
//	})
func (s *Socket) Conn() engine.Socket {
	return s.client.conn
}

// Returns the rooms the socket is currently in.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		fmt.Println(socket.Rooms()) // *types.Set { <socket.id> }
//
//		socket.Join("room1")
//
//		fmt.Println(socket.Rooms()) // *types.Set { <socket.id>, "room1" }
//	})
func (s *Socket) Rooms() *types.Set[Room] {
	if rooms := s.adapter.SocketRooms(s.id); rooms != nil {
		return rooms
	}
	return types.NewSet[Room]()
}

// Adds a listener that will be fired when any event is received. The event name is passed as the first argument to
// the callback.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		socket.OnAny(func(events ...any) {
//			fmt.Println(`got event `, events)
//		})
//	})
//
//	Param: events.Listener
func (s *Socket) OnAny(listener events.Listener) *Socket {
	s._anyListeners.Push(listener)
	return s
}

// Adds a listener that will be fired when any event is received. The event name is passed as the first argument to
// the callback. The listener is added to the beginning of the listeners array.
//
//	Param: events.Listener
func (s *Socket) PrependAny(listener events.Listener) *Socket {
	s._anyListeners.Unshift(listener)
	return s
}

// Removes the listener that will be fired when any event is received.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		catchAllListener := func(events ...any) {
//			fmt.Println(`got event `, events)
//		}
//
//		socket.OnAny(catchAllListener)
//
//		// remove a specific listener
//		socket.OffAny(catchAllListener)
//
//		// or remove all listeners
//		socket.OffAny(nil)
//	})
//
//	Param: events.Listener
func (s *Socket) OffAny(listener events.Listener) *Socket {
	if listener != nil {
		anyListeners := reflect.ValueOf(listener).Pointer()
		s._anyListeners.RangeAndSplice(func(listener events.Listener, i int) (bool, int, int, []events.Listener) {
			return reflect.ValueOf(listener).Pointer() == anyListeners, i, 1, nil
		})
	} else {
		s._anyListeners.Clear()
	}
	return s
}

// Returns an array of listeners that are listening for any event that is specified. This array can be manipulated,
// e.g. to remove listeners.
func (s *Socket) ListenersAny() []events.Listener {
	return s._anyListeners.All()
}

// Adds a listener that will be fired when any event is sent. The event name is passed as the first argument to
// the callback.
//
// Note: acknowledgements sent to the client are not included.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		socket.onAnyOutgoing(func(events ...any) {
//			fmt.Println(`got event `, events)
//		})
//	})
//
//	Param: events.Listener
func (s *Socket) OnAnyOutgoing(listener events.Listener) *Socket {
	s._anyOutgoingListeners.Push(listener)
	return s
}

// Adds a listener that will be fired when any event is emitted. The event name is passed as the first argument to the
// callback. The listener is added to the beginning of the listeners array.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		socket.PrependAnyOutgoing(func(events ...any) {
//			fmt.Println(`sent event `, events)
//		})
//	})
func (s *Socket) PrependAnyOutgoing(listener events.Listener) *Socket {
	s._anyOutgoingListeners.Unshift(listener)
	return s
}

// Removes the listener that will be fired when any event is sent.
//
//	io.On("connection", func(clients ...any) {
//		socket := clients[0].(*socket.Socket)
//		catchAllListener := func(events ...any) {
//			fmt.Println(`sent event `, events)
//		}
//
//		socket.OnAnyOutgoing(catchAllListener)
//
//		// remove a specific listener
//		socket.OffAnyOutgoing(catchAllListener)
//
//		// or remove all listeners
//		socket.OffAnyOutgoing(nil)
//	})
//
//	Param: events.Listener - the catch-all listener
func (s *Socket) OffAnyOutgoing(listener events.Listener) *Socket {
	if listener != nil {
		listenerPointer := reflect.ValueOf(listener).Pointer()
		s._anyOutgoingListeners.RangeAndSplice(func(listener events.Listener, i int) (bool, int, int, []events.Listener) {
			return reflect.ValueOf(listener).Pointer() == listenerPointer, i, 1, nil
		})
	} else {
		s._anyOutgoingListeners.Clear()
	}
	return s
}

// Returns an array of listeners that are listening for any event that is specified. This array can be manipulated,
// e.g. to remove listeners.
func (s *Socket) ListenersAnyOutgoing() []events.Listener {
	return s._anyOutgoingListeners.All()
}

// Notify the listeners for each packet sent (emit or broadcast)
func (s *Socket) notifyOutgoingListeners(packet *parser.Packet) {
	for _, listener := range s._anyOutgoingListeners.All() {
		if args, ok := packet.Data.([]any); ok {
			listener(args...)
		} else {
			listener(packet.Data)
		}
	}
}
func (s *Socket) NotifyOutgoingListeners() func(*parser.Packet) {
	return s.notifyOutgoingListeners
}

func (s *Socket) newBroadcastOperator() *BroadcastOperator {
	flags := *s.flags.Load()
	s.flags.Store(&BroadcastFlags{})
	return NewBroadcastOperator(s.adapter, types.NewSet[Room](), types.NewSet(Room(s.id)), &flags)
}
