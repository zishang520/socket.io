package socket

import (
	"compress/flate"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	"github.com/zishang520/socket.io/v3/pkg/version"
)

var (
	dotMapRegex = regexp.MustCompile(`\.map`)
	server_log  = log.NewLog("socket.io:server")
)

type (
	ParentNspNameMatchFn *func(string, any, func(error, bool))

	// Represents a Socket.IO server.
	//
	//	import (
	//		"github.com/zishang520/socket.io/v3/pkg/utils"
	//		"github.com/zishang520/socket.io/servers/socket/v3"
	//	)
	//
	//	io := socket.NewServer(nil, nil)
	//
	//	io.On("connection", func(clients ...any) {
	//		socket := clients[0].(*socket.Socket)
	//
	//		utils.Log().Info(`socket %s connected`, socket.Id())
	//
	//		// send an event to the client
	//		socket.Emit("foo", "bar")
	//
	//		socket.On("foobar", func(...any) {
	//			// an event was received from the client
	//		})
	//
	//		// upon disconnection
	//		socket.On("disconnect", func(reason ...any) {
	//			utils.Log().Info(`socket %s disconnected due to %s`, socket.Id(), reason[0])
	//		})
	//	})
	//	io.Listen(3000, nil)
	Server struct {
		*StrictEventEmitter

		// #readonly
		sockets Namespace
		// A reference to the underlying Engine.IO server.
		//
		//	clientsCount := io.Engine().ClientsCount()
		engine     engine.BaseServer
		_parser    parser.Parser
		encoder    parser.Encoder
		_nsps      *types.Map[string, Namespace]
		parentNsps *types.Map[ParentNspNameMatchFn, ParentNamespace]
		//
		// A subset of the {parentNsps} map, only containing {ParentNamespace} which are based on a regular
		// expression.
		parentNamespacesFromRegExp *types.Map[*regexp.Regexp, ParentNamespace]
		_adapter                   AdapterConstructor
		_serveClient               bool
		// #readonly
		opts            ServerOptionsInterface
		eio             engine.Server
		_path           string
		clientPathRegex *regexp.Regexp
		_connectTimeout time.Duration
		httpServer      *types.HttpServer
		_corsMiddleware engine.Middleware
	}
)

func MakeServer() *Server {
	s := &Server{
		_nsps:                      &types.Map[string, Namespace]{},
		parentNsps:                 &types.Map[ParentNspNameMatchFn, ParentNamespace]{},
		parentNamespacesFromRegExp: &types.Map[*regexp.Regexp, ParentNamespace]{},
	}
	return s
}

func NewServer(srv any, opts ServerOptionsInterface) *Server {
	s := MakeServer()

	s.Construct(srv, opts)

	return s
}

func (s *Server) Sockets() Namespace {
	return s.sockets
}

func (s *Server) Engine() engine.BaseServer {
	return s.engine
}

func (s *Server) Encoder() parser.Encoder {
	return s.encoder
}

func (s *Server) Construct(srv any, opts ServerOptionsInterface) {
	if opts == nil {
		opts = DefaultServerOptions()
	}

	if opts.GetRawPath() != nil {
		s.SetPath(opts.Path())
	} else {
		s.SetPath("/socket.io")
	}
	if opts.GetRawConnectTimeout() != nil {
		s.SetConnectTimeout(opts.ConnectTimeout())
	} else {
		s.SetConnectTimeout(45_000 * time.Millisecond)
	}
	s.SetServeClient(opts.ServeClient())
	if opts.GetRawParser() != nil {
		s._parser = opts.Parser()
	} else {
		s._parser = parser.NewParser()
	}
	s.encoder = s._parser.NewEncoder()
	s.opts = opts
	if opts.GetRawAdapter() != nil {
		s.SetAdapter(opts.Adapter())
	} else {
		if connectionStateRecovery := opts.ConnectionStateRecovery(); connectionStateRecovery != nil {
			if connectionStateRecovery.GetRawMaxDisconnectionDuration() == nil {
				connectionStateRecovery.SetMaxDisconnectionDuration(2 * 60 * 1000)
			}
			if connectionStateRecovery.GetRawSkipMiddlewares() == nil {
				connectionStateRecovery.SetSkipMiddlewares(true)
			}
			s.SetAdapter(&SessionAwareAdapterBuilder{})
		} else {
			s.SetAdapter(&AdapterBuilder{})
		}
	}
	s.sockets = s.Of("/", nil)

	s.StrictEventEmitter = s.sockets.EventEmitter()

	if srv != nil {
		s.Attach(srv, nil)
	}

	if s.opts.GetRawCors() != nil {
		s._corsMiddleware = types.MiddlewareWrapper(s.opts.Cors())
	}
}

