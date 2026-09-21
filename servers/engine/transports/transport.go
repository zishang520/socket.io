// Package transports provides implementations for Engine.IO transport mechanisms such as polling, WebSocket, and WebTransport.
package transports

import (
	"sync"
	"sync/atomic"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/errors"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

var transportLog = log.NewLog("engine:transport")

type transport struct {
	types.EventEmitter

	// Prototype interface, used to implement interface method rewriting
	_proto_ Transport

	maxHttpBufferSize atomic.Int64
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

	// gateMu guards gateOpen and gateBuf together. OnPacket's gate check +
	// buffer append and ReleaseGate's flush + open transition MUST execute
	// under this single lock as one critical section: if the two operations
	// were allowed to interleave (e.g. gateOpen flipped and observed by a
	// concurrent OnPacket call before the buffered packets are flushed), a
	// packet appended in that narrow window would never be flushed again,
	// reproducing the exact silent-drop defect this gate exists to close.
	gateMu sync.Mutex
	// gateOpen is only ever written while gateMu is held. It is an
	// atomic.Bool (rather than a plain bool) so ReleaseGate can use
	// CompareAndSwap to make repeated calls idempotent without a second
	// flag; correctness of the gate/buffer interaction still comes from
	// gateMu, not from the atomicity of this field on its own.
	gateOpen atomic.Bool
	// gateBuf holds packets received while the gate is closed, in arrival
	// (FIFO) order, so ReleaseGate can replay them in the order they were
	// actually received.
	gateBuf []*packet.Packet
}

func MakeTransport() Transport {
	t := &transport{
		EventEmitter: types.NewEventEmitter(),
	}
	t._readyState.Store("open")
	// Default to open (today's behavior) so that any caller which somehow
	// invokes OnPacket before Construct runs does not silently swallow
	// packets. Construct is the only place that ever closes the gate, and it
	// always closes it before the reader goroutine that would call OnPacket
	// is started (see websocket.go / webtransport.go Construct).
	t.gateOpen.Store(true)

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
	transportLog.Debug(`readyState updated from %s to %s (%s)`, t.ReadyState(), state, t._proto_.Name())

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
	return t.maxHttpBufferSize.Load()
}

func (t *transport) SetMaxHttpBufferSize(maxHttpBufferSize int64) {
	t.maxHttpBufferSize.Store(maxHttpBufferSize)
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

	// Close the gate for connections that originated from the Engine.IO
	// server's own ServeHTTP entry point: the caller (Handshake / onWebSocket
	// / OnWebTransportSession) still has "packet" listener registration left
	// to do after this constructor returns, and the reader goroutine started
	// right after Construct (see websocket.go / webtransport.go) must not be
	// allowed to deliver packets before that registration completes. The
	// caller reopens the gate with ReleaseGate once registration is done.
	// Connections built without the marker (e.g. websocket_test.go's direct
	// NewWebSocket(ctx) call) keep the pre-existing always-open behavior.
	t.gateOpen.Store(!isHandshakeOrigin(ctx.Context()))
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
		transportLog.Debug("ignored transport error %s (%v)", msg, desc)
	}
}

// Called with parsed out a packets from the data stream.
//
// While the gate is closed the packet is appended to gateBuf instead of
// being emitted; ReleaseGate flushes gateBuf (in arrival order) and reopens
// the gate. See the gateMu field comment for why the check-and-append here
// and the flush-and-open in ReleaseGate must share one critical section.
func (t *transport) OnPacket(p *packet.Packet) {
	t.gateMu.Lock()
	if !t.gateOpen.Load() {
		t.gateBuf = append(t.gateBuf, p)
		t.gateMu.Unlock()
		return
	}
	t.gateMu.Unlock()

	// Any OnPacket call that observes gateOpen == true is guaranteed (by
	// gateMu's mutual exclusion and the lock/unlock happens-before edges) to
	// run after ReleaseGate's flush of every packet buffered up to that
	// point has fully completed, so emitting outside the lock here cannot
	// reorder this packet ahead of previously buffered ones.
	t.Emit("packet", p)
}

// ReleaseGate reopens the packet-delivery gate, flushing any packets
// buffered by OnPacket while the gate was closed, in the order they were
// received. It is idempotent: calling it more than once (from any of
// Handshake / onWebSocket / OnWebTransportSession) after the first call is a
// no-op, guarded by CompareAndSwap. The flush happens while gateMu is still
// held so that no OnPacket call can observe the gate as open until every
// buffered packet has been emitted — see the gateMu field comment.
func (t *transport) ReleaseGate() {
	t.gateMu.Lock()
	defer t.gateMu.Unlock()

	if !t.gateOpen.CompareAndSwap(false, true) {
		// Already open (never closed, or a previous ReleaseGate call already
		// flushed and opened it). Nothing to do.
		return
	}

	buffered := t.gateBuf
	t.gateBuf = nil
	for _, p := range buffered {
		t.Emit("packet", p)
	}
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
