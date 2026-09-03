// Package emitter broadcasts Socket.IO events without a local Socket.IO server.
package emitter

import (
	"strings"

	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

const defaultNamespace = "/"

var emitterLog = log.NewLog("socket.io-valkey-emitter")

// Emitter broadcasts messages with classic or sharded Valkey Pub/Sub.
type Emitter struct {
	valkeyClient     *valkey.ValkeyClient
	opts             *EmitterOptions
	broadcastOptions *BroadcastOptions
}

// MakeEmitter creates an uninitialized emitter with default options.
func MakeEmitter() *Emitter {
	return &Emitter{opts: DefaultEmitterOptions()}
}

// NewEmitter creates and initializes a Valkey Pub/Sub emitter.
func NewEmitter(client *valkey.ValkeyClient, opts *EmitterOptions, nsps ...string) *Emitter {
	e := MakeEmitter()
	e.Construct(client, opts, nsps...)
	return e
}

// Construct initializes the emitter and its Valkey channels.
func (e *Emitter) Construct(client *valkey.ValkeyClient, opts *EmitterOptions, nsps ...string) {
	e.valkeyClient = client
	e.opts.Assign(opts)
	if e.opts.GetRawKey() == nil {
		e.opts.SetKey(DefaultEmitterKey)
	}
	if utils.IsNil(e.opts.Encoder()) {
		e.opts.SetEncoder(utils.MsgPack())
	}
	nsp := defaultNamespace
	if len(nsps) > 0 {
		nsp = nsps[0]
	}

	key := e.opts.Key()
	e.broadcastOptions = &BroadcastOptions{
		Nsp:              nsp,
		BroadcastChannel: key + "#" + nsp + "#",
		RequestChannel:   key + "-request#" + nsp + "#",
		Encoder:          e.opts.Encoder(),
		SubscriptionMode: e.opts.SubscriptionMode(),
	}
}

// Of returns an emitter for a namespace, normalizing its leading slash like
// socket.io-redis-emitter.
func (e *Emitter) Of(nsp string) *Emitter {
	if !strings.HasPrefix(nsp, "/") {
		nsp = "/" + nsp
	}
	return NewEmitter(e.valkeyClient, e.opts, nsp)
}

func (e *Emitter) Emit(ev string, args ...any) error {
	return e.newBroadcastOperator().Emit(ev, args...)
}

func (e *Emitter) To(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().To(rooms...)
}

func (e *Emitter) In(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.To(rooms...)
}

func (e *Emitter) Except(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Except(rooms...)
}

func (e *Emitter) Volatile() BroadcastOperatorInterface {
	return e.newBroadcastOperator().Volatile()
}

func (e *Emitter) Compress(compress bool) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Compress(compress)
}

func (e *Emitter) SocketsJoin(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsJoin(rooms...)
}

func (e *Emitter) SocketsLeave(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsLeave(rooms...)
}

func (e *Emitter) DisconnectSockets(close bool) error {
	return e.newBroadcastOperator().DisconnectSockets(close)
}

func (e *Emitter) ServerSideEmit(args ...any) error {
	return e.newBroadcastOperator().ServerSideEmit(args...)
}

func (e *Emitter) newBroadcastOperator() BroadcastOperatorInterface {
	if e.opts.Sharded() {
		return NewShardedBroadcastOperator(e.valkeyClient, e.broadcastOptions, nil, nil, nil)
	}
	return NewBroadcastOperator(e.valkeyClient, e.broadcastOptions, nil, nil, nil)
}
