// Package transports provides implementations for Engine.IO transport mechanisms such as polling, WebSocket, and WebTransport.
package transports

import (
	"sync/atomic"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/errors"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

var transport_log = log.NewLog("engine:transport")

type transport struct {
	types.EventEmitter

	// Prototype interface, used to implement interface method rewriting
	_proto_ Transport

	maxHttpBufferSize int64
	httpCompression   *types.HttpCompression
	perMessageDeflate *types.PerMessageDeflate

	// The session ID.
	sid      string
	protocol int // 3

	_readyState types.Atomic[string] //"open";

	_discarded atomic.Bool // false;

	parser parser.Parser // parser.PaserV3;

	supportsBinary bool

	// Whether the transport is currently ready to send packets.
	_writable atomic.Bool
}

func MakeTransport() Transport {
	t := &transport{
		EventEmitter: types.NewEventEmitter(),
	}
	t._readyState.Store("open")

	t.Prototype(t)

	return t
}

func NewTransport(ctx *types.HttpContext) Transport {
	t := MakeTransport()

	t.Construct(ctx)

	return t
}

func (t *transport) Prototype(_t Transport) {
	t._proto_ = _t
}

func (t *transport) Proto() Transport {
	return t._proto_
}

func (t *transport) Sid() string {
	return t.sid
}

func (t *transport) SetSid(sid string) {
	t.sid = sid
}

func (t *transport) Writable() bool {
	return t._writable.Load()
}

func (t *transport) SetWritable(writable bool) {
	t._writable.Store(writable)
}

func (t *transport) Protocol() int {
	return t.protocol
}

func (t *transport) Discarded() bool {
	return t._discarded.Load()
}

func (t *transport) Parser() parser.Parser {
	return t.parser
}

func (t *transport) SupportsBinary() bool {
	return t.supportsBinary
}

func (t *transport) SetSupportsBinary(supportsBinary bool) {
	t.supportsBinary = supportsBinary
}

func (t *transport) ReadyState() string {
	return t._readyState.Load()
}

func (t *transport) SetReadyState(state string) {
	transport_log.Debug(`readyState updated from %s to %s (%s)`, t.ReadyState(), state, t._proto_.Name())

	t._readyState.Store(state)
}

func (t *transport) HttpCompression() *types.HttpCompression {
	return t.httpCompression
}

func (t *transport) SetHttpCompression(httpCompression *types.HttpCompression) {
	t.httpCompression = httpCompression

}
func (t *transport) PerMessageDeflate() *types.PerMessageDeflate {
	return t.perMessageDeflate
}

func (t *transport) SetPerMessageDeflate(perMessageDeflate *types.PerMessageDeflate) {
	t.perMessageDeflate = perMessageDeflate
}

func (t *transport) MaxHttpBufferSize() int64 {
	return t.maxHttpBufferSize
}

func (t *transport) SetMaxHttpBufferSize(maxHttpBufferSize int64) {
	t.maxHttpBufferSize = maxHttpBufferSize
}

// Transport Construct.
func (t *transport) Construct(ctx *types.HttpContext) {
	if eio, ok := ctx.Query().Get("EIO"); ok && eio == "4" {
		t.parser = parser.Parserv4()
	} else {
		t.parser = parser.Parserv3()
	}

	t.protocol = t.parser.Protocol()
	t.supportsBinary = !ctx.Query().Has("b64")
}

// Flags the transport as discarded.
func (t *transport) Discard() {
	t._discarded.Store(true)
}

// Called with an incoming HTTP request.
func (t *transport) OnRequest(req *types.HttpContext) {}

// Closes the transport.
func (t *transport) Close(fn ...types.Callable) {
	if t.ReadyState() == "closed" || t.ReadyState() == "closing" {
		return
	}
	t.SetReadyState("closing")
	fn = append(fn, nil)
	t._proto_.DoClose(fn[0])
}

// Called with a transport error.
func (t *transport) OnError(msg string, desc error) {
	if t.ListenerCount("error") > 0 {
		t.Emit("error", errors.NewTransportError(msg, desc))
	} else {
		transport_log.Debug("ignored transport error %s (%v)", msg, desc)
	}
}

// Called with parsed out a packets from the data stream.
func (t *transport) OnPacket(packet *packet.Packet) {
	t.Emit("packet", packet)
}

// Called with the encoded packet data.
func (t *transport) OnData(data types.BufferInterface) {
	p, _ := t.parser.DecodePacket(data)
	t.OnPacket(p)
}

// Called upon transport close.
func (t *transport) OnClose() {
	if t.ReadyState() == "closed" {
		return
	}
	t.SetReadyState("closed")
	t.Emit("close")
}

func (t *transport) HandlesUpgrades() bool {
	return false
}

// The name of the transport.
func (t *transport) Name() string {
	return ""
}

// Sends an array of packets.
func (t *transport) Send([]*packet.Packet) {}

func (t *transport) DoClose(types.Callable) {}
