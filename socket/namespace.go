package socket

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/engine.io/log"
	"github.com/zishang520/engine.io/types"
)

var namespace_log = log.NewLog("socket.io:namespace")

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

var NAMESPACE_RESERVED_EVENTS = types.NewSet("connect", "connection", "new_namespace")

type Namespace struct {
	// _ids has to be first in the struct to guarantee alignment for atomic
	// operations. http://golang.org/pkg/sync/atomic/#pkg-note-BUG
	_ids uint64

	*StrictEventEmitter

	name    string
	sockets *sync.Map
	adapter Adapter
	server  *Server
	_fns    []func(*Socket, func(*ExtendedError))

	_fns_mu sync.RWMutex
}

func (n *Namespace) Sockets() *sync.Map {
	return n.sockets
}

func (n *Namespace) Server() *Server {
	return n.server
}

func (n *Namespace) Adapter() Adapter {
	return n.adapter
}

func (n *Namespace) Name() string {
	return n.name
}

func (n *Namespace) Ids() uint64 {
	return atomic.AddUint64(&n._ids, 1)
}

func (n *Namespace) EventEmitter() *StrictEventEmitter {
	return n.StrictEventEmitter
}

// Namespace constructor.
func NewNamespace(server *Server, name string) *Namespace {
	n := &Namespace{}
	n.StrictEventEmitter = NewStrictEventEmitter()
	n.sockets = &sync.Map{}
	n._fns = []func(*Socket, func(*ExtendedError)){}
	atomic.StoreUint64(&n._ids, 0)
	n.server = server
	n.name = name
	n._initAdapter()

	return n
}

// Initializes the `Adapter` for n nsp.
// Run upon changing adapter by `Server#adapter`
// in addition to the constructor.
func (n *Namespace) _initAdapter() {
	n.adapter = n.server.Adapter().New(n)
}

// Sets up namespace middleware.
func (n *Namespace) Use(fn func(*Socket, func(*ExtendedError))) NamespaceInterface {
	n._fns_mu.Lock()
	defer n._fns_mu.Unlock()

	n._fns = append(n._fns, fn)
	return n
}

// Executes the middleware for an incoming client.
func (n *Namespace) run(socket *Socket, fn func(err *ExtendedError)) {
	n._fns_mu.RLock()
	fns := append([]func(*Socket, func(*ExtendedError)){}, n._fns...)
	n._fns_mu.RUnlock()
	if length := len(fns); length > 0 {
		var run func(i int)
		run = func(i int) {
			fns[i](socket, func(err *ExtendedError) {
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

// Targets a room when emitting.
func (n *Namespace) To(room ...Room) *BroadcastOperator {
	return NewBroadcastOperator(n.adapter, nil, nil, nil).To(room...)
}

// Targets a room when emitting.
func (n *Namespace) In(room ...Room) *BroadcastOperator {
	return NewBroadcastOperator(n.adapter, nil, nil, nil).In(room...)
}

// Excludes a room when emitting.
func (n *Namespace) Except(room ...Room) *BroadcastOperator {
	return NewBroadcastOperator(n.adapter, nil, nil, nil).Except(room...)
}

// Adds a new client.
func (n *Namespace) Add(client *Client, query any, fn func(*Socket)) *Socket {
	namespace_log.Debug("adding socket to nsp %s", n.name)
	socket := NewSocket(n, client, query)
	n.run(socket, func(err *ExtendedError) {
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
	})
	return socket
}

// Removes a client. Called by each `Socket`.
func (n *Namespace) _remove(socket *Socket) {
	if _, ok := n.sockets.LoadAndDelete(socket.Id()); !ok {
		namespace_log.Debug("ignoring remove for %s", socket.Id())
	}
}

// Emits to all clients.
func (n *Namespace) Emit(ev string, args ...any) error {
	return NewBroadcastOperator(n.adapter, nil, nil, nil).Emit(ev, args...)
}

// Sends a `message` event to all clients.
func (n *Namespace) Send(args ...any) NamespaceInterface {
	n.Emit("message", args...)
	return n
}

// Sends a `message` event to all clients.
func (n *Namespace) Write(args ...any) NamespaceInterface {
	n.Emit("message", args...)
	return n
}

// Emit a packet to other Socket.IO servers
func (n *Namespace) ServerSideEmit(ev string, args ...any) error {
	if NAMESPACE_RESERVED_EVENTS.Has(ev) {
		return errors.New(fmt.Sprintf(`"%s" is a reserved event name`, ev))
	}

	n.adapter.ServerSideEmit(ev, args...)

	return nil
}

// Called when a packet is received from another Socket.IO server
func (n *Namespace) _onServerSideEmit(ev string, args ...any) {
	n.EmitUntyped(ev, args...)
}

// Gets a list of clients.
func (n *Namespace) AllSockets() (*types.Set[SocketId], error) {
	return NewBroadcastOperator(n.adapter, nil, nil, nil).AllSockets()
}

// Sets the compress flag.
func (n *Namespace) Compress(compress bool) *BroadcastOperator {
	return NewBroadcastOperator(n.adapter, nil, nil, nil).Compress(compress)
}

// Sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to
// receive messages (because of network slowness or other issues, or because they’re connected through long polling
// and is in the middle of a request-response cycle).
func (n *Namespace) Volatile() *BroadcastOperator {
	return NewBroadcastOperator(n.adapter, nil, nil, nil).Volatile()
}

// Sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node.
func (n *Namespace) Local() *BroadcastOperator {
	return NewBroadcastOperator(n.adapter, nil, nil, nil).Local()
}

// Adds a timeout in milliseconds for the next operation
//
//	io.Timeout(1000 * time.Millisecond).Emit("some-event", func(args ...any) {
//	  // ...
//	})
func (n *Namespace) Timeout(timeout time.Duration) *BroadcastOperator {
	return NewBroadcastOperator(n.adapter, nil, nil, nil).Timeout(timeout)
}

// Returns the matching socket instances
func (n *Namespace) FetchSockets() ([]*RemoteSocket, error) {
	return NewBroadcastOperator(n.adapter, nil, nil, nil).FetchSockets(), nil
}

// Makes the matching socket instances join the specified rooms
func (n *Namespace) SocketsJoin(room ...Room) {
	NewBroadcastOperator(n.adapter, nil, nil, nil).SocketsJoin(room...)
}

// Makes the matching socket instances leave the specified rooms
func (n *Namespace) SocketsLeave(room ...Room) {
	NewBroadcastOperator(n.adapter, nil, nil, nil).SocketsLeave(room...)
}

// Makes the matching socket instances disconnect
func (n *Namespace) DisconnectSockets(status bool) {
	NewBroadcastOperator(n.adapter, nil, nil, nil).DisconnectSockets(status)
}
