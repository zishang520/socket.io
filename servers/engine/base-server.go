// Package engine provides the core Engine.IO server implementation, including base server logic, protocol error handling, and middleware management.
package engine

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/errors"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	server_log = log.NewLog("engine")

	// Protocol errors mappings.
	UNKNOWN_TRANSPORT            = &types.CodeMessage{Code: 0, Message: `Transport unknown`}
	UNKNOWN_SID                  = &types.CodeMessage{Code: 1, Message: `Session ID unknown`}
	BAD_HANDSHAKE_METHOD         = &types.CodeMessage{Code: 2, Message: `Bad handshake method`}
	BAD_REQUEST                  = &types.CodeMessage{Code: 3, Message: `Bad request`}
	FORBIDDEN                    = &types.CodeMessage{Code: 4, Message: `Forbidden`}
	UNSUPPORTED_PROTOCOL_VERSION = &types.CodeMessage{Code: 4, Message: `Unsupported protocol version`}
)

type baseServer struct {
	types.EventEmitter

	// Prototype interface, used to implement interface method rewriting
	_proto_ BaseServer

	opts config.ServerOptionsInterface

	transports        *types.Set[string]       // Available transport types
	_transportsByName map[string]TransportCtor // Transport constructors by name

	clients      *types.Map[string, Socket]
	clientsCount atomic.Uint64
	middlewares  []Middleware
}

func MakeBaseServer() BaseServer {
	baseServer := &baseServer{
		EventEmitter: types.NewEventEmitter(),

		clients: &types.Map[string, Socket]{},
	}

	baseServer.Prototype(baseServer)

	return baseServer
}

func (bs *baseServer) Prototype(server BaseServer) {
	bs._proto_ = server
}

func (bs *baseServer) Proto() BaseServer {
	return bs._proto_
}

func (bs *baseServer) Opts() config.ServerOptionsInterface {
	return bs.opts
}

func (bs *baseServer) Clients() *types.Map[string, Socket] {
	return bs.clients
}

func (bs *baseServer) ClientsCount() uint64 {
	return bs.clientsCount.Load()
}

func (bs *baseServer) Middlewares() []Middleware {
	return bs.middlewares
}

func (bs *baseServer) Transports() *types.Set[string] {
	return bs.transports
}

func (bs *baseServer) TransportsByName() map[string]transports.TransportCtor {
	return bs._transportsByName
}

// BaseServer build.
func (bs *baseServer) Construct(opt any) {
	opts, _ := opt.(config.ServerOptionsInterface)

	options := config.DefaultServerOptions()
	options.SetPingTimeout(20_000 * time.Millisecond)
	options.SetPingInterval(25_000 * time.Millisecond)
	options.SetUpgradeTimeout(10_000 * time.Millisecond)
	options.SetMaxHttpBufferSize(1e6)
	options.SetTransports(types.NewSet(Polling, WebSocket))
	options.SetAllowUpgrades(true)
	options.SetHttpCompression(&types.HttpCompression{Threshold: 1024})
	options.SetCors(nil)
	options.SetAllowEIO3(false)

	bs.opts = options.Assign(opts)

	bs.transports = types.NewSet[string]()
	bs._transportsByName = map[string]TransportCtor{}
	if transports := bs.opts.Transports(); transports != nil {
		for _, transport := range transports.Keys() {
			transportName := transport.Name()
			bs.transports.Add(transportName)
			bs._transportsByName[transportName] = transport
		}
	}

	if opts != nil {
		if cookie := opts.Cookie(); cookie != nil {
			if len(cookie.Name) == 0 {
				cookie.Name = "io"
			}
			if len(cookie.Path) == 0 {
				cookie.Path = "/"
			}
			if len(cookie.Path) > 0 {
				cookie.HttpOnly = true
			}
			if cookie.SameSite == http.SameSiteDefaultMode {
				cookie.SameSite = http.SameSiteLaxMode
			}
			bs.opts.SetCookie(cookie)
		}
	}

	if cors := bs.opts.Cors(); cors != nil {
		bs.Use(types.MiddlewareWrapper(cors))
	}

	bs._proto_.Init()
}

// abstract
func (bs *baseServer) Init() {
}

// Compute the pathname of the requests that are handled by the server
func (bs *baseServer) ComputePath(options config.AttachOptionsInterface) string {
	path := "/engine.io"

	if options != nil {
		if options.GetRawPath() != nil {
			path = strings.TrimRight(options.Path(), "/")
		}
		if options.GetRawAddTrailingSlash() == nil || options.AddTrailingSlash() {
			// normalize path
			path += "/"
		}
	}

	return path
}

// Returns a list of available transports for upgrade given a certain transport.
func (bs *baseServer) Upgrades(transport string) []string {
	if !bs.opts.AllowUpgrades() {
		return nil
	}
	return bs._transportsByName[transport].UpgradesTo()
}

