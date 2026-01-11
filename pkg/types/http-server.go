package types

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/quic-go/webtransport-go"
)

var (
	server_log = log.NewLog("engine:server")
	http3_log  = slog.New(log.NewPrefixSimpleHandler(log.Output, "engine:server"))
)

type HttpServer struct {
	EventEmitter
	*ServeMux

	servers *Slice[any]
}

func NewWebServer(defaultHandler http.Handler) *HttpServer {
	s := &HttpServer{
		EventEmitter: NewEventEmitter(),
		ServeMux:     NewServeMux(defaultHandler),

		servers: NewSlice[any](),
	}
	return s
}

func (s *HttpServer) httpServer(addr string, handler http.Handler) *http.Server {
	server := &http.Server{Addr: addr, Handler: handler, ErrorLog: server_log.Logger}

	s.servers.Push(server)

	return server
}

func (s *HttpServer) h3Server(handler http.Handler) *http3.Server {
	// Start the servers
	server := &http3.Server{Handler: handler, Logger: http3_log}

	s.servers.Push(server)

	return server
}

func (s *HttpServer) webtransportServer(addr string, handler http.Handler) *webtransport.Server {
	// Start the servers
	server := &webtransport.Server{
		H3: http3.Server{Addr: addr, Handler: handler, Logger: http3_log},
		CheckOrigin: func(_ *http.Request) bool {
			return true
		},
	}

	s.servers.Push(server)

	return server
}

func (s *HttpServer) Close(fn func(error)) (err error) {
	s.Emit("close")

	var closingErr, serverErr error
	s.servers.Range(func(server any, _ int) bool {
		switch srv := server.(type) {
		case *http.Server:
			serverErr = srv.Shutdown(context.Background())
		case *http3.Server:
			serverErr = srv.Close()
		case *webtransport.Server:
			serverErr = srv.Close()
		default:
			serverErr = errors.New("unknown server type")
		}
		if serverErr != nil && closingErr == nil {
			closingErr = serverErr
		}
		return true
	})

	if closingErr != nil {
		err = fmt.Errorf("error occurred while closing servers: %v", closingErr)
	}

	if fn != nil {
		defer fn(err)
	}

	return err
}

func (s *HttpServer) Listen(addr string, fn Callable) *http.Server {
	server := s.httpServer(addr, s)
	// Idempotent repeated calls
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
	server := s.httpServer(addr, s)
	// Idempotent repeated calls
	go func() {
		if err := server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
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
	var err error
	// Load certs
	certs := make([]tls.Certificate, 1)
	certs[0], err = tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		panic(err)
	}
	// We currently only use the cert-related stuff from tls.Config,
	// so we don't need to make a full copy.
	config := &tls.Config{
		Certificates: certs,
	}

	if addr == "" {
		addr = ":https"
	}

	// Open the listeners
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		panic(err)
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		panic(err)
	}

	server := s.h3Server(s)
	server.TLSConfig = config
	server.QUICConfig = quicConfig

	// Idempotent repeated calls
	go func() {
		defer udpConn.Close()

		hErr := make(chan error)
		qErr := make(chan error)
		// Idempotent repeated calls
		go func() {
			hErr <- s.httpServer(addr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				server.SetQUICHeaders(w.Header())
				s.ServeHTTP(w, r)
			})).ListenAndServeTLS(certFile, keyFile)
		}()
		// Idempotent repeated calls
		go func() {
			qErr <- server.Serve(udpConn)
		}()

		select {
		case err := <-hErr:
			server.Close()
			if err != http.ErrServerClosed {
				panic(err)
			}
		case err := <-qErr:
			// Cannot close the HTTP server or wait for requests to complete properly :/
			if err != http.ErrServerClosed {
				panic(err)
			}
		}
	}()

	if fn != nil {
		defer fn()
	}
	s.Emit("listening")

	return server
}

func (s *HttpServer) ListenWebTransportTLS(addr string, certFile string, keyFile string, quicConfig *quic.Config, fn Callable) *webtransport.Server {
	server := s.webtransportServer(addr, s)
	server.H3.QUICConfig = quicConfig

	// Idempotent repeated calls
	go func() {
		if err := server.ListenAndServeTLS(certFile, keyFile); err != nil && err != http.ErrServerClosed {
			panic(err)
		}
	}()

	if fn != nil {
		defer fn()
	}
	s.Emit("listening")

	return server
}
