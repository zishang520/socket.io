package socket

import (
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/zishang520/engine.io/v2/log"
	"github.com/zishang520/engine.io/v2/types"
)

var (
	namespace_log = log.NewLog("socket.io:namespace")

	NAMESPACE_RESERVED_EVENTS = types.NewSet("connect", "connection", "new_namespace")
)

// A namespace is a communication channel that allows you to split the logic of your application over a single shared
// connection.
//
// Each namespace has its own:
//
// - event handlers
//
//	io.Of("/orders").On("connection", func(args ...any) {
//		socket := args[0].(*socket.Socket)
//		socket.On("order:list", func(...any){})
//		socket.On("order:create", func(...any){})
//	})
//
//	io.Of("/users").On("connection", func(args ...any) {
//		socket := args[0].(*socket.Socket)
//		socket.On("user:list", func(...any){})
//	})
//
// - rooms
//
//	orderNamespace := io.Of("/orders")
//
//	orderNamespace.On("connection", func(args ...any) {
//		socket := args[0].(*socket.Socket)
//		socket.Join("room1")
//		orderNamespace.To("room1").Emit("hello")
//	})
//
//	userNamespace := io.Of("/users")
//
//	userNamespace.On("connection", func(args ...any) {
//		socket := args[0].(*socket.Socket)
//		socket.Join("room1") // distinct from the room in the "orders" namespace
//		userNamespace.To("room1").Emit("holà")
//	})
//
// - middlewares
//
//	orderNamespace := io.Of("/orders")
//
//	orderNamespace.Use(func(socket *socket.Socket, next func(*socket.ExtendedError)) {
//		// ensure the socket has access to the "orders" namespace
//	})
//
//	userNamespace := io.Of("/users")
//
//	userNamespace.Use(func(socket *socket.Socket, next func(*socket.ExtendedError)) {
//		// ensure the socket has access to the "users" namespace
//	})
type namespace struct {
	*StrictEventEmitter

	// Prototype interface, used to implement interface method rewriting
	_proto_ Namespace

	// #readonly
	// @public

	name    string
	sockets *types.Map[SocketId, *Socket]

	adapter Adapter

	// @private
	_ids atomic.Uint64

	server *Server

	_fns *types.Slice[func(*Socket, func(*ExtendedError))]

	_cleanup func()
}

func MakeNamespace() Namespace {
	n := &namespace{
		StrictEventEmitter: NewStrictEventEmitter(),

		sockets:  &types.Map[SocketId, *Socket]{},
		_fns:     types.NewSlice[func(*Socket, func(*ExtendedError))](),
		_cleanup: nil,
	}

	n.Prototype(n)

	return n
}

func NewNamespace(server *Server, name string) Namespace {
	n := MakeNamespace()

	n.Construct(server, name)

	return n
}

func (n *namespace) Prototype(_n Namespace) {
	n._proto_ = _n
}

func (n *namespace) Proto() Namespace {
	return n._proto_
}

func (n *namespace) EventEmitter() *StrictEventEmitter {
	return n.StrictEventEmitter
}

func (n *namespace) Sockets() *types.Map[SocketId, *Socket] {
	return n.sockets
}

func (n *namespace) Server() *Server {
	return n.server
}

func (n *namespace) Adapter() Adapter {
	return n.adapter
}

func (n *namespace) Name() string {
	return n.name
}

func (n *namespace) Ids() uint64 {
	return n._ids.Add(1)
}

func (n *namespace) Fns() *types.Slice[func(*Socket, func(*ExtendedError))] {
	return n._fns
}

func (n *namespace) Construct(server *Server, name string) {
	n.server = server
	n.name = name
	n.Proto().InitAdapter()
}

// @protected
//
// Initializes the `Adapter` for n nsp.
// Run upon changing adapter by [Server.Adapter]
// in addition to the constructor.
func (n *namespace) InitAdapter() {
	n.adapter = n.server.Adapter().New(n)
}

// Registers a middleware, which is a function that gets executed for every incoming [Socket].
//
//	myNamespace := io.Of("/my-namespace")
//
//	myNamespace.Use(func(socket *socket.Socket, next func(*socket.ExtendedError)) {
//		// ...
//		next(nil)
//	})
//
// Param: func(*ExtendedError) - the middleware function
func (n *namespace) Use(fn func(*Socket, func(*ExtendedError))) Namespace {
	n._fns.Push(fn)
	return n
}