func (s *Server) Opts() ServerOptionsInterface {
	return s.opts
}

// SetServeClient sets whether to serve the client code to browsers.
func (s *Server) SetServeClient(v bool) *Server {
	s._serveClient = v
	return s
}

// ServeClient returns whether the server is serving client code.
func (s *Server) ServeClient() bool {
	return s._serveClient
}

// _checkNamespace executes the middleware for an incoming namespace not already created on the server.
// name is the name of the incoming namespace, auth is the auth parameters, fn is the callback.
func (s *Server) _checkNamespace(name string, auth any, fn func(nsp Namespace)) {
	end := true
	s.parentNsps.Range(func(nextFn ParentNspNameMatchFn, pnsp ParentNamespace) bool {
		status := false
		(*nextFn)(name, auth, func(err error, allow bool) {
			if err != nil || !allow {
				status = true
				return
			}
			if nsp, ok := s._nsps.Load(name); ok {
				// the namespace was created in the meantime
				server_log.Debug("dynamic namespace %s already exists", name)
				fn(nsp)
				end = false
				return
			}
			namespace := pnsp.CreateChild(name)
			server_log.Debug("dynamic namespace %s was created", name)
			fn(namespace)
			end = false
		})
		return status // whether to continue traversing.
	})
	if end {
		fn(nil)
	}
}

// SetPath sets the client serving path.
func (s *Server) SetPath(v string) *Server {
	s._path = strings.TrimRight(v, "/")
	s.clientPathRegex = regexp.MustCompile(`^` + regexp.QuoteMeta(s._path) + `/socket\.io(\.msgpack|\.esm)?(\.min)?\.js(\.map)?(?:\?|$)`)
	return s
}

// Path returns the current client serving path.
func (s *Server) Path() string {
	return s._path
}

// SetConnectTimeout sets the delay after which a client without namespace is closed.
func (s *Server) SetConnectTimeout(v time.Duration) *Server {
	s._connectTimeout = v
	return s
}

// ConnectTimeout returns the current connect timeout duration.
func (s *Server) ConnectTimeout() time.Duration {
	return s._connectTimeout
}

// SetAdapter sets the adapter for rooms.
func (s *Server) SetAdapter(v AdapterConstructor) *Server {
	s._adapter = v
	s._nsps.Range(func(_ string, nsp Namespace) bool {
		nsp.InitAdapter()
		return true
	})
	return s
}

func (s *Server) Adapter() AdapterConstructor {
	return s._adapter
}

// Listen attaches socket.io to a server or port.
// srv is the server or port, opts are options passed to engine.io.
func (s *Server) Listen(srv any, opts *ServerOptions) *Server {
	return s.Attach(srv, opts)
}

// Attach attaches socket.io to a server or port.
// srv is the server or port, opts are options passed to engine.io.
func (s *Server) Attach(srv any, opts *ServerOptions) *Server {
	var server *types.HttpServer
	switch address := srv.(type) {
	case int:
		_address := fmt.Sprintf(":%d", address)
		// handle a port as a int
		server_log.Debug("creating http server and binding to %s", _address)
		server = types.NewWebServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "404 page not found", http.StatusNotFound)
		}))
		server.Listen(_address, nil)
	case string:
		// handle a port as a string
		server_log.Debug("creating http server and binding to %s", address)
		server = types.NewWebServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "404 page not found", http.StatusNotFound)
		}))
		server.Listen(address, nil)
	case *types.HttpServer:
		server = address
	default:
		panic(fmt.Errorf("trying to attach socket.io to express request handler %T, please pass a *types.HttpServer instance", address))
	}
	if opts == nil {
		opts = DefaultServerOptions()
	}

	// merge the options passed to the Socket.IO server
	opts.Assign(s.opts)
	// set engine.io path to `/socket.io`
	if opts.GetRawPath() == nil {
		opts.SetPath(s._path)
	}
	s.initEngine(server, opts)

	return s
}

