// Package engine provides the Engine.IO server implementation, including HTTP/WebSocket/WebTransport handling and protocol management.
package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/errors"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	webtrans "github.com/zishang520/socket.io/v3/pkg/webtransport"
	"github.com/quic-go/webtransport-go"
)

type server struct {
	BaseServer

	httpServer *types.HttpServer
}

// new server.
func MakeServer() Server {
	s := &server{BaseServer: MakeBaseServer()}

	s.Prototype(s)

	return s
}

// create server.
func NewServer(opt any) Server {
	s := MakeServer()

	s.Construct(opt)

	return s
}

func (s *server) SetHttpServer(httpServer *types.HttpServer) {
	s.httpServer = httpServer
}

func (s *server) HttpServer() *types.HttpServer {
	return s.httpServer
}

func (s *server) Init() {
}

func (s *server) Cleanup() {
}

func (s *server) CreateTransport(transportName string, ctx *types.HttpContext) (transports.Transport, error) {
	if transport, ok := s.TransportsByName()[transportName]; ok {
		return transport.New(ctx), nil
	}
	return nil, errors.ErrUnsupportedTransport
}

// Handles an Engine.IO HTTP request.
func (s *server) HandleRequest(ctx *types.HttpContext) {
	server_log.Debug(`handling "%s" http request "%s"`, ctx.Method(), ctx.Request().RequestURI)

	callback := func(codeMessage *types.CodeMessage, errorContext map[string]any) {
		if codeMessage != nil {
			s.emitAbortRequest(ctx, codeMessage, errorContext)
			return
		}

		if sid := ctx.Query().Peek("sid"); sid != "" {
			server_log.Debug("setting new request for existing client")
			if socket, ok := s.Clients().Load(sid); ok {
				socket.Transport().OnRequest(ctx)
			} else {
				abortRequest(ctx, UNKNOWN_SID, map[string]any{"sid": sid})
			}
		} else {
			if codeMessage, t := s.Handshake(ctx.Query().Peek("transport"), ctx); t == nil {
				abortRequest(ctx, codeMessage, nil)
			}
		}
	}

	s.ApplyMiddlewares(ctx, func(err error) {
		if err != nil {
			callback(BAD_REQUEST, map[string]any{"name": "MIDDLEWARE_FAILURE"})
		} else {
			callback(s.Verify(ctx, false))
		}
	})

	// Wait for data to be written to the client.
	<-ctx.Done()
}

// Handles an Engine.IO HTTP Upgrade.
func (s *server) HandleUpgrade(ctx *types.HttpContext) {
	callback := func(codeMessage *types.CodeMessage, errorContext map[string]any) {
		if codeMessage != nil {
			s.emitAbortUpgrade(ctx, codeMessage, errorContext)
			return
		}

		wsc := &types.WebSocketConn{EventEmitter: types.NewEventEmitter()}

		ws := &websocket.Upgrader{
			ReadBufferSize:    1024,
			WriteBufferSize:   1024,
			EnableCompression: s.Opts().PerMessageDeflate() != nil,
			Error: func(_ http.ResponseWriter, _ *http.Request, _ int, reason error) {
				if websocket.IsUnexpectedCloseError(reason) {
					wsc.Emit("close")
				} else {
					wsc.Emit("error", reason)
				}
			},
			CheckOrigin: func(*http.Request) bool {
				// Verified in *server.Verify()
				return true
			},
		}

		// delegate to ws
		if conn, err := ws.Upgrade(ctx.Response(), ctx.Request(), ctx.ResponseHeaders().All()); err != nil {
			s.emitAbortRequest(ctx, BAD_REQUEST, map[string]any{"name": "UPGRADE_FAILURE"})
			server_log.Debug("websocket error before upgrade: %s", err.Error())
		} else {
			conn.SetReadLimit(s.Opts().MaxHttpBufferSize())
			wsc.Conn = conn
			s.onWebSocket(ctx, wsc)
		}
	}

	s.ApplyMiddlewares(ctx, func(err error) {
		if err != nil {
			callback(BAD_REQUEST, map[string]any{"name": "MIDDLEWARE_FAILURE"})
		} else {
			callback(s.Verify(ctx, true))
		}
	})
}