// Executes the middleware for an incoming client.
//
// Param: socket - the socket that will get added
//
// Param: fn - last fn call in the middleware
func (n *namespace) run(socket *Socket, fn func(err *ExtendedError)) {
	fns := n._fns.All()
	if length := len(fns); length > 0 {
		var run func(i int)
		run = func(i int) {
			fns[i](socket, func(err *ExtendedError) {
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

// Targets a room when emitting.
//
//	myNamespace := io.Of("/my-namespace")
//
//	// the “foo” event will be broadcast to all connected clients in the “room-101” room
//	myNamespace.To("room-101").Emit("foo", "bar")
//
//	// with an array of rooms (a client will be notified at most once)
//	myNamespace.To("room-101", "room-102").Emit("foo", "bar")
//	myNamespace.To([]Room{"room-101", "room-102"}...).Emit("foo", "bar")
//
//	// with multiple chained calls
//	myNamespace.To("room-101").To("room-102").Emit("foo", "bar")
//
// Param: Room - a `Room`, or a `Room` slice to expand
//
// Return: a new [BroadcastOperator] instance for chaining
func (n *namespace) To(room ...Room) *BroadcastOperator {
	return NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).To(room...)
}

// Targets a room when emitting.
//
//	myNamespace := io.Of("/my-namespace")
//
//	// disconnect all clients in the "room-101" room
//	myNamespace.In("room-101").DisconnectSockets(false)
//
// Param: Room - a `Room`, or a `Room` slice to expand
//
// Return: a new [BroadcastOperator] instance for chaining
func (n *namespace) In(room ...Room) *BroadcastOperator {
	return NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).In(room...)
}

// Excludes a room when emitting.
//
//	myNamespace := io.Of("/my-namespace")
//
//	// the "foo" event will be broadcast to all connected clients, except the ones that are in the "room-101" room
//	myNamespace.Except("room-101").Emit("foo", "bar")
//
//	// with an array of rooms
//	myNamespace.Except(["room-101", "room-102"]).Emit("foo", "bar")
//	myNamespace.Except([]Room{"room-101", "room-102"}...).Emit("foo", "bar")
//
//	// with multiple chained calls
//	myNamespace.Except("room-101").Except("room-102").Emit("foo", "bar")
//
// Param: Room - a `Room`, or a `Room` slice to expand
//
// Return: a new [BroadcastOperator] instance for chaining
func (n *namespace) Except(room ...Room) *BroadcastOperator {
	return NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).Except(room...)
}

// Adds a new client.
func (n *namespace) Add(client *Client, auth any, fn func(*Socket)) {
	namespace_log.Debug("adding socket to nsp %s", n.name)
	socket := n._createSocket(client, auth)
	if n.server.Opts().ConnectionStateRecovery().SkipMiddlewares() && socket.Recovered() && client.Conn().ReadyState() == "open" {
		n._doConnect(socket, fn)
		return
	}
	// socket := NewSocket(n, client, query)
	n.run(socket, func(err *ExtendedError) {
		go func() {
			if "open" != client.conn.ReadyState() {
				namespace_log.Debug("next called after client was closed - ignoring socket")
				socket._cleanup()
				return
			}
			if err != nil {
				namespace_log.Debug("middleware error, sending CONNECT_ERROR packet to the client")
				socket._cleanup()
				if client.conn.Protocol() == 3 {
					if e := err.Data(); e != nil {
						socket._error(e)
						return
					}
					socket._error(err.Error())
					return
				} else {
					socket._error(map[string]any{
						"message": err.Error(),
						"data":    err.Data(),
					})
					return
				}
			}

			n._doConnect(socket, fn)
		}()
	})
}

func parseSessionData(auth any) (*SessionData, bool) {
	switch v := auth.(type) {
	case map[string]any:
		sd := SessionData{}
		if pid, ok := v["pid"]; ok {
			sd.Pid = pid
		}
		if offset, ok := v["offset"]; ok {
			sd.Offset = offset
		}
		return &sd, true

	case map[string]string:
		sd := SessionData{}
		if pid, ok := v["pid"]; ok {
			sd.Pid = pid
		}
		if offset, ok := v["offset"]; ok {
			sd.Offset = offset
		}
		return &sd, true
	default:
		return nil, false
	}
}

func (n *namespace) _createSocket(client *Client, auth any) *Socket {
	if _auth, ok := parseSessionData(auth); ok {
		sessionId, has_sessionId := _auth.GetPid()
		offset, has_offset := _auth.GetOffset()
		if has_sessionId && has_offset && n.server.Opts().GetRawConnectionStateRecovery() != nil {
			session, err := n.Proto().Adapter().RestoreSession(PrivateSessionId(sessionId), offset)
			if err != nil {
				namespace_log.Debug("error while restoring session: %v", err)
			} else if session != nil {
				namespace_log.Debug("connection state recovered for sid %s", session.Sid)
				return NewSocket(n, client, auth, session)
			}
		}
	}
	return NewSocket(n, client, auth, nil)
}

func (n *namespace) _doConnect(socket *Socket, fn func(*Socket)) {
	// track socket
	n.sockets.Store(socket.Id(), socket)

	// it's paramount that the internal `onconnect` logic
	// fires before user-set events to prevent state order
	// violations (such as a disconnection before the connection
	// logic is complete)
	socket._onconnect()
	if fn != nil {
		fn(socket)
	}

	// fire user-set events
	n.EmitReserved("connect", socket)
	n.EmitReserved("connection", socket)
}

// Whether to remove child namespaces that have no sockets connected to them
func (n *namespace) Cleanup(cleanup func()) {
	n._cleanup = cleanup
}

// @private
//
// Removes a client. Called by each [Socket].
func (n *namespace) Remove(socket *Socket) {
	if _, ok := n.sockets.LoadAndDelete(socket.Id()); !ok {
		namespace_log.Debug("ignoring remove for %s", socket.Id())
	}
	if n._cleanup != nil {
		n._cleanup()
	}
}

// Emits to all clients.
//
//	myNamespace := io.Of("/my-namespace")
//
//	// the “foo” event will be broadcast to all connected clients
//	myNamespace.Emit("foo", "bar")
//
//	// the “foo” event will be broadcast to all connected clients in the “room-101” room
//	myNamespace.To("room-101").Emit("foo", "bar")
//
//	// with an acknowledgement expected from all connected clients
//	myNamespace.Timeout(1000 * time.Millisecond).Emit("some-event", func(args []any, err error) {
//		if err != nil {
//			// some clients did not acknowledge the event in the given delay
//		} else {
//			fmt.Println(args) // one response per client
//		}
//	})
//
// Return: Always nil
func (n *namespace) Emit(ev string, args ...any) error {
	return NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).Emit(ev, args...)
}

// Sends a `message` event to all clients.
//
// This method mimics the WebSocket.send() method.
//
// See: https://developer.mozilla.org/en-US/docs/Web/API/WebSocket/send
//
//	myNamespace := io.Of("/my-namespace")
//
//	myNamespace.Send("hello")
//
//	// this is equivalent to
//	myNamespace.Emit("message", "hello")
func (n *namespace) Send(args ...any) Namespace {
	// This type-cast is needed because EmitEvents likely doesn't have `message` as a key.
	// if you specify the EmitEvents, the type of args will be never.
	n.Emit("message", args...)
	return n
}

// Sends a `message` event to all clients. Sends a `message` event. Alias of [Send].
func (n *namespace) Write(args ...any) Namespace {
	// This type-cast is needed because EmitEvents likely doesn't have `message` as a key.
	// if you specify the EmitEvents, the type of args will be never.
	n.Emit("message", args...)
	return n
}

// Emit a packet to other Socket.IO servers
//
//	myNamespace := io.Of("/my-namespace")
//
//	myNamespace.ServerSideEmit("hello", "world")
//
//	myNamespace.On("hello", func(args ...any) {
//		fmt.Println(args) // prints "world"
//	})
//
//	// acknowledgements (without binary content) are supported too:
//	myNamespace.ServerSideEmit("ping", func(args []any, err error) {
//		if err != nil {
//			// some servers did not acknowledge the event in the given delay
//		} else {
//			fmt.Println(args) // one response per server (except the current one)
//		}
//	})
//
//	myNamespace.On("ping", func(args ...any) {
//		args[0]("pong")
//	})
//
// Param: ev - the event name
//
// Param: args - an slice of arguments, which may include an acknowledgement callback at the end
func (n *namespace) ServerSideEmit(ev string, args ...any) error {
	if NAMESPACE_RESERVED_EVENTS.Has(ev) {
		return errors.New(fmt.Sprintf(`"%s" is a reserved event name`, ev))
	}

	n.Proto().Adapter().ServerSideEmit(append([]any{ev}, args...))

	return nil
}

// Sends a message and expect an acknowledgement from the other Socket.IO servers of the cluster.
//
//	myNamespace := io.Of("/my-namespace")
//
//	myNamespace.Timeout(1000 * time.Millisecond).ServerSideEmitWithAck("some-event")(func(args []any, err error) {
//		if err == nil {
//			fmt.Println(args) // one response per client
//		} else {
//			// some servers did not acknowledge the event in the given delay
//		}
//	})
//
// Return: a `func(socket.Ack)` that will be fulfilled when all servers have acknowledged the event
func (n *namespace) ServerSideEmitWithAck(ev string, args ...any) func(Ack) {
	return func(ack Ack) {
		n.ServerSideEmit(ev, append(args, ack)...)
	}
}

// Called when a packet is received from another Socket.IO server
func (n *namespace) OnServerSideEmit(args []any) {
	// Convert the first argument to a string and pass the rest as args
	if len(args) == 0 {
		return // No arguments provided
	}

	ev, ok := args[0].(string)
	if !ok {
		// Handle error, the first argument should be a string
		return
	}

	// Remove the first argument (event name) and pass the rest
	n.EmitUntyped(ev, args[1:]...)
}

// Gets a list of socket ids.
//
// Deprecated: this method will be removed in the next major release, please use [Namespace#ServerSideEmit] or [Namespace#FetchSockets] instead.
func (n *namespace) AllSockets() (*types.Set[SocketId], error) {
	return NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).AllSockets()
}

