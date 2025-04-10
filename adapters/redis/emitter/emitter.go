package emitter

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	"github.com/zishang520/socket.io/adapters/redis/v3/types"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

const UID adapter.ServerId = "emitter"

var emitter_log = log.NewLog("socket.io-emitter")

type Emitter struct {
	redisClient *types.RedisClient

	opts             *EmitterOptions
	broadcastOptions *BroadcastOptions
	nsp              string
}

func MakeEmitter() *Emitter {
	e := &Emitter{
		opts: DefaultEmitterOptions(),
		nsp:  "/",
	}

	return e
}

func NewEmitter(redisClient *types.RedisClient, opts *EmitterOptions, nsps ...string) *Emitter {
	e := MakeEmitter()

	e.Construct(redisClient, opts, nsps...)

	return e
}

func (e *Emitter) Construct(redisClient *types.RedisClient, opts *EmitterOptions, nsps ...string) {
	e.redisClient = redisClient

	if opts == nil {
		opts = DefaultEmitterOptions()
	}
	e.opts.Assign(opts)

	if e.opts.GetRawKey() == nil {
		e.opts.SetKey("socket.io")
	}

	if e.opts.GetRawParser() == nil {
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

// Return a new emitter for the given namespace.
func (e *Emitter) Of(nsp string) *Emitter {
	if !strings.HasPrefix(nsp, "/") {
		nsp = "/" + nsp
	}
	return NewEmitter(e.redisClient, e.opts, nsp)
}

// Emits to all clients.
func (e *Emitter) Emit(ev string, args ...any) error {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).Emit(ev, args...)
}

// Targets a room when emitting.
func (e *Emitter) To(rooms ...socket.Room) *BroadcastOperator {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).To(rooms...)
}

// Targets a room when emitting.
func (e *Emitter) In(rooms ...socket.Room) *BroadcastOperator {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).In(rooms...)
}

// Excludes a room when emitting.
func (e *Emitter) Except(rooms ...socket.Room) *BroadcastOperator {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).Except(rooms...)
}

// Sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to
// receive messages (because of network slowness or other issues, or because they’re connected through long polling
// and is in the middle of a request-response cycle).
func (e *Emitter) Volatile() *BroadcastOperator {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).Volatile()
}

// Sets the compress flag.
//
// compress - if `true`, compresses the sending data
func (e *Emitter) Compress(compress bool) *BroadcastOperator {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).Compress(compress)
}

// Makes the matching socket instances join the specified rooms
func (e *Emitter) SocketsJoin(rooms ...socket.Room) error {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).SocketsJoin(rooms...)
}

// Makes the matching socket instances leave the specified rooms
func (e *Emitter) SocketsLeave(rooms ...socket.Room) error {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).SocketsLeave(rooms...)
}

// Makes the matching socket instances disconnect
func (e *Emitter) DisconnectSockets(state bool) error {
	return NewBroadcastOperator(e.redisClient, e.broadcastOptions, nil, nil, nil).DisconnectSockets(state)
}

// Send a packet to the Socket.IO servers in the cluster
func (e *Emitter) ServerSideEmit(args ...any) error {
	if len(args) > 0 {
		if _, withAck := args[len(args)-1].(socket.Ack); withAck {
			return errors.New("Acknowledgements are not supported")
		}
	}
	request, err := json.Marshal(&Request{
		Uid:  UID,
		Type: types.SERVER_SIDE_EMIT,
		Data: args,
	})
	if err != nil {
		return err
	}
	return e.redisClient.Client.Publish(e.redisClient.Context, e.broadcastOptions.RequestChannel, request).Err()
}