// Verifies a request.
func (bs *baseServer) Verify(ctx *types.HttpContext, upgrade bool) (*types.CodeMessage, map[string]any) {
	// transport check
	transport := ctx.Query().Peek("transport")
	if !bs.transports.Has(transport) || transport == transports.WEBTRANSPORT {
		server_log.Debug(`unknown transport "%s"`, transport)
		return UNKNOWN_TRANSPORT, map[string]any{"transport": transport}
	}

	// 'Origin' header check
	if origin := ctx.Headers().Peek("Origin"); utils.CheckInvalidHeaderChar(origin) {
		ctx.Headers().Remove("Origin")
		server_log.Debug("origin header invalid")
		return BAD_REQUEST, map[string]any{"name": "INVALID_ORIGIN", "origin": origin}
	}

	// sid check
	sid := ctx.Query().Peek("sid")
	if len(sid) > 0 {
		scoket, ok := bs.clients.Load(sid)
		if !ok {
			server_log.Debug(`unknown sid "%s"`, sid)
			return UNKNOWN_SID, map[string]any{"sid": sid}
		}
		if previousTransport := scoket.Transport().Name(); !upgrade && previousTransport != transport {
			server_log.Debug("bad request: unexpected transport without upgrade")
			return BAD_REQUEST, map[string]any{"name": "TRANSPORT_MISMATCH", "transport": transport, "previousTransport": previousTransport}
		}
	} else {
		// handshake is GET only
		if method := ctx.Method(); method != http.MethodGet {
			return BAD_HANDSHAKE_METHOD, map[string]any{"method": method}
		}

		if transport == transports.WEBSOCKET && !upgrade {
			server_log.Debug("invalid transport upgrade")
			return BAD_REQUEST, map[string]any{"name": "TRANSPORT_HANDSHAKE_ERROR"}
		}

		if allowRequest := bs.opts.AllowRequest(); allowRequest != nil {
			if err := allowRequest(ctx); err != nil {
				return FORBIDDEN, map[string]any{"message": err.Error()}
			}
		}
	}

	return nil, nil
}

// Adds a new middleware.
func (bs *baseServer) Use(fn Middleware) {
	// It seems that there is no need to lock? ? ?
	bs.middlewares = append(bs.middlewares, fn)
}

/**
 * Apply the middlewares to the request.
 */
func (bs *baseServer) ApplyMiddlewares(ctx *types.HttpContext, callback func(error)) {
	if len(bs.middlewares) == 0 {
		server_log.Debug("no middleware to apply, skipping")
		callback(nil)
		return
	}
	var apply func(int)
	apply = func(i int) {
		server_log.Debug("applying middleware n°%d", i+1)
		bs.middlewares[i](ctx, func(err error) {
			if err != nil {
				callback(err)
				return
			}
			if i+1 < len(bs.middlewares) {
				apply(i + 1)
			} else {
				callback(nil)
			}
		})
	}

	apply(0)
}

// Closes all clients.
func (bs *baseServer) Close() BaseServer {
	server_log.Debug("closing all open clients")
	bs.clients.Range(func(_ string, client Socket) bool {
		client.Close(true)
		return true
	})

	bs._proto_.Cleanup()

	return bs
}

func (bs *baseServer) Cleanup() {
}

// generate a socket id.
// Overwrite this method to generate your custom socket id
func (bs *baseServer) GenerateId(*types.HttpContext) (string, error) {
	return utils.Base64Id().GenerateId()
}

// Handshakes a new client.
func (bs *baseServer) Handshake(transportName string, ctx *types.HttpContext) (*types.CodeMessage, transports.Transport) {
	protocol := 3 // 3rd revision by default
	if ctx.Query().Peek("EIO") == "4" {
		protocol = 4
	}

	if protocol == 3 && !bs.opts.AllowEIO3() {
		server_log.Debug("unsupported protocol version")
		bs.Emit("connection_error", &types.ErrorMessage{
			CodeMessage: UNSUPPORTED_PROTOCOL_VERSION,
			Req:         ctx,
			Context: map[string]any{
				"protocol": protocol,
			},
		})
		return UNSUPPORTED_PROTOCOL_VERSION, nil
	}

	id, err := bs.GenerateId(ctx)
	if err != nil {
		server_log.Debug("error while generating an id")
		bs.Emit("connection_error", &types.ErrorMessage{
			CodeMessage: BAD_REQUEST,
			Req:         ctx,
			Context: map[string]any{
				"name":  "ID_GENERATION_ERROR",
				"error": err,
			},
		})
		return BAD_REQUEST, nil
	}

	server_log.Debug(`handshaking client "%s" (%s)`, id, transportName)

	transport, err := bs._proto_.CreateTransport(transportName, ctx)
	if err != nil {
		server_log.Debug(`handshaking client "%s" (%s)`, id, transportName)
		bs.Emit("connection_error", &types.ErrorMessage{
			CodeMessage: BAD_REQUEST,
			Req:         ctx,
			Context: map[string]any{
				"name":  "TRANSPORT_HANDSHAKE_ERROR",
				"error": err,
			},
		})
		return BAD_REQUEST, nil
	}
	if transports.POLLING == transportName {
		transport.SetMaxHttpBufferSize(bs.opts.MaxHttpBufferSize())
		transport.SetHttpCompression(bs.opts.HttpCompression())
	} else if transports.WEBSOCKET == transportName {
		transport.SetPerMessageDeflate(bs.opts.PerMessageDeflate())
	} else if transports.WEBTRANSPORT == transportName {
		transport.SetMaxHttpBufferSize(bs.opts.MaxHttpBufferSize())
	}

	transport.On("headers", func(args ...any) {
		headers, req := args[0].(*utils.ParameterBag), args[1].(*types.HttpContext)
		if !ctx.Query().Has("sid") {
			if cookie := bs.opts.Cookie(); cookie != nil {
				headers.Set("Set-Cookie", cookie.String())
			}
			bs.Emit("initial_headers", headers, req)
		}
		bs.Emit("headers", headers, req)
	})

	transport.OnRequest(ctx)

	socket := NewSocket(id, bs, transport, ctx, protocol)

	bs.clients.Store(id, socket)
	bs.clientsCount.Add(1)

	socket.Once("close", func(...any) {
		bs.clients.Delete(id)
		bs.clientsCount.Add(^uint64(0))
	})

	bs.Emit("connection", socket)

	return nil, transport
}

// abstract
func (*baseServer) CreateTransport(string, *types.HttpContext) (transports.Transport, error) {
	return nil, errors.ErrTransportNotImplemented
}
