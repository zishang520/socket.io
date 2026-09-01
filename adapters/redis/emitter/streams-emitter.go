package emitter

import (
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
)

var redisStreamsEmitterLog = log.NewLog("socket.io-redis-streams-emitter")

// RedisStreamsEmitter broadcasts messages through a Redis stream.
type RedisStreamsEmitter struct {
	redisClient *redis.RedisClient
	opts        *RedisStreamsEmitterOptions
	nsp         string
}

// MakeRedisStreamsEmitter creates a Redis Streams emitter with default options.
func MakeRedisStreamsEmitter() *RedisStreamsEmitter {
	return &RedisStreamsEmitter{
		opts: DefaultRedisStreamsEmitterOptions(),
		nsp:  defaultNamespace,
	}
}

// NewRedisStreamsEmitter creates a Redis Streams emitter.
func NewRedisStreamsEmitter(client *redis.RedisClient, opts *RedisStreamsEmitterOptions, nsps ...string) *RedisStreamsEmitter {
	e := MakeRedisStreamsEmitter()
	e.Construct(client, opts, nsps...)
	return e
}

// Construct initializes the Redis Streams emitter.
func (e *RedisStreamsEmitter) Construct(client *redis.RedisClient, opts *RedisStreamsEmitterOptions, nsps ...string) {
	e.redisClient = client
	e.opts.Assign(opts)
	if e.opts.GetRawStreamName() == nil {
		e.opts.SetStreamName(DefaultStreamName)
	}
	if e.opts.GetRawStreamCount() == nil {
		e.opts.SetStreamCount(DefaultStreamCount)
	}
	if e.opts.GetRawMaxLen() == nil {
		e.opts.SetMaxLen(DefaultStreamMaxLen)
	}
	if len(nsps) > 0 {
		e.nsp = nsps[0]
	}
}

// Of returns a Redis Streams emitter for a namespace.
func (e *RedisStreamsEmitter) Of(nsp string) *RedisStreamsEmitter {
	return NewRedisStreamsEmitter(e.redisClient, e.opts, nsp)
}

// Emit broadcasts an event to all matching clients.
func (e *RedisStreamsEmitter) Emit(ev string, args ...any) error {
	return e.newBroadcastOperator().Emit(ev, args...)
}

// To targets rooms for the next operation.
func (e *RedisStreamsEmitter) To(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().To(rooms...)
}

// In is an alias for To.
func (e *RedisStreamsEmitter) In(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.To(rooms...)
}

// Except excludes rooms from the next operation.
func (e *RedisStreamsEmitter) Except(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Except(rooms...)
}

// Volatile marks the next operation as volatile.
func (e *RedisStreamsEmitter) Volatile() BroadcastOperatorInterface {
	return e.newBroadcastOperator().Volatile()
}

// Compress sets compression for the next operation.
func (e *RedisStreamsEmitter) Compress(compress bool) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Compress(compress)
}

// SocketsJoin makes all matching sockets join rooms.
func (e *RedisStreamsEmitter) SocketsJoin(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsJoin(rooms...)
}

// SocketsLeave makes all matching sockets leave rooms.
func (e *RedisStreamsEmitter) SocketsLeave(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsLeave(rooms...)
}

// DisconnectSockets disconnects all matching sockets.
func (e *RedisStreamsEmitter) DisconnectSockets(close bool) error {
	return e.newBroadcastOperator().DisconnectSockets(close)
}

// ServerSideEmit sends an event to all Socket.IO servers in the cluster.
func (e *RedisStreamsEmitter) ServerSideEmit(args ...any) error {
	return e.newBroadcastOperator().ServerSideEmit(args...)
}

func (e *RedisStreamsEmitter) newBroadcastOperator() *RedisStreamsBroadcastOperator {
	return NewRedisStreamsBroadcastOperator(e.redisClient, e.nsp, e.opts, nil, nil, nil)
}
