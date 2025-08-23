// Package emitter provides an API for broadcasting messages to Socket.IO servers via Redis.
package emitter

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

const UID adapter.ServerId = "emitter"

var emitter_log = log.NewLog("socket.io-emitter")

// Emitter is responsible for broadcasting messages to Socket.IO servers using Redis.
type Emitter struct {
	redisClient *redis.RedisClient

	opts             *EmitterOptions
	broadcastOptions *BroadcastOptions
	nsp              string
}

// MakeEmitter creates a new Emitter with default options and namespace.
func MakeEmitter() *Emitter {
	e := &Emitter{
		opts: DefaultEmitterOptions(),
		nsp:  "/",
	}

	return e
}

// NewEmitter creates a new Emitter with the given Redis client, options, and optional namespace.
func NewEmitter(redisClient *redis.RedisClient, opts *EmitterOptions, nsps ...string) *Emitter {
	e := MakeEmitter()

	e.Construct(redisClient, opts, nsps...)

	return e
}

// Construct initializes the Emitter with the given Redis client, options, and namespace.
func (e *Emitter) Construct(redisClient *redis.RedisClient, opts *EmitterOptions, nsps ...string) {
	e.redisClient = redisClient

	if opts == nil {
		opts = DefaultEmitterOptions()
	}
	e.opts.Assign(opts)

	if e.opts.GetRawKey() == nil {
		e.opts.SetKey("socket.io")
	}

	if e.opts.Parser() == nil {
		e.opts.SetParser(utils.MsgPack())
	}

	if len(nsps) > 0 && len(nsps[0]) > 0 {
		e.nsp = nsps[0]
	}

	e.broadcastOptions = &BroadcastOptions{
		Nsp:              e.nsp,
		BroadcastChannel: e.opts.Key() + "#" + e.nsp + "#",
		RequestChannel:   e.opts.Key() + "-request#" + e.nsp + "#",
		Parser:           e.opts.Parser(),
	}
}

// Of returns a new Emitter for the given namespace.
func (e *Emitter) Of(nsp string) *Emitter {
	if !strings.HasPrefix(nsp, "/") {
		nsp = "/" + nsp
	}
	return NewEmitter(e.redisClient, e.opts, nsp)
}

// Emit emits an event to all clients.
func (e *Emitter) Emit(ev string, args ...any) error {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).Emit(ev, args...)
}

// To targets a room when emitting.
func (e *Emitter) To(rooms ...socket.Room) *BroadcastOperator {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).To(rooms...)
}

// In targets a room when emitting.
func (e *Emitter) In(rooms ...socket.Room) *BroadcastOperator {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).In(rooms...)
}

// Except excludes a room when emitting.
func (e *Emitter) Except(rooms ...socket.Room) *BroadcastOperator {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).Except(rooms...)
}

// Volatile sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to receive messages.
func (e *Emitter) Volatile() *BroadcastOperator {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).Volatile()
}

// Compress sets the compress flag for sending data.
func (e *Emitter) Compress(compress bool) *BroadcastOperator {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).Compress(compress)
}

// SocketsJoin makes the matching socket instances join the specified rooms.
func (e *Emitter) SocketsJoin(rooms ...socket.Room) error {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).SocketsJoin(rooms...)
}

// SocketsLeave makes the matching socket instances leave the specified rooms.
func (e *Emitter) SocketsLeave(rooms ...socket.Room) error {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).SocketsLeave(rooms...)
}

// DisconnectSockets makes the matching socket instances disconnect.
func (e *Emitter) DisconnectSockets(state bool) error {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).DisconnectSockets(state)
}

// ServerSideEmit sends a packet to the Socket.IO servers in the cluster.
// Acknowledgements are not supported.
func (e *Emitter) ServerSideEmit(args ...any) error {
	if len(args) > 0 {
		if _, withAck := args[len(args)-1].(socket.Ack); withAck {
			return errors.New("acknowledgements are not supported")
		}
	}
	request, err := json.Marshal(&Request{
		Uid:  UID,
		Type: redis.SERVER_SIDE_EMIT,
		Data: args,
	})
	if err != nil {
		return err
	}
	return e.redisClient.Client.Publish(e.redisClient.Context, e.broadcastOptions.RequestChannel, request).Err()
}
