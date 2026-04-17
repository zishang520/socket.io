// Package emitter provides an API for broadcasting messages to Socket.IO servers
// via PostgreSQL without requiring a full Socket.IO server instance.
//
// This is useful for sending messages from other processes or services that don't
// run a Socket.IO server but need to communicate with connected clients.
package emitter

import (
	"strings"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/postgres/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
)

const (
	// emitterUID is the unique identifier for messages sent by the emitter.
	emitterUID adapter.ServerId = "emitter"

	// defaultNamespace is the default Socket.IO namespace.
	defaultNamespace = "/"
)

// emitterLog is the logger for the emitter package.
var emitterLog = log.NewLog("socket.io-postgres-emitter")

// Emitter broadcasts messages to Socket.IO servers using PostgreSQL LISTEN/NOTIFY.
// It allows sending events to clients without running a full Socket.IO server.
type Emitter struct {
	postgresClient   *postgres.PostgresClient
	opts             *EmitterOptions
	broadcastOptions *BroadcastOptions
	nsp              string
}

// MakeEmitter creates a new Emitter with default options and the root namespace.
// Call Construct() to complete initialization before use.
func MakeEmitter() *Emitter {
	return &Emitter{
		opts: DefaultEmitterOptions(),
		nsp:  defaultNamespace,
	}
}

// NewEmitter creates and initializes a new Emitter with the given PostgreSQL client and options.
// An optional namespace can be provided; if not specified, the root namespace "/" is used.
func NewEmitter(client *postgres.PostgresClient, opts *EmitterOptions, nsps ...string) *Emitter {
	e := MakeEmitter()
	e.Construct(client, opts, nsps...)
	return e
}

// Construct initializes the Emitter with the given PostgreSQL client, options, and namespace.
// This method sets up the broadcast and request channels based on the configured key prefix.
func (e *Emitter) Construct(client *postgres.PostgresClient, opts *EmitterOptions, nsps ...string) {
	e.postgresClient = client

	// Merge provided options with defaults
	if opts == nil {
		opts = DefaultEmitterOptions()
	}
	e.opts.Assign(opts)

	// Apply default key if not set
	if e.opts.GetRawKey() == nil {
		e.opts.SetKey(DefaultEmitterKey)
	}

	// Apply default table name if not set
	if e.opts.GetRawTableName() == nil {
		e.opts.SetTableName(DefaultTableName)
	}

	// Apply default payload threshold if not set
	if e.opts.GetRawPayloadThreshold() == nil {
		e.opts.SetPayloadThreshold(DefaultPayloadThreshold)
	}

	// Set namespace if provided
	if len(nsps) > 0 && len(nsps[0]) > 0 {
		e.nsp = nsps[0]
	}

	// Configure broadcast options with channel names
	key := e.opts.Key()
	e.broadcastOptions = &BroadcastOptions{
		Nsp:              e.nsp,
		BroadcastChannel: key + "#" + e.nsp,
		TableName:        e.opts.TableName(),
		PayloadThreshold: e.opts.PayloadThreshold(),
	}
}

// Of returns a new Emitter for the specified namespace.
// If the namespace doesn't start with "/", it will be prepended.
func (e *Emitter) Of(nsp string) *Emitter {
	if !strings.HasPrefix(nsp, "/") {
		nsp = "/" + nsp
	}
	return NewEmitter(e.postgresClient, e.opts, nsp)
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
	return e.newBroadcastOperator().In(rooms...)
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

// Compress sets the compress flag for the broadcast.
// When true, the message will be compressed before sending.
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
	return NewBroadcastOperator(e.postgresClient, e.broadcastOptions, nil, nil, nil)
}
