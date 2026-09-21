// Package engine provides the core Engine.IO server implementation, including base server logic, protocol error handling, and middleware management.
package engine

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/errors"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	serverLog = log.NewLog("engine")

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
	middlewareMu sync.RWMutex
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

// Middlewares returns the registered slice. Modifying its entries requires
// external synchronization with registration and request snapshots.
func (bs *baseServer) Middlewares() []Middleware {
	bs.middlewareMu.RLock()
	defer bs.middlewareMu.RUnlock()
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
	options.SetIdleTimeout(120 * time.Second)
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
			if len(cookie.Path) > 0 {
				cookie.HttpOnly = true
			}
			if len(cookie.Path) == 0 {
				cookie.Path = "/"
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
	ctor, ok := bs._transportsByName[transport]
	if !ok {
		return nil
	}
	return ctor.UpgradesTo()
}

// Verifies a request.
func (bs *baseServer) Verify(ctx *types.HttpContext, upgrade bool) (*types.CodeMessage, map[string]any) {
	// transport check
	transport := ctx.Query().Peek("transport")
	if !bs.transports.Has(transport) || transport == transports.WEBTRANSPORT {
		serverLog.Debug(`unknown transport "%s"`, transport)
		return UNKNOWN_TRANSPORT, map[string]any{"transport": transport}
	}

	// 'Origin' header check
	if origin := ctx.Headers().Peek("Origin"); utils.CheckInvalidHeaderChar(origin) {
		ctx.Headers().Remove("Origin")
		serverLog.Debug("origin header invalid")
		return BAD_REQUEST, map[string]any{"name": "INVALID_ORIGIN", "origin": origin}
	}

	// sid check
	sid := ctx.Query().Peek("sid")
	if len(sid) > 0 {
		// Validate SID format to prevent abuse (e.g. excessively long values)
		if !utils.IsValidSid(sid) {
			serverLog.Debug(`invalid sid format "%s"`, sid)
			return BAD_REQUEST, map[string]any{"name": "INVALID_SID", "sid": sid}
		}
		socket, ok := bs.clients.Load(sid)
		if !ok {
			serverLog.Debug(`unknown sid "%s"`, sid)
			return UNKNOWN_SID, map[string]any{"sid": sid}
		}
		if previousTransport := socket.Transport().Name(); !upgrade && previousTransport != transport {
			serverLog.Debug("bad request: unexpected transport without upgrade")
			return BAD_REQUEST, map[string]any{"name": "TRANSPORT_MISMATCH", "transport": transport, "previousTransport": previousTransport}
		}
	} else {
		// handshake is GET only
		if method := ctx.Method(); method != http.MethodGet {
			return BAD_HANDSHAKE_METHOD, map[string]any{"method": method}
		}

		if transport == transports.WEBSOCKET && !upgrade {
			serverLog.Debug("invalid transport upgrade")
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
	bs.middlewareMu.Lock()
	defer bs.middlewareMu.Unlock()
	bs.middlewares = append(bs.middlewares, fn)
}

// Apply the middlewares to the request.
func (bs *baseServer) ApplyMiddlewares(ctx *types.HttpContext, callback func(error)) {
	bs.middlewareMu.RLock()
	middlewares := make([]Middleware, len(bs.middlewares))
	copy(middlewares, bs.middlewares)
	bs.middlewareMu.RUnlock()

	if len(middlewares) == 0 {
		serverLog.Debug("no middleware to apply, skipping")
		callback(nil)
		return
	}
	runMiddlewares(ctx, middlewares, 0, callback)
}

func runMiddlewares(ctx *types.HttpContext, middlewares []Middleware, index int, callback func(error)) {
	serverLog.Debug("applying middleware n°%d", index+1)
	middlewares[index](ctx, func(err error) {
		if err != nil {
			callback(err)
			return
		}
		if index+1 < len(middlewares) {
			runMiddlewares(ctx, middlewares, index+1, callback)
		} else {
			callback(nil)
		}
	})
}

// Closes all clients.
func (bs *baseServer) Close() BaseServer {
	serverLog.Debug("closing all open clients")
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
func (bs *baseServer) GenerateId(*types.HttpContext) string {
	return utils.Base64Id().GenerateId()
}

// Handshakes a new client.
func (bs *baseServer) Handshake(transportName string, ctx *types.HttpContext) (*types.CodeMessage, transports.Transport) {
	protocol := 3 // 3rd revision by default
	if ctx.Query().Peek("EIO") == "4" {
		protocol = 4
	}

	if protocol == 3 && !bs.opts.AllowEIO3() {
		serverLog.Debug("unsupported protocol version")
		bs.Emit("connection_error", &types.ErrorMessage{
			CodeMessage: UNSUPPORTED_PROTOCOL_VERSION,
			Req:         ctx,
			Context: map[string]any{
				"protocol": protocol,
			},
		})
		return UNSUPPORTED_PROTOCOL_VERSION, nil
	}

	ctx.IdleTimeout = bs.opts.IdleTimeout()
	var transport transports.Transport
	var socket Socket
	if !withTransportInitialization(ctx, bs.opts.UpgradeTimeout(), func(aborted func() bool) bool {
		id := bs._proto_.GenerateId(ctx)
		if aborted() {
			return false
		}
		serverLog.Debug(`handshaking client "%s" (%s)`, id, transportName)

		var err error
		transport, err = bs._proto_.CreateTransport(transportName, ctx)
		if err != nil {
			serverLog.Debug(`error creating transport "%s": %v`, transportName, err)
			bs.Emit("connection_error", &types.ErrorMessage{
				CodeMessage: BAD_REQUEST,
				Req:         ctx,
				Context: map[string]any{
					"name":  "TRANSPORT_HANDSHAKE_ERROR",
					"error": err,
				},
			})
			return false
		}
		if state := transport.ReadyState(); aborted() || state == "closing" || state == "closed" {
			transport.Close()
			return false
		}

		if transports.POLLING == transportName {
			transport.SetMaxHttpBufferSize(bs.opts.MaxHttpBufferSize())
			transport.SetHttpCompression(bs.opts.HttpCompression())
		} else if transports.WEBSOCKET == transportName {
			transport.SetPerMessageDeflate(bs.opts.PerMessageDeflate())
		}

		_ = transport.On("headers", func(args ...any) {
			headers, req := slices.TryGetAny[*types.ParameterBag](args, 0), slices.TryGetAny[*types.HttpContext](args, 1)
			if !ctx.Query().Has("sid") {
				if cookie := bs.opts.Cookie(); cookie != nil {
					headers.Set("Set-Cookie", cookie.String())
				}
				bs.Emit("initial_headers", headers, req)
			}
			bs.Emit("headers", headers, req)
		})

		transport.OnRequest(ctx)

		socket = NewSocket(id, bs._proto_, transport, ctx, protocol)
		if aborted() || socket.ReadyState() == "closed" {
			return false
		}

		bs.clients.Store(id, socket)
		bs.clientsCount.Add(1)

		onClose := func(...any) {
			if bs.clients.CompareAndDelete(id, socket) {
				bs.clientsCount.Add(^uint64(0))
			}
		}
		_ = socket.Once("close", onClose)
		// Closing can race registration, including during the OPEN write. Check
		// after subscribing so a close just before the subscription is not lost.
		if aborted() || socket.ReadyState() == "closed" {
			onClose()
			return false
		}

		bs.Emit("connection", socket)
		return true
	}) {
		if socket != nil {
			socket.Close(true)
		}
		return BAD_REQUEST, nil
	}

	return nil, transport
}

// withTransportInitialization owns read permission and the completion callback.
// Its timer and close listener are released before successful completion runs.
func withTransportInitialization(ctx *types.HttpContext, timeout time.Duration, initialize func(aborted func() bool) bool) (initialized bool) {
	defer func() {
		if ready := ctx.TakeTransportReady(); initialized && ready != nil {
			ready()
		}
	}()

	var connection types.EventEmitter
	var closeConnection func()
	if conn := ctx.Websocket; conn != nil {
		connection, closeConnection = conn, func() { _ = conn.Close() }
	} else if conn := ctx.WebTransport; conn != nil {
		connection, closeConnection = conn, func() { _ = conn.CloseWithError(0, "") }
	} else {
		return initialize(func() bool { return false })
	}

	const pending, ready, canceled = 0, 1, 2
	var state atomic.Uint32
	permission := make(chan bool, 1)
	ctx.SetTransportReadPermission(permission)
	resolve := func(result uint32) bool {
		if !state.CompareAndSwap(pending, result) {
			return false
		}
		if result == ready {
			permission <- true
		}
		// Release waiting readers before invoking any connection close callbacks.
		close(permission)
		return true
	}
	abort := func() {
		if resolve(canceled) {
			closeConnection()
		}
	}
	defer abort()
	onClose := func(...any) { resolve(canceled) }
	_ = connection.Once("close", onClose)
	defer connection.RemoveListener("close", onClose)

	if timeout <= 0 {
		return false
	}
	deadline := time.Now().Add(timeout)
	timer := time.AfterFunc(timeout, abort)
	defer timer.Stop()

	aborted := func() bool { return state.Load() == canceled }
	if aborted() || !initialize(aborted) {
		return false
	}
	// The budget may have elapsed before the timer callback gets to run.
	if !time.Now().Before(deadline) {
		return false
	}
	return resolve(ready)
}

// abstract
func (*baseServer) CreateTransport(string, *types.HttpContext) (transports.Transport, error) {
	return nil, errors.ErrTransportNotImplemented
}