// Sets the compress flag.
//
//	io.Compress(false).Emit("hello")
//
// Param: bool - if `true`, compresses the sending data
//
// Return: a new [BroadcastOperator] instance for chaining
func (n *namespace) Compress(compress bool) *BroadcastOperator {
	return NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).Compress(compress)
}

// Sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to
// receive messages (because of network slowness or other issues, or because they’re connected through long polling
// and is in the middle of a request-response cycle).
//
//	myNamespace := io.Of("/my-namespace")
//
//	myNamespace.Volatile().Emit("hello") // the clients may or may not receive it
func (n *namespace) Volatile() *BroadcastOperator {
	return NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).Volatile()
}

// Sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node.
//
//	myNamespace := io.Of("/my-namespace")
//
//	// the “foo” event will be broadcast to all connected clients on this node
//	myNamespace.Local().Emit("foo", "bar")
//
// Return: a new [BroadcastOperator] instance for chaining
func (n *namespace) Local() *BroadcastOperator {
	return NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).Local()
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
func (n *namespace) Timeout(timeout time.Duration) *BroadcastOperator {
	return NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).Timeout(timeout)
}

// Returns the matching socket instances
//
// Note: this method also works within a cluster of multiple Socket.IO servers, with a compatible Adapter.
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
func (n *namespace) FetchSockets() func(func([]*RemoteSocket, error)) {
	return NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).FetchSockets()
}

