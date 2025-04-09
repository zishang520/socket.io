package socket

import (
	"time"

	"github.com/zishang520/socket.io/servers/engine/v3/events"
	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

type NamespaceMiddleware = func(*Socket, func(*ExtendedError))

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
type Namespace interface {
	On(string, ...events.Listener) error
	Once(string, ...events.Listener) error
	EmitReserved(string, ...any)
	EmitUntyped(string, ...any)
	Listeners(string) []events.Listener

	// #prototype

	Prototype(Namespace)
	Proto() Namespace

	// #getters

	EventEmitter() *StrictEventEmitter
	Sockets() *types.Map[SocketId, *Socket]
	Server() *Server
	Adapter() Adapter
	Name() string
	Ids() uint64
	Fns() *types.Slice[NamespaceMiddleware]

	// Construct() should be called after calling Prototype()
	Construct(*Server, string)

	// @protected
	//
	// Initializes the `Adapter` for n nsp.
	// Run upon changing adapter by `Server.Adapter`
	// in addition to the constructor.
	InitAdapter()

	// Whether to remove child namespaces that have no sockets connected to them
	Cleanup(types.Callable)

	// Sets up namespace middleware.
	Use(NamespaceMiddleware) Namespace

	// Targets a room when emitting.
	To(...Room) *BroadcastOperator

	// Targets a room when emitting.
	In(...Room) *BroadcastOperator

	// Excludes a room when emitting.
	Except(...Room) *BroadcastOperator

	// Adds a new client.
	Add(*Client, any, func(*Socket))

	// Emits to all clients.
	Emit(string, ...any) error

	// Sends a `message` event to all clients.
	Send(...any) Namespace

	// Sends a `message` event to all clients.
	Write(...any) Namespace

	// Emit a packet to other Socket.IO servers
	ServerSideEmit(string, ...any) error

	// Sends a message and expect an acknowledgement from the other Socket.IO servers of the cluster.
	ServerSideEmitWithAck(string, ...any) func(Ack) error

	// @private
	//
	// Called when a packet is received from another Socket.IO server
	OnServerSideEmit([]any)

	// Gets a list of clients.
	AllSockets() (*types.Set[SocketId], error)

	// Sets the compress flag.
	Compress(bool) *BroadcastOperator

	// Sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to
	// receive messages (because of network slowness or other issues, or because they’re connected through long polling
	// and is in the middle of a request-response cycle).
	Volatile() *BroadcastOperator

	// Sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node.
	Local() *BroadcastOperator

	// Adds a timeout in milliseconds for the next operation
	Timeout(time.Duration) *BroadcastOperator

	// Returns the matching socket instances
	//
	// Deprecated: this method will be removed in the next major release, please use [Server.ServerSideEmit] or [BroadcastOperator.FetchSockets] instead.
	FetchSockets() func(func([]*RemoteSocket, error))

	// Makes the matching socket instances join the specified rooms
	SocketsJoin(...Room)

	// Makes the matching socket instances leave the specified rooms
	SocketsLeave(...Room)

	// Makes the matching socket instances disconnect
	DisconnectSockets(bool)

	// Removes a client. Called by each [Socket].
	Remove(*Socket)
}

type ParentNamespace interface {
	Namespace

	Children() *types.Set[Namespace]
	CreateChild(string) Namespace
}

type ExtendedError struct {
	message string
	data    any
}

func NewExtendedError(message string, data any) *ExtendedError {
	return &ExtendedError{message: message, data: data}
}

func (e *ExtendedError) Err() error {
	return e
}

func (e *ExtendedError) Data() any {
	return e.data
}

func (e *ExtendedError) Error() string {
	return e.message
}

type SessionData struct {
	Pid    any `json:"pid" msgpack:"pid"`
	Offset any `json:"offset" msgpack:"offset"`
}

func (s *SessionData) GetPid() (pid string, ok bool) {
	if s != nil && s.Pid != nil {
		switch _pid := s.Pid.(type) {
		case []string:
			if l := len(_pid); l > 0 {
				pid = _pid[l-1]
			}
		case string:
			pid = _pid
		}
	}
	return pid, len(pid) > 0
}

func (s *SessionData) GetOffset() (offset string, ok bool) {
	if s != nil && s.Offset != nil {
		switch _offset := s.Offset.(type) {
		case []string:
			if l := len(_offset); l > 0 {
				offset = _offset[l-1]
			}
		case string:
			offset = _offset
		}
	}
	return offset, len(offset) > 0
}
