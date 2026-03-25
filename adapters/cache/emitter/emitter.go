// Package emitter provides an API for broadcasting messages to Socket.IO servers
// via the cache pub/sub layer without requiring a full Socket.IO server instance.
//
// This is useful for sending events from services that are not themselves Socket.IO
// servers but need to communicate with connected clients through the cluster.
package emitter

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

const emitterUID adapter.ServerId = "emitter"

const defaultNamespace = "/"

var emitterLog = log.NewLog("socket.io-cache-emitter")

// Emitter broadcasts messages to Socket.IO servers using cache pub/sub.
type Emitter struct {
	cacheClient      cache.CacheClient
	opts             *EmitterOptions
	broadcastOptions *BroadcastOptions
	nsp              string
}

// MakeEmitter returns an Emitter with default options and root namespace.
func MakeEmitter() *Emitter {
	return &Emitter{
		opts: DefaultEmitterOptions(),
		nsp:  defaultNamespace,
	}
}

// NewEmitter creates and initializes an Emitter with the given client and options.
// An optional namespace may be supplied; defaults to "/".
func NewEmitter(client cache.CacheClient, opts *EmitterOptions, nsps ...string) *Emitter {
	e := MakeEmitter()
	e.Construct(client, opts, nsps...)
	return e
}

// Construct initializes the Emitter.
func (e *Emitter) Construct(client cache.CacheClient, opts *EmitterOptions, nsps ...string) {
	e.cacheClient = client

	if opts == nil {
		opts = DefaultEmitterOptions()
	}
	e.opts.Assign(opts)

	if e.opts.GetRawKey() == nil {
		e.opts.SetKey(DefaultEmitterKey)
	}
	if e.opts.Parser() == nil {
		e.opts.SetParser(utils.MsgPack())
	}

	if len(nsps) > 0 && len(nsps[0]) > 0 {
		e.nsp = nsps[0]
	}

	key := e.opts.Key()
	e.broadcastOptions = &BroadcastOptions{
		Nsp:              e.nsp,
		BroadcastChannel: key + "#" + e.nsp + "#",
		RequestChannel:   key + "-request#" + e.nsp + "#",
		Parser:           e.opts.Parser(),
		Sharded:          e.opts.Sharded(),
		SubscriptionMode: e.opts.SubscriptionMode(),
	}
}

// Of returns a new Emitter targeting the given namespace.
func (e *Emitter) Of(nsp string) *Emitter {
	if !strings.HasPrefix(nsp, "/") {
		nsp = "/" + nsp
	}
	return NewEmitter(e.cacheClient, e.opts, nsp)
}

// Emit broadcasts an event to all clients in the namespace.
func (e *Emitter) Emit(ev string, args ...any) error {
	return e.newBroadcastOperator().Emit(ev, args...)
}

// To targets specific room(s) for event emission.
func (e *Emitter) To(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().To(rooms...)
}

// In is an alias for To.
func (e *Emitter) In(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().In(rooms...)
}

// Except excludes specific room(s) from event emission.
func (e *Emitter) Except(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Except(rooms...)
}

// Volatile sets the volatile flag; event data may be dropped for unready clients.
func (e *Emitter) Volatile() BroadcastOperatorInterface {
	return e.newBroadcastOperator().Volatile()
}

// Compress sets the compress flag for the broadcast.
func (e *Emitter) Compress(compress bool) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Compress(compress)
}

// SocketsJoin makes all matching sockets join the specified rooms.
func (e *Emitter) SocketsJoin(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsJoin(rooms...)
}

// SocketsLeave makes all matching sockets leave the specified rooms.
func (e *Emitter) SocketsLeave(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsLeave(rooms...)
}

// DisconnectSockets disconnects all matching sockets.
func (e *Emitter) DisconnectSockets(state bool) error {
	return e.newBroadcastOperator().DisconnectSockets(state)
}

// ServerSideEmit sends a message to all Socket.IO servers in the cluster.
// Acknowledgements are not supported when using the emitter.
func (e *Emitter) ServerSideEmit(args ...any) error {
	if len(args) > 0 {
		if _, withAck := args[len(args)-1].(socket.Ack); withAck {
			return errors.New("acknowledgements are not supported when using emitter")
		}
	}

	if e.opts.Sharded() {
		message := &ClusterMessage{
			Uid:  emitterUID,
			Nsp:  e.nsp,
			Type: adapter.SERVER_SIDE_EMIT,
			Data: &ServerSideEmitMessage{
				Packet: args,
			},
		}
		msg, err := e.broadcastOptions.Parser.Encode(message)
		if err != nil {
			return err
		}
		return e.cacheClient.SPublish(e.cacheClient.Context(), e.broadcastOptions.BroadcastChannel, msg)
	}

	request, err := json.Marshal(&Request{
		Uid:  emitterUID,
		Type: cache.SERVER_SIDE_EMIT,
		Data: args,
	})
	if err != nil {
		return err
	}

	return e.cacheClient.Publish(e.cacheClient.Context(), e.broadcastOptions.RequestChannel, request)
}

func (e *Emitter) newBroadcastOperator() BroadcastOperatorInterface {
	if e.opts.Sharded() {
		return NewShardedBroadcastOperator(e.cacheClient, e.broadcastOptions, nil, nil, nil)
	}
	return NewBroadcastOperator(e.cacheClient, e.broadcastOptions, nil, nil, nil)
}
