package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/request"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	"github.com/zishang520/socket.io/v3/pkg/webtransport"
	wt "github.com/zishang520/webtransport-go"
)

// webTransport implements the WebTransport transport for Engine.IO.
// This transport provides low-latency, bidirectional communication using the QUIC protocol.
// It offers several advantages over WebSocket:
// - Lower latency
// - Better multiplexing support
// - Built-in congestion control
// - Support for unreliable datagrams
type webTransport struct {
	Transport

	// dialer is the WebTransport dialer used to establish connections
	dialer *wt.Dialer

	// session is the WebTransport connection instance
	session *types.WebTransportConn

	// mu is a mutex to protect concurrent access to the WebTransport connection
	mu sync.Mutex
}

// Name returns the identifier for the WebTransport transport.
func (w *webTransport) Name() string {
	return transports.WEBTRANSPORT
}

// MakeWebTransport creates a new WebTransport instance with default settings.
// This is the factory function for creating a new WebTransport.
func MakeWebTransport() WebTransport {
	s := &webTransport{
		Transport: MakeTransport(),
	}

	s.Prototype(s)

	return s
}

// NewWebTransport creates a new WebTransport instance with the specified socket and options.
//
// Parameters:
//   - socket: The parent socket instance
//   - opts: The socket options configuration
//
// Returns: A new WebTransport instance
func NewWebTransport(socket Socket, opts SocketOptionsInterface) WebTransport {
	s := MakeWebTransport()

	s.Construct(socket, opts)

	return s
}

// Construct initializes the WebTransport with the given socket and options.
// This sets up the WebTransport dialer with appropriate configuration for the connection.
func (w *webTransport) Construct(socket Socket, opts SocketOptionsInterface) {
	w.Transport.Construct(socket, opts)

	w.dialer = &wt.Dialer{
		TLSClientConfig: w.Opts().TLSClientConfig(),
		QUICConfig:      w.Opts().QUICConfig(),
	}
}

// DoOpen initiates the WebTransport connection.
// This method establishes the WebTransport connection, handles cookie management,
// and sets up event listeners.
func (w *webTransport) DoOpen() {
	headers := http.Header{}
	for k, vs := range w.Opts().ExtraHeaders() {
		for _, v := range vs {
			headers.Add(k, v)
		}
	}

	uri := w.uri()
	if w.Socket().CookieJar() != nil {
		for _, cookie := range w.Socket().CookieJar().Cookies(uri) {
			s := fmt.Sprintf("%s=%s", request.SanitizeCookieName(cookie.Name), request.SanitizeCookieValue(cookie.Value, cookie.Quoted))
			if c := headers.Get("Cookie"); c != "" {
				headers.Set("Cookie", c+"; "+s)
			} else {
				headers.Set("Cookie", s)
			}
		}
	}
	response, session, err := w.dialer.Dial(context.Background(), uri.String(), headers)
	if err != nil {
		w.Emit("error", err)
		return
	}
	if w.Socket().CookieJar() != nil {
		w.Socket().CookieJar().SetCookies(uri, response.Cookies())
	}

	stream, err := session.OpenStreamSync(context.Background())
	if err != nil {
		client_webtransport_log.Debug("session is closed")
		w.Emit("error", err)
		return
	}

	w.session = &types.WebTransportConn{EventEmitter: types.NewEventEmitter(), Conn: webtransport.NewConn(session, stream, true, 0, 0, nil, nil, nil)}

	w.addEventListeners()
}

// message handles the WebTransport message reading loop.
// This method processes incoming WebTransport messages and handles different message types.
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
				w.OnData(read)
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
				w.OnData(read)
			}
		}
		if c, ok := message.(io.Closer); ok {
			c.Close()
		}
	}
}

// handshake performs the initial handshake with the server.
// This method sends an OPEN packet with the session ID if available.
func (w *webTransport) handshake() {
	packet := &packet.Packet{
		Type: packet.OPEN,
	}
	if w.Query().Has("sid") {
		if data, err := json.Marshal(map[string]any{
			"sid": w.Query().Get("sid"),
		}); err != nil {
			client_webtransport_log.Debug("JSON Marshal error: %s", err.Error())
		} else {
			packet.Data = types.NewStringBuffer(data)
		}
	}

	data, err := parser.Parserv4().EncodePacket(packet, w.SupportsBinary())
	if err != nil {
		client_webtransport_log.Debug(`Send Error "%s"`, err.Error())
		if errors.Is(err, net.ErrClosed) {
			w.session.Emit("close")
		} else {
			w.session.Emit("error", err)
		}
		return
	}
	w.doWrite(data, true)

	w.OnOpen()
}

