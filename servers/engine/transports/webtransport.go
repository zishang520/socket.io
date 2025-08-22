// Package transports implements the WebTransport transport for Engine.IO.
package transports

import (
	"errors"
	"io"
	"net"
	"sync"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/webtransport"
)

var (
	wt_log = log.NewLog("engine:webtransport")
)

type webTransport struct {
	Transport

	session *types.WebTransportConn
	mu      sync.Mutex
}

// WebTransport transport
func MakeWebTransport() WebTransport {
	w := &webTransport{Transport: MakeTransport()}

	w.Prototype(w)

	return w
}

func NewWebTransport(ctx *types.HttpContext) WebTransport {
	w := MakeWebTransport()

	w.Construct(ctx)

	return w
}

func (w *webTransport) Construct(ctx *types.HttpContext) {
	w.Transport.Construct(ctx)

	w.session = ctx.WebTransport

	w.session.On("error", func(errs ...any) {
		w.OnError("webtransport error", errs[0].(error))
	})
	w.session.Once("close", func(...any) {
		w.OnClose()
	})

	go w.message()

	w.SetWritable(true)
	w.SetPerMessageDeflate(nil)
}

// Transport name
func (w *webTransport) Name() string {
	return WEBTRANSPORT
}

// Advertise upgrade support.
func (w *webTransport) HandlesUpgrades() bool {
	return true
}

// Receiving Messages
func (w *webTransport) message() {
	for {
		mt, message, err := w.session.NextReader()
		if err != nil {
			if webtransport.IsUnexpectedCloseError(err) || errors.Is(err, net.ErrClosed) {
				w.session.Emit("close")
			} else {
				w.session.Emit("error", err)
			}
			return
		}

		switch mt {
		case webtransport.BinaryMessage:
			read := types.NewBytesBuffer(nil)
			if _, err := read.ReadFrom(message); err != nil {
				if errors.Is(err, net.ErrClosed) {
					w.session.Emit("close")
				} else {
					w.session.Emit("error", err)
				}
			} else {
				w.onMessage(read)
			}
		case webtransport.TextMessage:
			read := types.NewStringBuffer(nil)
			if _, err := read.ReadFrom(message); err != nil {
				if errors.Is(err, net.ErrClosed) {
					w.session.Emit("close")
				} else {
					w.session.Emit("error", err)
				}
			} else {
				w.onMessage(read)
			}
		}
		if c, ok := message.(io.Closer); ok {
			c.Close()
		}
	}
}

func (w *webTransport) onMessage(data types.BufferInterface) {
	wt_log.Debug(`webTransport received "%s"`, data)
	w.Transport.OnData(data)
}

// Writes a packet payload.
func (w *webTransport) Send(packets []*packet.Packet) {
	w.SetWritable(false)
	go w.send(packets)
}
func (w *webTransport) send(packets []*packet.Packet) {
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
				mt := webtransport.BinaryMessage
				if _, ok := packet.Options.WsPreEncodedFrame.(*types.StringBuffer); ok {
					mt = webtransport.TextMessage
				}
				pm, err := webtransport.NewPreparedMessage(mt, packet.Options.WsPreEncodedFrame.Bytes())
				if err != nil {
					wt_log.Debug(`Send Error "%s"`, err.Error())
					if errors.Is(err, net.ErrClosed) {
						w.session.Emit("close")
					} else {
						w.session.Emit("error", err)
					}
					return
				}
				if err := w.session.WritePreparedMessage(pm); err != nil {
					wt_log.Debug(`Send Error "%s"`, err.Error())
					if errors.Is(err, net.ErrClosed) {
						w.session.Emit("close")
					} else {
						w.session.Emit("error", err)
					}
					return
				}
				return

			}
		}

		data, err := w.Parser().EncodePacket(packet, w.SupportsBinary())
		if err != nil {
			wt_log.Debug(`Send Error "%s"`, err.Error())
			if errors.Is(err, net.ErrClosed) {
				w.session.Emit("close")
			} else {
				w.session.Emit("error", err)
			}
			return
		}
		w.write(data, compress)
	}
}

func (w *webTransport) write(data types.BufferInterface, _ bool) {
	// if w.PerMessageDeflate() != nil {
	// 	if data.Len() < w.PerMessageDeflate().Threshold {
	// 		compress = false
	// 	}
	// }
	wt_log.Debug(`writing %#s`, data)

	// w.session.EnableWriteCompression(compress)
	mt := webtransport.BinaryMessage
	if _, ok := data.(*types.StringBuffer); ok {
		mt = webtransport.TextMessage
	}
	write, err := w.session.NextWriter(mt)
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			w.session.Emit("close")
		} else {
			w.session.Emit("error", err)
		}
		return
	}
	defer func() {
		if err := write.Close(); err != nil {
			if errors.Is(err, net.ErrClosed) {
				w.session.Emit("close")
			} else {
				w.session.Emit("error", err)
			}
			return
		}
	}()
	if _, err := io.Copy(write, data); err != nil {
		if errors.Is(err, net.ErrClosed) {
			w.session.Emit("close")
		} else {
			w.session.Emit("error", err)
		}
		return
	}
}

// Closes the transport.
func (w *webTransport) DoClose(fn types.Callable) {
	wt_log.Debug(`closing WebTransport session`)
	defer w.session.CloseWithError(0, "")
	if fn != nil {
		fn()
	}
}
