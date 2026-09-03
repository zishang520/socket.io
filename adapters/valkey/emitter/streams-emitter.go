package emitter

import (
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
)

var valkeyStreamsEmitterLog = log.NewLog("socket.io-valkey-streams-emitter")

// ValkeyStreamsEmitter broadcasts messages through a Valkey stream.
type ValkeyStreamsEmitter struct {
	valkeyClient *valkey.ValkeyClient
	opts         *ValkeyStreamsEmitterOptions
	nsp          string
}

func MakeValkeyStreamsEmitter() *ValkeyStreamsEmitter {
	return &ValkeyStreamsEmitter{
		opts: DefaultValkeyStreamsEmitterOptions(),
		nsp:  defaultNamespace,
	}
}

func NewValkeyStreamsEmitter(client *valkey.ValkeyClient, opts *ValkeyStreamsEmitterOptions, nsps ...string) *ValkeyStreamsEmitter {
	e := MakeValkeyStreamsEmitter()
	e.Construct(client, opts, nsps...)
	return e
}

func (e *ValkeyStreamsEmitter) Construct(client *valkey.ValkeyClient, opts *ValkeyStreamsEmitterOptions, nsps ...string) {
	e.valkeyClient = client
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

func (e *ValkeyStreamsEmitter) Of(nsp string) *ValkeyStreamsEmitter {
	return NewValkeyStreamsEmitter(e.valkeyClient, e.opts, nsp)
}

func (e *ValkeyStreamsEmitter) Emit(ev string, args ...any) error {
	return e.newBroadcastOperator().Emit(ev, args...)
}

func (e *ValkeyStreamsEmitter) To(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().To(rooms...)
}

func (e *ValkeyStreamsEmitter) In(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.To(rooms...)
}

func (e *ValkeyStreamsEmitter) Except(rooms ...socket.Room) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Except(rooms...)
}

func (e *ValkeyStreamsEmitter) Volatile() BroadcastOperatorInterface {
	return e.newBroadcastOperator().Volatile()
}

func (e *ValkeyStreamsEmitter) Compress(compress bool) BroadcastOperatorInterface {
	return e.newBroadcastOperator().Compress(compress)
}

func (e *ValkeyStreamsEmitter) SocketsJoin(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsJoin(rooms...)
}

func (e *ValkeyStreamsEmitter) SocketsLeave(rooms ...socket.Room) error {
	return e.newBroadcastOperator().SocketsLeave(rooms...)
}

func (e *ValkeyStreamsEmitter) DisconnectSockets(close bool) error {
	return e.newBroadcastOperator().DisconnectSockets(close)
}

func (e *ValkeyStreamsEmitter) ServerSideEmit(args ...any) error {
	return e.newBroadcastOperator().ServerSideEmit(args...)
}

func (e *ValkeyStreamsEmitter) newBroadcastOperator() *ValkeyStreamsBroadcastOperator {
	return NewValkeyStreamsBroadcastOperator(e.valkeyClient, e.nsp, e.opts, nil, nil, nil)
}
