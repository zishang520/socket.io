package socket

import (
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"github.com/andybalholm/brotli"
	"github.com/zishang520/engine.io/engine"
	"github.com/zishang520/engine.io/log"
	"github.com/zishang520/engine.io/types"
	"github.com/zishang520/engine.io/utils"
	"github.com/zishang520/socket.io/parser"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const clientVersion = "4.5.1"

var (
	dotMapRegex = regexp.MustCompile(`\.map`)
	server_log  = log.NewLog("socket.io:server")
)

type ParentNspNameMatchFn *func(string, any, func(error, bool))

type Server struct {
	*StrictEventEmitter

	sockets NamespaceInterface
	engine  engine.Server

	_parser parser.Parser
	encoder parser.Encoder

	_nsps *sync.Map

	parentNsps      *sync.Map
	_adapter        Adapter
	_serveClient    bool
	opts            *ServerOptions
	eio             engine.Server
	_path           string
	clientPathRegex *regexp.Regexp

	_connectTimeout time.Duration
	httpServer      *types.HttpServer
}

func (s *Server) Sockets() NamespaceInterface {
	return s.sockets
}

func (s *Server) Engine() engine.Server {
	return s.engine
}

func (s *Server) Encoder() parser.Encoder {
	return s.encoder
}

func NewServer(srv any, opts *ServerOptions) *Server {
	s := &Server{}
	// @private
	s._nsps = &sync.Map{}
	s.parentNsps = &sync.Map{}

	if opts == nil {
		opts = DefaultServerOptions()
	}

	s.SetPath(opts.Path())
	s.SetConnectTimeout(opts.ConnectTimeout())
	s.SetServeClient(false != opts.ServeClient())
	if _parser := opts.Parser(); _parser != nil {
		s._parser = _parser
	} else {
		s._parser = parser.NewParser()
	}
	s.encoder = s._parser.Encoder()
	if _adapter := opts.Adapter(); _adapter != nil {
		s.SetAdapter(_adapter)
	} else {
		s.SetAdapter(&adapter{})
	}
	s.sockets = s.Of("/", nil)
	s.StrictEventEmitter = s.sockets.EventEmitter()

	s.opts = opts
	if srv != nil {
		s.Attach(srv, nil)
	}

	return s
}

// Sets/gets whether client code is being served.
func (s *Server) SetServeClient(v bool) *Server {
	s._serveClient = v
	return s
}
func (s *Server) ServeClient() bool {
	return s._serveClient
}

// Executes the middleware for an incoming namespace not already created on the server.
func (s *Server) _checkNamespace(name string, auth any, fn func(nsp *Namespace)) {
	s.parentNsps.Range(func(nextFn any, pnsp any) bool {
		status := false
		(*(nextFn.(ParentNspNameMatchFn)))(name, auth, func(err error, allow bool) {
			if err != nil || !allow {
				status = true
				return
			}
			if nsp, ok := s._nsps.Load(name); ok {
				// the namespace was created in the meantime
				server_log.Debug("dynamic namespace %s already exists", name)
				fn(nsp.(*Namespace))
				return
			}
			namespace := pnsp.(*ParentNamespace).CreateChild(name)
			server_log.Debug("dynamic namespace %s was created", name)
			s.sockets.EmitReserved("new_namespace", namespace)
			fn(namespace)
		})
		return status // whether to continue traversing.
	})
	fn(nil)
}

// Sets the client serving path.
func (s *Server) SetPath(v string) *Server {
	s._path = strings.TrimRight(v, "/")
	s.clientPathRegex = regexp.MustCompile(`^` + regexp.QuoteMeta(s._path) + `/socket\.io(\.msgpack|\.esm)?(\.min)?\.js(\.map)?(?:\?|$)`)
	return s
}
func (s *Server) Path() string {
	return s._path
}

// Set the delay after which a client without namespace is closed
func (s *Server) SetConnectTimeout(v time.Duration) *Server {
	s._connectTimeout = v
	return s
}
func (s *Server) ConnectTimeout() time.Duration {
	return s._connectTimeout
}

// Sets the adapter for rooms.
func (s *Server) SetAdapter(v Adapter) *Server {
	s._adapter = v
	s._nsps.Range(func(_, nsp any) bool {
		nsp.(*Namespace)._initAdapter()
		return true
	})
	return s
}
func (s *Server) Adapter() Adapter {
	return s._adapter
}

// Attaches socket.io to a server or port.
func (s *Server) Listen(srv any, opts *ServerOptions) *Server {
	return s.Attach(srv, opts)
}

// Attaches socket.io to a server or port.
func (s *Server) Attach(srv any, opts *ServerOptions) *Server {
	var server *types.HttpServer
	switch address := srv.(type) {
	case string:
		// handle a port as a string
		server_log.Debug("creating http server and binding to %s", address)
		server = types.CreateServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "404 page not found", http.StatusNotFound)
		}))
		server.Listen(address, nil)
	case *types.HttpServer:
		server = address
	default:
		panic(errors.New(fmt.Sprintf("You are trying to attach socket.io to an express request handler %T. Please pass a *types.HttpServer instance.", address)))
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

// Initialize engine
func (s *Server) initEngine(srv *types.HttpServer, opts any) {
	// initialize engine
	server_log.Debug("creating engine.io instance with opts %v", opts)
	s.eio = engine.Attach(srv, opts)

	// attach static file serving
	if s._serveClient {
		s.attachServe(srv)
	}

	// Export http server
	s.httpServer = srv

	// bind to engine events
	s.Bind(s.eio)
}