// Called upon a ws.io connection.
func (s *server) onWebSocket(ctx *types.HttpContext, wsc *types.WebSocketConn) {
	onUpgradeError := func(...any) {
		server_log.Debug("websocket error before upgrade")
		// wsc.close() not needed
	}

	wsc.On("error", onUpgradeError)

	transportName := ctx.Query().Peek("transport")
	if transport, ok := s.TransportsByName()[transportName]; ok && !transport.HandlesUpgrades() {
		server_log.Debug("transport doesnt handle upgraded requests")
		wsc.Close()
		return
	}

	// get client id
	id := ctx.Query().Peek("sid")

	// keep a reference to the ws.Socket
	ctx.Websocket = wsc

	if len(id) == 0 {
		if codeMessage, t := s.Handshake(transportName, ctx); t == nil {
			abortUpgrade(ctx, codeMessage, nil)
		} else {
			// transport error handling takes over
			wsc.RemoveListener("error", onUpgradeError)
		}
		return
	}

	client, ok := s.Clients().Load(id)

	if !ok {
		server_log.Debug("upgrade attempt for closed client")
		wsc.Close()
	} else if client.Upgrading() {
		server_log.Debug("transport has already been trying to upgrade")
		wsc.Close()
	} else if client.Upgraded() {
		server_log.Debug("transport had already been upgraded")
		wsc.Close()
	} else {
		server_log.Debug("upgrading existing transport")

		// transport error handling takes over
		wsc.RemoveListener("error", onUpgradeError)

		transport, err := s.CreateTransport(transportName, ctx)
		if err != nil {
			server_log.Debug("upgrading not existing transport")
			wsc.Close()
		} else {
			transport.SetPerMessageDeflate(s.Opts().PerMessageDeflate())
			client.MaybeUpgrade(transport)
		}
	}
}

func (s *server) OnWebTransportSession(ctx *types.HttpContext, wt *webtransport.Server) {
	if allowRequest := s.Opts().AllowRequest(); allowRequest != nil {
		if err := allowRequest(ctx); err != nil {
			s.emitAbortRequest(ctx, FORBIDDEN, map[string]any{"message": err.Error()})
			return
		}
	}

	session, err := wt.Upgrade(ctx.Response(), ctx.Request())
	if err != nil {
		server_log.Debug("upgrading failed: %s", err.Error())
		s.emitAbortRequest(ctx, BAD_REQUEST, map[string]any{"name": "UPGRADE_FAILURE"})
		return
	}

	timeout := utils.SetTimeout(func() {
		server_log.Debug("the client failed to establish a bidirectional stream in the given period")
		session.CloseWithError(0, "")
	}, s.Opts().UpgradeTimeout())

	stream, err := session.AcceptStream(context.Background())
	if err != nil {
		server_log.Debug("session is closed")
		abortUpgrade(ctx, BAD_REQUEST, nil)
		return
	}

	wtc := webtrans.NewConn(session, stream, true, 0, 0, nil, nil, nil)
	wtc.SetReadLimit(s.Opts().MaxHttpBufferSize())

	ctx.WebTransport = &types.WebTransportConn{EventEmitter: types.NewEventEmitter(), Conn: wtc}

	mt, message, err := wtc.NextReader()
	if err != nil {
		server_log.Debug("stream is closed: %s", err.Error())
		abortUpgrade(ctx, BAD_REQUEST, nil)
		return
	}

	var data types.BufferInterface

	switch mt {
	case webtrans.BinaryMessage:
		data = types.NewBytesBuffer(nil)
		if _, err := data.ReadFrom(message); err != nil {
			server_log.Debug("WebTransport handshake data read failed: %s", err.Error())
			abortUpgrade(ctx, BAD_REQUEST, nil)
			return
		}
	case webtrans.TextMessage:
		data = types.NewStringBuffer(nil)
		if _, err := data.ReadFrom(message); err != nil {
			server_log.Debug("WebTransport handshake data read failed: %s", err.Error())
			abortUpgrade(ctx, BAD_REQUEST, nil)
			return
		}
	}
	if c, ok := message.(io.Closer); ok {
		c.Close()
	}

	utils.ClearTimeout(timeout)

	value, _ := parser.Parserv4().DecodePacket(data)

	if v, ok := value.Data.(io.Closer); ok {
		defer v.Close()
	}

	if value.Type != packet.OPEN {
		server_log.Debug("invalid WebTransport handshake")
		abortUpgrade(ctx, BAD_REQUEST, nil)
		return
	}

	if data, ok := value.Data.(types.BufferInterface); ok && data.Len() == 0 {
		ctx.Query().Set("EIO", "4")
		if codeMessage, t := s.Handshake(ctx.Request().Proto, ctx); t == nil {
			abortUpgrade(ctx, codeMessage, nil)
		}
		return
	}

	var wth *struct {
		Sid string `json:"sid"`
	}

	if json.NewDecoder(value.Data).Decode(&wth) != nil {
		server_log.Debug("invalid WebTransport handshake")
		abortUpgrade(ctx, BAD_REQUEST, nil)
		return
	}

	if len(wth.Sid) == 0 {
		server_log.Debug("invalid WebTransport handshake")
		abortUpgrade(ctx, BAD_REQUEST, nil)
		return
	}

	client, ok := s.Clients().Load(wth.Sid)

	if !ok {
		server_log.Debug("upgrade attempt for closed client")
		session.CloseWithError(0, "")
	} else if client.Upgrading() {
		server_log.Debug("transport has already been trying to upgrade")
		session.CloseWithError(0, "")
	} else if client.Upgraded() {
		server_log.Debug("transport had already been upgraded")
		session.CloseWithError(0, "")
	} else {
		server_log.Debug("upgrading existing transport")

		transport, err := s.CreateTransport(ctx.Request().Proto, ctx)
		if err != nil {
			server_log.Debug("upgrading not existing transport")
			session.CloseWithError(0, "")
		} else {
			transport.SetPerMessageDeflate(s.Opts().PerMessageDeflate())
			client.MaybeUpgrade(transport)
		}
	}
}