// ServeHandler returns an http.Handler for the server.
func (s *Server) ServeHandler(opts *ServerOptions) http.Handler {
	// If an instance already exists, reuse it.
	if s.eio != nil {
		return s.eio
	}

	if opts == nil {
		opts = DefaultServerOptions()
	}

	// merge the options passed to the Socket.IO server
	opts.Assign(s.opts)
	// set engine.io path to `/socket.io`
	if opts.GetRawPath() == nil {
		opts.SetPath(s._path)
	}

	// initialize engine
	server_log.Debug("creating http.Handler-based engine with opts %v", opts)
	s.eio = engine.NewServer(opts)
	// bind to engine events
	s.Bind(s.eio)

	return s.eio
}

// initEngine initializes the engine.io server and attaches it to the HTTP server.
func (s *Server) initEngine(srv *types.HttpServer, opts ServerOptionsInterface) {
	// initialize engine
	server_log.Debug("creating engine.io instance with opts %+v", opts)
	s.eio = engine.Attach(srv, opts)

	// attach static file serving
	if s._serveClient {
		s.attachServe(srv, s.eio, opts)
	}

	// Export http server
	s.httpServer = srv

	// bind to engine events
	s.Bind(s.eio)
}

// attachServe attaches the static file serving handler.
func (s *Server) attachServe(srv *types.HttpServer, egs engine.Server, opts ServerOptionsInterface) {
	server_log.Debug("attaching client serving req handler")
	srv.HandleFunc(s._path+"/", func(w http.ResponseWriter, r *http.Request) {
		if s.clientPathRegex.MatchString(r.URL.Path) {
			if s._corsMiddleware != nil {
				s._corsMiddleware(types.NewHttpContext(w, r), func(error) {
					s.serve(w, r)
				})
			} else {
				s.serve(w, r)
			}
		} else {
			if opts.GetRawAddTrailingSlash() == nil || opts.AddTrailingSlash() {
				egs.ServeHTTP(w, r)
			} else {
				srv.DefaultHandler.ServeHTTP(w, r)
			}
		}
	})
}

// serve handles a request for serving client source and map files.
func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	filename := filepath.Base(r.URL.Path)
	isMap := dotMapRegex.MatchString(filename)
	_type := "source"
	if isMap {
		_type = "map"
	}
	// Per the standard, ETags must be quoted:
	// https://tools.ietf.org/html/rfc7232#section-2.3
	expectedEtag := `"` + version.VERSION + `"`
	if s.opts.GetRawClientVersion() != nil {
		expectedEtag = `"` + s.opts.ClientVersion() + `"`
	}
	weakEtag := "W/" + expectedEtag

	if etag := r.Header.Get("If-None-Match"); etag != "" {
		if expectedEtag == etag || weakEtag == etag {
			server_log.Debug("serve client %s 304", _type)
			w.WriteHeader(http.StatusNotModified)
			w.Write(nil)
			return
		}
	}

	server_log.Debug("serve client %s", _type)
	w.Header().Set("Cache-Control", "public, max-age=0")
	if isMap {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	}
	w.Header().Set("ETag", expectedEtag)
	s.sendFile(filename, w, r)
}

