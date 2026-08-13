// Package emitter broadcasts Socket.IO events without a local Socket.IO server.
package emitter

import (
	"strings"

	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

const defaultNamespace = "/"

var emitterLog = log.NewLog("socket.io-emitter")

// Emitter broadcasts messages with classic or sharded Redis Pub/Sub.
type Emitter struct {
	redisClient      *redis.RedisClient
	opts             *EmitterOptions
	broadcastOptions *BroadcastOptions
	nsp              string
}

// MakeEmitter creates an emitter with the root namespace.
func MakeEmitter() *Emitter {
	return &Emitter{opts: DefaultEmitterOptions(), nsp: defaultNamespace}
}

// NewEmitter creates and initializes a Redis Pub/Sub emitter.
func NewEmitter(client *redis.RedisClient, opts *EmitterOptions, nsps ...string) *Emitter {
	e := MakeEmitter()
	e.Construct(client, opts, nsps...)
	return e
}

// Construct initializes the emitter and its Redis channels.
func (e *Emitter) Construct(client *redis.RedisClient, opts *EmitterOptions, nsps ...string) {
	e.redisClient = client
	if opts != nil {
		e.opts.Assign(opts)
	}
	if e.opts.GetRawKey() == nil {
		e.opts.SetKey(DefaultEmitterKey)
	}
	if e.opts.Parser() == nil {
		e.opts.SetParser(utils.MsgPack())
	}
	if len(nsps) > 0 {
		e.nsp = nsps[0]
	}

	key := e.opts.Key()
	e.broadcastOptions = &BroadcastOptions{
		Nsp:              e.nsp,
		BroadcastChannel: key + "#" + e.nsp + "#",
		RequestChannel:   key + "-request#" + e.nsp + "#",
		Parser:           e.opts.Parser(),
		SubscriptionMode: e.opts.SubscriptionMode(),
	}
}

// Of returns an emitter for a namespace, normalizing its leading slash like
// socket.io-redis-emitter.
func (e *Emitter) Of(nsp string) *Emitter {
	if !strings.HasPrefix(nsp, "/") {
		nsp = "/" + nsp
	}
	return NewEmitter(e.redisClient, e.opts, nsp)
}

// Emit broadcasts an event to all matching clients.
func (e *Emitter) Emit(ev string, args ...any) error {
	return e.newBroadcastOperator().Emit(ev, args...)
}

// To targets rooms for the next operation.
func (e *Emitter) To(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().To(rooms...)
}

// In is an alias for To.
func (e *Emitter) In(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.To(rooms...)
}

// Except excludes rooms from the next operation.
func (e *Emitter) Except(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Except(rooms...)
}

// Volatile marks the next operation as volatile.
func (e *Emitter) Volatile() BroadcastOperatorInterface {
	return e.newBroadcastOperator().Volatile()
}

// Compress sets compression for the next operation.
func (e *Emitter) Compress(compress bool) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Compress(compress)
}

// SocketsJoin makes all matching sockets join rooms.
func (e *Emitter) SocketsJoin(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsJoin(rooms...)
}

// SocketsLeave makes all matching sockets leave rooms.
func (e *Emitter) SocketsLeave(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsLeave(rooms...)
}

// DisconnectSockets disconnects all matching sockets.
func (e *Emitter) DisconnectSockets(close bool) error {
	return e.newBroadcastOperator().DisconnectSockets(close)
}

// ServerSideEmit sends an event to all Socket.IO servers in the cluster.
func (e *Emitter) ServerSideEmit(args ...any) error {
	return e.newBroadcastOperator().ServerSideEmit(args...)
}

func (e *Emitter) newBroadcastOperator() BroadcastOperatorInterface {
	if e.opts.Sharded() {
		return NewShardedBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil)
	}
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil)
}
