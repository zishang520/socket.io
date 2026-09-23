package types

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
	"github.com/zishang520/socket.io/v3/pkg/log"
)

var (
	serverLog = log.NewLog("engine:server")
	http3Log  = slog.New(log.NewPrefixSimpleHandler(log.Output, "engine:server"))
)

// HttpServer manages HTTP listeners. It must not be copied after first use.
type HttpServer struct {
	EventEmitter
	*ServeMux

	mu sync.Mutex
	// Registered actions are append-only, so Close can use a fixed-length
	// snapshot without holding the registry lock during shutdown.
	shutdowns []func() error
}

func NewWebServer(defaultHandler http.Handler) *HttpServer {
	return &HttpServer{
		EventEmitter: NewEventEmitter(),
		ServeMux:     NewServeMux(defaultHandler),
	}
}

func (s *HttpServer) addShutdowns(shutdowns ...func() error) {
	s.mu.Lock()
	s.shutdowns = append(s.shutdowns, shutdowns...)
	s.mu.Unlock()
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{Addr: addr, Handler: handler, ErrorLog: serverLog.Logger}
}

func (s *HttpServer) Close(fn func(error)) (err error) {
	s.Emit("close")

	s.mu.Lock()
	shutdowns := s.shutdowns
	s.mu.Unlock()
	var closingErr error
	for _, shutdown := range shutdowns {
		if serverErr := shutdown(); serverErr != nil && closingErr == nil {
			closingErr = serverErr
		}
	}

	if closingErr != nil {
		err = fmt.Errorf("error occurred while closing servers: %v", closingErr)
	}

	if fn != nil {
		defer fn(err)
	}

	return err
}

func shutdownHTTPServer(server *http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return server.Shutdown(ctx)
}

func serverTLSConfig(certFile, keyFile string) *tls.Config {
	certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		panic(err)
	}
	return &tls.Config{Certificates: []tls.Certificate{certificate}}
}

func listenUDP(addr string) *net.UDPConn {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panic(err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		panic(err)
	}
	return conn
}

// WebTransport's Serve registers its lifetime before QUIC first accesses the
// socket. Wait for that access before publishing readiness: an earlier Close
// would race its initial WaitGroup.Add. Embedding UDPConn retains QUIC's UDP
// optimizations, including ReadMsgUDP and SyscallConn.
type webTransportListenConn struct {
	*net.UDPConn
	started chan struct{}
	once    sync.Once
}

func (c *webTransportListenConn) LocalAddr() net.Addr {
	c.once.Do(func() { close(c.started) })
	return c.UDPConn.LocalAddr()
}

func (s *HttpServer) Listen(addr string, fn Callable) *http.Server {
	// Bind before notifying listeners or callers that the server is ready.
	listenAddr := addr
	if listenAddr == "" {
		listenAddr = ":http"
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		panic(err)
	}
	server := newHTTPServer(addr, s)
	s.addShutdowns(func() error { return shutdownHTTPServer(server) })
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	if fn != nil {
		defer fn()
	}
	s.Emit("listening")

	return server
}

func (s *HttpServer) ListenTLS(addr string, certFile string, keyFile string, fn Callable) *http.Server {
	config := serverTLSConfig(certFile, keyFile)
	listenAddr := addr
	if listenAddr == "" {
		listenAddr = ":https"
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		panic(err)
	}
	server := newHTTPServer(addr, s)
	server.TLSConfig = config
	s.addShutdowns(func() error { return shutdownHTTPServer(server) })
	go func() {
		if err := server.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	if fn != nil {
		defer fn()
	}
	s.Emit("listening")

	return server
}

func (s *HttpServer) ListenHTTP3TLS(addr string, certFile string, keyFile string, quicConfig *quic.Config, fn Callable) *http3.Server {
	config := serverTLSConfig(certFile, keyFile)
	if addr == "" {
		addr = ":https"
	}
	udpConn := listenUDP(addr)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		_ = udpConn.Close()
		panic(err)
	}

	server := &http3.Server{
		Handler:    s,
		Logger:     http3Log,
		TLSConfig:  config,
		QUICConfig: quicConfig,
	}
	httpsServer := newHTTPServer(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = server.SetQUICHeaders(w.Header())
		s.ServeHTTP(w, r)
	}))
	httpsServer.TLSConfig = config.Clone()
	s.addShutdowns(server.Close, func() error { return shutdownHTTPServer(httpsServer) })

	// Each serving goroutine owns its listener and stops the paired server.
	// There are no result senders left waiting after one side shuts down.
	go func() {
		defer func() { _ = server.Close() }()
		if err := httpsServer.ServeTLS(listener, "", ""); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()
	go func() {
		defer func() { _ = udpConn.Close() }()
		defer func() { _ = shutdownHTTPServer(httpsServer) }()
		if err := server.Serve(udpConn); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	if fn != nil {
		defer fn()
	}
	s.Emit("listening")

	return server
}

func (s *HttpServer) ListenWebTransportTLS(addr string, certFile string, keyFile string, quicConfig *quic.Config, fn Callable) *webtransport.Server {
	config := http3.ConfigureTLSConfig(serverTLSConfig(certFile, keyFile))
	listenAddr := addr
	if listenAddr == "" {
		listenAddr = ":https"
	}
	conn := &webTransportListenConn{UDPConn: listenUDP(listenAddr), started: make(chan struct{})}
	server := &webtransport.Server{
		H3: &http3.Server{
			Addr:       addr,
			Handler:    s,
			Logger:     http3Log,
			TLSConfig:  config,
			QUICConfig: quicConfig,
		},
	}

	startErr := make(chan error)
	go func() {
		// Serve owns WebTransport's HTTP/3 settings and ConnContext initialization.
		err := server.Serve(conn)
		_ = conn.Close()
		select {
		case startErr <- err:
			// Serve failed before it could publish startup. Return the error to
			// the caller without leaving a goroutine waiting for a receiver.
		case <-conn.started:
			if err != nil && err != http.ErrServerClosed && !errors.Is(err, context.Canceled) {
				panic(err)
			}
		}
	}()
	select {
	case <-conn.started:
	case err := <-startErr:
		panic(err)
	}
	s.addShutdowns(server.Close)

	if fn != nil {
		defer fn()
	}
	s.Emit("listening")

	return server
}