// sendFile sends a static file to the client.
func (Server) sendFile(filename string, w http.ResponseWriter, r *http.Request) {
	_file, err := os.Executable()
	if err != nil {
		server_log.Debug("Failed to get run path: %v", err)
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	// Construct the full, intended destination path
	basePath := filepath.Dir(filepath.Dir(_file))
	targetPath := filepath.Clean(filepath.Join(basePath, "client-dist", filename))

	// Verify the target path is still within the intended directory boundary
	if !strings.HasPrefix(targetPath, basePath) {
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	file, err := os.Open(targetPath)
	if err != nil {
		server_log.Debug("File read failed: %v", err)
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	defer file.Close()

	encoding := utils.Contains(r.Header.Get("Accept-Encoding"), []string{"gzip", "deflate", "br", "zstd"})

	switch encoding {
	case "br":
		br := brotli.NewWriterLevel(w, brotli.DefaultCompression)
		defer br.Close()
		w.Header().Set("Content-Encoding", "br")
		w.WriteHeader(http.StatusOK)
		io.Copy(br, file)
	case "gzip":
		gz, err := gzip.NewWriterLevel(w, gzip.DefaultCompression)
		if err != nil {
			server_log.Debug("Failed to compress data: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		io.Copy(gz, file)
	case "deflate":
		fl, err := flate.NewWriter(w, flate.DefaultCompression)
		if err != nil {
			server_log.Debug("Failed to compress data: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer fl.Close()
		w.Header().Set("Content-Encoding", "deflate")
		w.WriteHeader(http.StatusOK)
		io.Copy(fl, file)
	case "zstd":
		zd, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedDefault))
		if err != nil {
			server_log.Debug("Failed to compress data: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer zd.Close()
		w.Header().Set("Content-Encoding", "zstd")
		w.WriteHeader(http.StatusOK)
		io.Copy(zd, file)
	default:
		w.WriteHeader(http.StatusOK)
		io.Copy(w, file)
	}
}

// Bind binds socket.io to an engine.io instance.
// egs is the engine.io (or compatible) server.
func (s *Server) Bind(egs engine.BaseServer) *Server {
	s.engine = egs
	s.engine.On("connection", s.onconnection)
	return s
}

// onconnection is called with each incoming transport connection.
func (s *Server) onconnection(conns ...any) {
	conn := conns[0].(engine.Socket)
	server_log.Debug("incoming connection with id %s", conn.Id())
	client := NewClient(s, conn)
	if conn.Protocol() == 3 {
		client.connect("/", nil)
	}
}

// Of looks up a namespace by name or pattern and optionally registers a connection event handler.
// name can be a string, regexp, or ParentNspNameMatchFn; fn is the connection event handler.
func (s *Server) Of(name any, fn types.EventListener) Namespace {
	switch n := name.(type) {
	case ParentNspNameMatchFn:
		parentNsp := NewParentNamespace(s)
		server_log.Debug("initializing parent namespace %s", parentNsp.Name())

		s.parentNsps.Store(n, parentNsp)

		if fn != nil {
			parentNsp.On("connect", fn)
		}
		return parentNsp
	case *regexp.Regexp:
		parentNsp := NewParentNamespace(s)
		server_log.Debug("initializing parent namespace %s", parentNsp.Name())

		nfn := func(nsp string, _ any, next func(error, bool)) {
			next(nil, n.MatchString(nsp))
		}
		s.parentNsps.Store(ParentNspNameMatchFn(&nfn), parentNsp)
		s.parentNamespacesFromRegExp.Store(n, parentNsp)

		if fn != nil {
			parentNsp.On("connect", fn)
		}
		return parentNsp
	}

	n, ok := name.(string)
	if ok {
		if len(n) > 0 {
			if n[0] != '/' {
				n = "/" + n
			}
		} else {
			n = "/"
		}
	} else {
		n = "/"
	}

	var namespace Namespace

	if nsp, ok := s._nsps.Load(n); ok {
		namespace = nsp
	} else {
		s.parentNamespacesFromRegExp.Range(func(regex *regexp.Regexp, parentNamespace ParentNamespace) bool {
			if regex.MatchString(n) {
				server_log.Debug("attaching namespace %s to parent namespace %s", n, regex.String())
				namespace = parentNamespace.CreateChild(n)
				return false
			}
			return true
		})

		if namespace != nil {
			return namespace
		}

		server_log.Debug("initializing namespace %s", n)
		namespace = NewNamespace(s, n)
		s._nsps.Store(n, namespace)
		if n != "/" {
			s.sockets.EmitReserved("new_namespace", namespace)
		}
	}

	if fn != nil {
		namespace.On("connect", fn)
	}
	return namespace
}

// Close closes the server and all client connections. If fn is provided, it is called on error or when all connections are closed.
func (s *Server) Close(fn func(error)) {
	s._nsps.Range(func(_ string, nsp Namespace) bool {
		nsp.Sockets().Range(func(_ SocketId, socket *Socket) bool {
			socket._onclose("server shutting down")
			return true
		})
		nsp.Adapter().Close()
		return true
	})

	if s.httpServer != nil {
		s.httpServer.Close(fn)
		// The engine has been closed through the close event processing, and the subsequent process is exited here.
		return
	}

	if s.engine != nil {
		s.engine.Close()
	}

	if fn != nil {
		fn(nil)
	}
}

// Use registers a middleware function that is executed for every incoming Socket.
func (s *Server) Use(fn NamespaceMiddleware) *Server {
	s.sockets.Use(fn)
	return s
}

// To targets a room when emitting events. Returns a new BroadcastOperator for chaining.
func (s *Server) To(room ...Room) *BroadcastOperator {
	return s.sockets.To(room...)
}

// In targets a room when emitting events. Returns a new BroadcastOperator for chaining.
func (s *Server) In(room ...Room) *BroadcastOperator {
	return s.sockets.In(room...)
}

// Except excludes a room when emitting events. Returns a new BroadcastOperator for chaining.
func (s *Server) Except(room ...Room) *BroadcastOperator {
	return s.sockets.Except(room...)
}

// Emit broadcasts an event to all connected clients.
func (s *Server) Emit(ev string, args ...any) *Server {
	s.sockets.Emit(ev, args...)
	return s
}

// Send sends a "message" event to all clients. This mimics the WebSocket.send() method.
func (s *Server) Send(args ...any) *Server {
	// This type-cast is needed because EmitEvents likely doesn't have `message` as a key.
	// if you specify the EmitEvents, the type of args will be never.
	s.sockets.Emit("message", args...)
	return s
}

// Write sends a "message" event to all clients. Alias of Send.
func (s *Server) Write(args ...any) *Server {
	// This type-cast is needed because EmitEvents likely doesn't have `message` as a key.
	// if you specify the EmitEvents, the type of args will be never.
	s.sockets.Emit("message", args...)
	return s
}

// ServerSideEmit sends a message to other Socket.IO servers in the cluster.
// ev is the event name, args are the arguments (may include an acknowledgement callback).
func (s *Server) ServerSideEmit(ev string, args ...any) error {
	return s.sockets.ServerSideEmit(ev, args...)
}

// ServerSideEmitWithAck sends a message and expects an acknowledgement from other Socket.IO servers in the cluster.
// Returns a function that will be fulfilled when all servers have acknowledged the event.
func (s *Server) ServerSideEmitWithAck(ev string, args ...any) func(Ack) error {
	return s.sockets.ServerSideEmitWithAck(ev, args...)
}

// Compress sets the compress flag for subsequent event emissions.
func (s *Server) Compress(compress bool) *BroadcastOperator {
	return s.sockets.Compress(compress)
}

// Volatile sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to receive messages.
func (s *Server) Volatile() *BroadcastOperator {
	return s.sockets.Volatile()
}

// Local sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node.
func (s *Server) Local() *BroadcastOperator {
	return s.sockets.Local()
}

// Timeout adds a timeout for the next operation.
func (s *Server) Timeout(timeout time.Duration) *BroadcastOperator {
	return s.sockets.Timeout(timeout)
}

// FetchSockets returns a function to fetch the matching socket instances.
func (s *Server) FetchSockets() func(func([]*RemoteSocket, error)) {
	return s.sockets.FetchSockets()
}

// SocketsJoin makes the matching socket instances join the specified rooms.
func (s *Server) SocketsJoin(room ...Room) {
	s.sockets.SocketsJoin(room...)
}

// SocketsLeave makes the matching socket instances leave the specified rooms.
func (s *Server) SocketsLeave(room ...Room) {
	s.sockets.SocketsLeave(room...)
}

// DisconnectSockets makes the matching socket instances disconnect. If status is true, closes the underlying connection.
func (s *Server) DisconnectSockets(status bool) {
	s.sockets.DisconnectSockets(status)
}
