// Package emitter provides an API for broadcasting messages to Socket.IO servers
// via Unix Domain Sockets without requiring a full Socket.IO server instance.
//
// This is useful for sending messages from other processes or services that don't
// run a Socket.IO server but need to communicate with connected clients.
package emitter

import (
	"strings"

	"github.com/zishang520/socket.io/adapters/unix/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// defaultNamespace is the default Socket.IO namespace.
const defaultNamespace = "/"

// Emitter broadcasts messages to Socket.IO servers using Unix Domain Sockets.
// It allows sending events to clients without running a full Socket.IO server.
type Emitter struct {
	unixClient       *unix.UnixClient
	broadcastOptions *BroadcastOptions
}

// MakeEmitter creates an uninitialized Emitter for the root namespace.
// Call Construct() to complete initialization before use.
func MakeEmitter() *Emitter {
	return &Emitter{}
}

// NewEmitter creates and initializes a new Emitter with the given Unix client.
// An optional namespace can be provided; if not specified, the root namespace "/" is used.
func NewEmitter(client *unix.UnixClient, nsps ...string) *Emitter {
	e := MakeEmitter()
	e.Construct(client, nsps...)
	return e
}

// Construct initializes the Emitter with the given Unix client and namespace.
func (e *Emitter) Construct(client *unix.UnixClient, nsps ...string) {
	e.unixClient = client

	nsp := defaultNamespace
	if len(nsps) > 0 {
		nsp = nsps[0]
	}

	e.broadcastOptions = &BroadcastOptions{
		Nsp: nsp,
	}
}

// Of returns a new Emitter for the specified namespace.
// If the namespace doesn't start with "/", it will be prepended.
func (e *Emitter) Of(nsp string) *Emitter {
	if !strings.HasPrefix(nsp, "/") {
		nsp = "/" + nsp
	}
	return NewEmitter(e.unixClient, nsp)
}

// Emit broadcasts an event to all clients in the namespace.
// Returns an error if the event emission fails.
func (e *Emitter) Emit(ev string, args ...any) error {
	return e.newBroadcastOperator().Emit(ev, args...)
}

// To targets specific room(s) for event emission.
// Returns a BroadcastOperatorInterface for method chaining.
func (e *Emitter) To(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().To(rooms...)
}

// In is an alias for To, targeting specific room(s) for event emission.
func (e *Emitter) In(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.To(rooms...)
}

// Except excludes specific room(s) from event emission.
// Returns a BroadcastOperatorInterface for method chaining.
func (e *Emitter) Except(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Except(rooms...)
}

// Volatile sets a flag indicating the event data may be lost if the client
// is not ready to receive messages (e.g., due to network issues).
func (e *Emitter) Volatile() BroadcastOperatorInterface {
	return e.newBroadcastOperator().Volatile()
}

// Compress sets the client-facing transport compression preference for the
// broadcast. It does not compress the Unix cluster frame.
func (e *Emitter) Compress(compress bool) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Compress(compress)
}

// SocketsJoin makes all matching socket instances join the specified rooms.
// This sends a request to all Socket.IO servers in the cluster.
func (e *Emitter) SocketsJoin(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsJoin(rooms...)
}

// SocketsLeave makes all matching socket instances leave the specified rooms.
// This sends a request to all Socket.IO servers in the cluster.
func (e *Emitter) SocketsLeave(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsLeave(rooms...)
}

// DisconnectSockets disconnects all matching socket instances.
// If state is true, the underlying connection will be closed.
func (e *Emitter) DisconnectSockets(state bool) error {
	return e.newBroadcastOperator().DisconnectSockets(state)
}

// ServerSideEmit sends a message to all Socket.IO servers in the cluster.
// Note: Acknowledgements are not supported when using the emitter.
func (e *Emitter) ServerSideEmit(args ...any) error {
	return e.newBroadcastOperator().ServerSideEmit(args...)
}

// newBroadcastOperator creates a new broadcast operator with the emitter's configuration.
func (e *Emitter) newBroadcastOperator() BroadcastOperatorInterface {
	return NewBroadcastOperator(e.unixClient, e.broadcastOptions, nil, nil, nil)
}
