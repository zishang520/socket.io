// Package transports implements the WebSocket transport for Engine.IO.
package transports

import (
	"errors"
	"io"
	"net"
	"sync"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/queue"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

var wsLog = log.NewLog("engine:ws")

var newWebSocketPreparedMessage = ws.NewPreparedMessage

type webSocketPreparedFrame interface {
	PreparedWebSocketFrame(func(types.BufferInterface) (any, error)) (any, error)
}

type websocket struct {
	Transport

	idleTimeout time.Duration

	socket     *types.WebSocketConn
	mu         sync.Mutex
	writeQueue *queue.Queue
}

// WebSocket transport
func MakeWebSocket() Websocket {
	w := &websocket{Transport: MakeTransport()}

	w.Prototype(w)

	return w
}

func NewWebSocket(ctx *types.HttpContext) Websocket {
	w := MakeWebSocket()

	w.Construct(ctx)

	return w
}

func (w *websocket) Construct(ctx *types.HttpContext) {
	w.Transport.Construct(ctx)

	w.idleTimeout = ctx.IdleTimeout
	w.socket = ctx.Websocket
	w.writeQueue = queue.New()

	_ = w.socket.On("error", func(errs ...any) {
		w.OnError("websocket error", slices.TryGetAny[error](errs, 0))
	})
	_ = w.socket.Once("close", func(...any) {
		w.OnClose()
	})

	// This goroutine is invoked only once.
	go w.message()

	w.SetWritable(true)
	w.SetPerMessageDeflate(nil)
}

// Transport name
func (w *websocket) Name() string {
	return WEBSOCKET
}

// Advertise upgrade support.
func (w *websocket) HandlesUpgrades() bool {
	return true
}

func (w *websocket) _error(err error) {
	if ws.IsUnexpectedCloseError(err) || errors.Is(err, net.ErrClosed) {
		w.socket.Emit("close")
	} else {
		w.socket.Emit("error", err)
	}
}

// Receiving Messages
func (w *websocket) message() {
	// Guarantee cleanup on any exit path — covers errors that _error()
	// routes as "error" (not "close") on the socket and any future paths
	// that return without calling _error at all.
	defer func() {
		if !w.writeQueue.IsShuttingDown() {
			w.socket.Emit("close")
		}
	}()

	for {
		if w.idleTimeout > 0 {
			_ = w.socket.SetReadDeadline(time.Now().Add(w.idleTimeout))
		}
		mt, message, err := w.socket.NextReader()
		if err != nil {
			w._error(err)
			return
		}

		switch mt {
		case ws.BinaryMessage:
			read := types.NewBytesBuffer(nil)
			if _, err := read.ReadFrom(message); err != nil {
				w._error(err)
			} else {
				w.onMessage(read)
			}
		case ws.TextMessage:
			read := types.NewStringBuffer(nil)
			if _, err := read.ReadFrom(message); err != nil {
				w._error(err)
			} else {
				w.onMessage(read)
			}
		case ws.CloseMessage:
			w.socket.Emit("close")
			if c, ok := message.(io.Closer); ok {
				_ = c.Close()
			}
			return
		case ws.PingMessage:
		case ws.PongMessage:
		}
		if c, ok := message.(io.Closer); ok {
			_ = c.Close()
		}
	}
}

func (w *websocket) onMessage(data types.BufferInterface) {
	if log.DEBUG.Load() {
		wsLog.Debug(`websocket received "%s"`, data)
	}
	w.OnData(data)
}

// Writes a packet payload.
func (w *websocket) Send(packets []*packet.Packet) {
	w.SetWritable(false)
	w.writeQueue.Enqueue(func() { w.send(packets) })
}
func (w *websocket) send(packets []*packet.Packet) {
	defer func() {
		w.Emit("drain")
		w.SetWritable(true)
		w.Emit("ready")
	}()

	w.mu.Lock()
	defer w.mu.Unlock()

	for _, packet := range packets {
		// always creates a new object since ws modifies it
		compress := true
		if packet.Options != nil {
			if packet.Options.Compress != nil && !*packet.Options.Compress {
				compress = false
			}

			if w.PerMessageDeflate() == nil && packet.Options.WsPreEncodedFrame != nil {
				pm, err := websocketPreparedMessage(packet)
				if err != nil {
					wsLog.Debug(`Send Error "%s"`, err.Error())
					w._error(err)
					return
				}
				if err := w.socket.WritePreparedMessage(pm); err != nil {
					wsLog.Debug(`Send Error "%s"`, err.Error())
					w._error(err)
					return
				}
				continue

			}
		}

		data, err := w.Parser().EncodePacket(packet, w.SupportsBinary())
		if err != nil {
			wsLog.Debug(`Send Error "%s"`, err.Error())
			w._error(err)
			return
		}
		w.write(data, compress)
	}
}

func websocketPreparedMessage(packet *packet.Packet) (*ws.PreparedMessage, error) {
	frame := packet.Options.WsPreEncodedFrame
	messageType := ws.BinaryMessage
	if isStringBuffer(frame) || isStringBuffer(packet.Data) {
		messageType = ws.TextMessage
	}

	build := func(data types.BufferInterface) (any, error) {
		return newWebSocketPreparedMessage(messageType, data.Bytes())
	}

	if cached, ok := frame.(webSocketPreparedFrame); ok {
		prepared, err := cached.PreparedWebSocketFrame(build)
		if err != nil {
			return nil, err
		}
		pm, ok := prepared.(*ws.PreparedMessage)
		if !ok {
			return nil, errors.New("websocket prepared frame cache returned unexpected type")
		}
		return pm, nil
	}

	pm, err := build(frame)
	if err != nil {
		return nil, err
	}
	return pm.(*ws.PreparedMessage), nil
}

func isStringBuffer(data any) bool {
	_, ok := data.(*types.StringBuffer)
	return ok
}

func (w *websocket) write(data types.BufferInterface, compress bool) {
	if w.PerMessageDeflate() != nil {
		if data.Len() < w.PerMessageDeflate().Threshold {
			compress = false
		}
	}
	if log.DEBUG.Load() {
		wsLog.Debug(`writing %#v`, data)
	}

	w.socket.EnableWriteCompression(compress)
	mt := ws.BinaryMessage
	if _, ok := data.(*types.StringBuffer); ok {
		mt = ws.TextMessage
	}
	write, err := w.socket.NextWriter(mt)
	if err != nil {
		w._error(err)
		return
	}
	defer func() {
		if err := write.Close(); err != nil {
			w._error(err)
			return
		}
	}()
	if _, err := io.Copy(write, data); err != nil {
		w._error(err)
		return
	}
}

// OnClose tears down the write queue regardless of how the transport
// reaches the "closed" state. The base Transport.Close() path runs
// DoClose (which closes the queue), but any close that originates as
// an error / unexpected peer drop goes straight through OnClose with
// the state already flipped to "closed" — at which point a follow-up
// Transport.Close() returns early and DoClose never fires. Without
// this override the per-socket queue.loop goroutine waits on its
// sync.Cond forever, leaking once per ungraceful disconnect.
//
// The hijacked websocket connection is closed here as well, so an
// ungraceful disconnect (where DoClose never runs) still releases the
// underlying connection instead of leaking it. socket.Close is
// idempotent, so closing again on the DoClose path is harmless.
func (w *websocket) OnClose() {
	w.writeQueue.TryClose()
	w.Transport.OnClose()
	_ = w.socket.Close()
}

// Closes the transport.
func (w *websocket) DoClose(fn types.Callable) {
	wsLog.Debug(`closing`)
	w.writeQueue.TryClose()
	defer func() { _ = w.socket.Close() }()
	if fn != nil {
		fn()
	}
}