// addEventListeners sets up event handlers for the WebTransport connection.
// This method configures error and close event handlers and starts the message reading loop.
func (w *webTransport) addEventListeners() {
	w.session.On("error", func(errs ...any) {
		w.OnError("webtransport error", utils.TryCast[error](errs[0]), w.session.Session().Context())
	})
	w.session.Once("close", func(...any) {
		client_webtransport_log.Debug(`transport closed gracefully`)
		w.OnClose(NewTransportError("webtransport connection closed", nil, w.session.Session().Context()).Err())
	})

	go w.message()

	go w.handshake()
}

// Write sends packets over the WebTransport connection.
// This method handles packet encoding and WebTransport message framing.
func (w *webTransport) Write(packets []*packet.Packet) {
	w.SetWritable(false)

	go w.write(packets)
}
func (w *webTransport) write(packets []*packet.Packet) {
	// fake drain
	// defer to next tick to allow Socket to clear writeBuffer
	defer func() {
		w.SetWritable(true)
		w.Emit("drain")
	}()

	w.mu.Lock()
	defer w.mu.Unlock()

	// encodePacket efficient as it uses webTransport framing
	// no need for encodePayload
	for _, packet := range packets {
		// always creates a new object since ws modifies it
		compress := true
		if packet.Options != nil {
			if packet.Options.Compress != nil && !*packet.Options.Compress {
				compress = false
			}

			if w.Opts().PerMessageDeflate() == nil && packet.Options.WsPreEncodedFrame != nil {
				mt := webtransport.BinaryMessage
				if _, ok := packet.Options.WsPreEncodedFrame.(*types.StringBuffer); ok {
					mt = webtransport.TextMessage
				}
				pm, err := webtransport.NewPreparedMessage(mt, packet.Options.WsPreEncodedFrame.Bytes())
				if err != nil {
					client_webtransport_log.Debug(`Send Error "%s"`, err.Error())
					if errors.Is(err, net.ErrClosed) {
						w.session.Emit("close")
					} else {
						w.session.Emit("error", err)
					}
					return
				}
				if err := w.session.WritePreparedMessage(pm); err != nil {
					client_webtransport_log.Debug(`Send Error "%s"`, err.Error())
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

		data, err := parser.Parserv4().EncodePacket(packet, w.SupportsBinary())
		if err != nil {
			client_webtransport_log.Debug(`Send Error "%s"`, err.Error())
			if errors.Is(err, net.ErrClosed) {
				w.session.Emit("close")
			} else {
				w.session.Emit("error", err)
			}
			return
		}
		w.doWrite(data, compress)
	}
}

// doWrite performs the actual WebTransport write operation.
// This method handles message compression and WebTransport message framing.
func (w *webTransport) doWrite(data types.BufferInterface, _ bool) {
	// if perMessageDeflate := w.Opts().PerMessageDeflate(); perMessageDeflate != nil {
	// 	if data.Len() < perMessageDeflate.Threshold {
	// 		compress = false
	// 	}
	// }
	client_webtransport_log.Debug(`writing %#v`, data)

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

// DoClose gracefully closes the WebTransport connection.
// This method ensures proper cleanup of the WebTransport connection.
func (w *webTransport) DoClose() {
	if w.session != nil {
		defer w.session.CloseWithError(0, "")
	}
}

// Generates uri for connection.
func (w *webTransport) uri() *url.URL {
	query := url.Values{}
	for k, vs := range w.Query() {
		for _, v := range vs {
			query.Add(k, v)
		}
	}

	if w.Opts().TimestampRequests() {
		query.Set(w.Opts().TimestampParam(), request.RandomString())
	}

	if !w.SupportsBinary() {
		query.Set("b64", "1")
	}

	return w.CreateUri("https", query)
}