// Makes the matching socket instances join the specified rooms
//
// Note: this method also works within a cluster of multiple Socket.IO servers, with a compatible Adapter.
//
//	// make all socket instances join the "room1" room
//	io.SocketsJoin("room1")
//
//	// make all socket instances in the "room1" room join the "room2" and "room3" rooms
//	io.In("room1").SocketsJoin([]Room{"room2", "room3"}...)
//
// Param: Room - a `Room`, or a `Room` slice to expand
func (n *namespace) SocketsJoin(room ...Room) {
	NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).SocketsJoin(room...)
}

// Makes the matching socket instances leave the specified rooms
//
// Note: this method also works within a cluster of multiple Socket.IO servers, with a compatible Adapter.
//
//	// make all socket instances leave the "room1" room
//	io.SocketsLeave("room1")
//
//	// make all socket instances in the "room1" room leave the "room2" and "room3" rooms
//	io.In("room1").SocketsLeave([]Room{"room2", "room3"}...)
//
// Param: Room - a `Room`, or a `Room` slice to expand
func (n *namespace) SocketsLeave(room ...Room) {
	NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).SocketsLeave(room...)
}

// Makes the matching socket instances disconnect
//
// Note: this method also works within a cluster of multiple Socket.IO servers, with a compatible Adapter.
//
//	// make all socket instances disconnect (the connections might be kept alive for other namespaces)
//	io.DisconnectSockets(false)
//
//	// make all socket instances in the "room1" room disconnect and close the underlying connections
//	io.In("room1").DisconnectSockets(true)
//
// Param: close - whether to close the underlying connection
func (n *namespace) DisconnectSockets(status bool) {
	NewBroadcastOperator(n.Proto().Adapter(), nil, nil, nil).DisconnectSockets(status)
}