// Attaches the static file serving.
func (s *Server) attachServe(srv *types.HttpServer) {
	server_log.Debug("attaching client serving req handler")
	srv.HandleFunc(s._path+"/", func(w http.ResponseWriter, r *http.Request) {
		if s.clientPathRegex.MatchString(r.URL.Path) {
			s.serve(w, r)
		} else {
			srv.DefaultHandler.ServeHTTP(w, r)
		}
	})
}

// Handles a request serving of client source and map
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
	expectedEtag := `"` + clientVersion + `"`
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
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "application/javascript")
	}
	w.Header().Set("ETag", expectedEtag)
	s.sendFile(filename, w, r)
}

func (Server) sendFile(filename string, w http.ResponseWriter, r *http.Request) {
	_file, err := os.Executable()
	if err != nil {
		server_log.Debug("Failed to get run path: %v", err)
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}

	file, err := os.Open(filepath.Clean(path.Join(filepath.Dir(filepath.Dir(_file)), "client-dist", filename)))
	if err != nil {
		server_log.Debug("File read failed: %v", err)
		http.Error(w, "file not found", http.StatusNotFound)
		return
	}
	encoding := utils.Contains(r.Header.Get("Accept-Encoding"), []string{"gzip", "deflate", "br"})

	switch encoding {
	case "br":
		br := brotli.NewWriterLevel(w, 1)
		defer br.Close()
		w.Header().Set("Content-Encoding", "br")
		w.WriteHeader(http.StatusOK)
		io.Copy(br, file)
	case "gzip":
		gz, err := gzip.NewWriterLevel(w, 1)
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
		fl, err := flate.NewWriter(w, 1)
		if err != nil {
			server_log.Debug("Failed to compress data: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		defer fl.Close()
		w.Header().Set("Content-Encoding", "deflate")
		w.WriteHeader(http.StatusOK)
		io.Copy(fl, file)
	default:
		w.WriteHeader(http.StatusOK)
		io.Copy(w, file)
	}
}

// Binds socket.io to an engine.io instance.
func (s *Server) Bind(egs engine.Server) *Server {
	s.engine = egs
	s.engine.On("connection", s.onconnection)
	return s
}

// Called with each incoming transport connection.
func (s *Server) onconnection(conns ...any) {
	conn := conns[0].(engine.Socket)
	server_log.Debug("incoming connection with id %s", conn.Id())
	client := NewClient(s, conn)
	if conn.Protocol() == 3 {
		client.connect("/", nil)
	}
}

// Looks up a namespace.
func (s *Server) Of(name any, fn func(...any)) NamespaceInterface {
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

	var namespace *Namespace
	if nsp, ok := s._nsps.Load(n); ok {
		namespace = nsp.(*Namespace)
	} else {
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

// Closes server connection
func (s *Server) Close(fn func()) {
	s.sockets.Sockets().Range(func(_ any, socket any) bool {
		socket.(*Socket)._onclose("server shutting down")
		return true
	})

	if s.httpServer != nil {
		s.httpServer.Close(fn)
	} else {
		s.engine.Close()
		if fn != nil {
			fn()
		}
	}
}

// Sets up namespace middleware.
func (s *Server) Use(fn func(*Socket, func(*ExtendedError))) *Server {
	s.sockets.Use(fn)
	return s
}

// Targets a room when emitting.
func (s *Server) To(room ...Room) *BroadcastOperator {
	return s.sockets.To(room...)
}

// Targets a room when emitting.
func (s *Server) In(room ...Room) *BroadcastOperator {
	return s.sockets.In(room...)
}

// Excludes a room when emitting.
func (s *Server) Except(room ...Room) *BroadcastOperator {
	return s.sockets.Except(room...)
}

// Sends a `message` event to all clients.
func (s *Server) Send(args ...any) *Server {
	s.sockets.Emit("message", args...)
	return s
}

// Sends a `message` event to all clients.
func (s *Server) Write(args ...any) *Server {
	s.sockets.Emit("message", args...)
	return s
}

// Emit a packet to other Socket.IO servers
func (s *Server) ServerSideEmit(ev string, args ...any) error {
	return s.sockets.ServerSideEmit(ev, args...)
}

// Gets a list of socket ids.
func (s *Server) AllSockets() (*types.Set[SocketId], error) {
	return s.sockets.AllSockets()
}

// Sets the compress flag.
func (s *Server) Compress(compress bool) *BroadcastOperator {
	return s.sockets.Compress(compress)
}

// Sets a modifier for a subsequent event emission that the event data may be lost if the client is not ready to
// receive messages (because of network slowness or other issues, or because they’re connected through long polling
// and is in the middle of a request-response cycle).
func (s *Server) Volatile() *BroadcastOperator {
	return s.sockets.Volatile()
}

// Sets a modifier for a subsequent event emission that the event data will only be broadcast to the current node.
func (s *Server) Local() *BroadcastOperator {
	return s.sockets.Local()
}

// Adds a timeout in milliseconds for the next operation
//
// <pre><code>
//
//	io.Timeout(1000 * time.Millisecond).Emit("some-event", func(args ...any) {
//	  // ...
//	});
//
// </pre></code>
func (s *Server) Timeout(timeout time.Duration) *BroadcastOperator {
	return s.sockets.Timeout(timeout)
}

// Returns the matching socket instances
func (s *Server) FetchSockets() ([]*RemoteSocket, error) {
	return s.sockets.FetchSockets()
}

// Makes the matching socket instances join the specified rooms
func (s *Server) SocketsJoin(room ...Room) {
	s.sockets.SocketsJoin(room...)
}

// Makes the matching socket instances leave the specified rooms
func (s *Server) SocketsLeave(room ...Room) {
	s.sockets.SocketsLeave(room...)
}

// Makes the matching socket instances disconnect
func (s *Server) DisconnectSockets(status bool) {
	s.sockets.DisconnectSockets(status)
}