// Captures upgrade requests for a types.HttpServer.
func (s *server) Attach(server *types.HttpServer, opts any) {
	options, _ := opts.(config.AttachOptionsInterface)
	path := s.ComputePath(options)

	server.Once("close", func(...any) {
		s.Close()
	})

	server.Once("listening", func(...any) {
		s.Proto().Init()
	})

	server.HandleFunc(path, s.ServeHTTP)
}

// Captures upgrade requests for a http.Handler, Need to handle server shutdown disconnecting client connections.
func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !websocket.IsWebSocketUpgrade(r) {
		server_log.Debug(`intercepting request for path "%s"`, utils.CleanPath(r.URL.Path))
		s.HandleRequest(types.NewHttpContext(w, r))
	} else if s.Transports().Has(transports.WEBSOCKET) {
		s.HandleUpgrade(types.NewHttpContext(w, r))
	} else {
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
	}
}

// Close the HTTP long-polling request
func abortRequest(ctx *types.HttpContext, codeMessage *types.CodeMessage, errorContext map[string]any) {
	server_log.Debug("abortRequest %d, %+v", codeMessage.Code, errorContext)
	statusCode := http.StatusBadRequest
	if codeMessage == FORBIDDEN {
		statusCode = http.StatusForbidden
	}
	message := codeMessage.Message
	if errorContext != nil {
		if m, ok := errorContext["message"]; ok {
			message = utils.TryCast[string](m)
		}
	}
	ctx.ResponseHeaders().Set("Content-Type", "application/json")
	ctx.SetStatusCode(statusCode)
	if b, err := json.Marshal(types.CodeMessage{Code: codeMessage.Code, Message: message}); err == nil {
		ctx.Write(b)
		return
	}
	io.WriteString(ctx, `{"code":400,"message":"Bad request"}`)
}

func (s *server) emitAbortRequest(ctx *types.HttpContext, codeMessage *types.CodeMessage, errorContext map[string]any) {
	s.Emit("connection_error", &types.ErrorMessage{
		CodeMessage: codeMessage,
		Req:         ctx,
		Context:     errorContext,
	})
	abortRequest(ctx, codeMessage, errorContext)
}

// Close the WebSocket connection
func abortUpgrade(ctx *types.HttpContext, codeMessage *types.CodeMessage, errorContext map[string]any) {
	ctx.On("error", func(...any) {
		server_log.Debug("ignoring error from closed connection")
	})

	message := codeMessage.Message
	if errorContext != nil {
		if m, ok := errorContext["message"]; ok {
			message = utils.TryCast[string](m)
		}
	}

	if ctx.Websocket != nil {
		defer ctx.Websocket.Close()
		ctx.Websocket.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, message))
	} else if ctx.WebTransport != nil {
		ctx.WebTransport.CloseWithError(http.StatusBadRequest, message)
	} else {
		ctx.SetStatusCode(http.StatusBadRequest)
		io.WriteString(ctx, message)
	}
}

func (s *server) emitAbortUpgrade(ctx *types.HttpContext, codeMessage *types.CodeMessage, errorContext map[string]any) {
	s.Emit("connection_error", &types.ErrorMessage{
		CodeMessage: codeMessage,
		Req:         ctx,
		Context:     errorContext,
	})
	abortUpgrade(ctx, codeMessage, errorContext)
}
