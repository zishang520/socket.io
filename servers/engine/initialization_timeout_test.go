package engine

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type initializationTimeoutServer struct {
	Server
	create func(string, *types.HttpContext) (transports.Transport, error)
}

func (s *initializationTimeoutServer) CreateTransport(name string, ctx *types.HttpContext) (transports.Transport, error) {
	return s.create(name, ctx)
}

func initializationTimeoutOptions() *config.ServerOptions {
	opts := config.DefaultServerOptions()
	opts.SetUpgradeTimeout(50 * time.Millisecond)
	opts.SetPingInterval(20 * time.Millisecond)
	opts.SetPingTimeout(time.Minute)
	return opts
}

func initializationTimeoutClient(t *testing.T, s Server, release func()) (*websocket.Conn, <-chan struct{}) {
	t.Helper()
	handled := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handled)
		s.ServeHTTP(w, r)
	}))
	var client *websocket.Conn
	t.Cleanup(func() {
		release()
		if client != nil {
			_ = client.Close()
		}
		select {
		case <-handled:
		case <-time.After(2 * time.Second):
			t.Error("initialization did not return after releasing the test callback")
		}
		s.Close()
		httpServer.Close()
	})
	var err error
	client, _, err = websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/engine.io/?EIO=4&transport=websocket", nil)
	if err != nil {
		t.Fatal(err)
	}
	return client, handled
}

func initializationTimeoutDisconnected(t *testing.T, client *websocket.Conn) {
	t.Helper()
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := client.ReadMessage()
	if err == nil {
		t.Fatalf("expected the connection to close, received %q", data)
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		t.Fatalf("connection remained open until the test read deadline: %v", err)
	}
}

func initializationTimeoutEmpty(t *testing.T, s Server) {
	t.Helper()
	if count, size := s.ClientsCount(), s.Clients().Len(); count != 0 || size != 0 {
		t.Fatalf("closed initialization retained clients: count=%d, map size=%d", count, size)
	}
}

func TestInitializationTimeoutDuringTransportConstruction(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		peerCloses    bool
		lateConstruct bool
	}{
		{name: "idle_peer"},
		{name: "disconnected_peer", peerCloses: true},
		{name: "constructor_after_timeout", lateConstruct: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			entered, resume := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			rawClosed, transportClosed := make(chan struct{}), make(chan struct{})
			markRawClosed := sync.OnceFunc(func() { close(rawClosed) })
			markTransportClosed := sync.OnceFunc(func() { close(transportClosed) })
			made := make(chan transports.Transport, 1)
			s := &initializationTimeoutServer{Server: MakeServer()}
			s.Prototype(s)
			s.Construct(initializationTimeoutOptions())
			s.create = func(name string, ctx *types.HttpContext) (transports.Transport, error) {
				_ = ctx.Websocket.Once("close", func(...any) { markRawClosed() })
				if scenario.lateConstruct {
					close(entered)
					<-resume
				}
				transport, err := s.Server.CreateTransport(name, ctx)
				if err != nil {
					return nil, err
				}
				_ = transport.Once("close", func(...any) { markTransportClosed() })
				// Cancellation can win before a late constructor returns.
				if transport.ReadyState() == "closed" {
					markTransportClosed()
				}
				made <- transport
				if !scenario.lateConstruct {
					close(entered)
					<-resume
				}
				return transport, nil
			}
			var connections atomic.Int32
			_ = s.On("connection", func(...any) { connections.Add(1) })
			client, handled := initializationTimeoutClient(t, s, release)
			readinessWait(t, entered, "blocked transport constructor")
			if scenario.peerCloses {
				_ = client.Close()
			}
			readinessWait(t, rawClosed, "network close while the constructor remains blocked")
			if !scenario.peerCloses {
				initializationTimeoutDisconnected(t, client)
			}
			if !scenario.lateConstruct {
				readinessWait(t, transportClosed, "transport cancellation before constructor return")
			}
			initializationTimeoutEmpty(t, s)
			release()
			readinessWait(t, handled, "late constructor return")
			readinessWait(t, transportClosed, "late transport cancellation")
			transport := <-made
			if state := transport.ReadyState(); state != "closed" {
				t.Fatalf("late transport state = %q, want closed", state)
			}
			initializationTimeoutEmpty(t, s)
			if count := connections.Load(); count != 0 {
				t.Fatalf("timed-out constructor published %d connections", count)
			}
		})
	}
}

func TestInitializationTimeoutDuringOpenFlush(t *testing.T) {
	s := NewServer(initializationTimeoutOptions())
	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	initializing := make(chan Socket, 1)
	closed := make(chan struct{})
	markClosed := sync.OnceFunc(func() { close(closed) })
	lateEvent := make(chan string, 2)
	var closeCount, connections atomic.Int32
	_ = s.On("connection", func(...any) { connections.Add(1) })
	_ = s.On("flush", func(args ...any) {
		packets := args[1].([]*packet.Packet)
		if len(packets) == 0 || packets[0].Type != packet.OPEN {
			return
		}
		conn := args[0].(Socket)
		_ = conn.On("close", func(...any) {
			closeCount.Add(1)
			markClosed()
		})
		_ = conn.On("open", func(...any) { lateEvent <- "open" })
		_ = conn.On("packetCreate", func(args ...any) {
			if args[0].(*packet.Packet).Type == packet.PING {
				lateEvent <- "ping"
			}
		})
		initializing <- conn
		<-resume
	})
	client, handled := initializationTimeoutClient(t, s, release)
	var conn Socket
	select {
	case conn = <-initializing:
	case <-time.After(2 * time.Second):
		t.Fatal("OPEN did not reach the flush callback")
	}
	readinessWait(t, closed, "socket cancellation during OPEN flush")
	initializationTimeoutDisconnected(t, client)
	initializationTimeoutEmpty(t, s)
	release()
	readinessWait(t, handled, "OPEN flush callback return")
	if conn.Request().TakeTransportReady() != nil {
		t.Error("failed initialization retained its readiness callback")
	}
	// Cover multiple configured heartbeat intervals after resuming onOpen.
	select {
	case event := <-lateEvent:
		t.Fatalf("closed initialization emitted %q after resuming OPEN", event)
	case <-time.After(3 * s.Opts().PingInterval()):
	}
	if state := conn.ReadyState(); state != "closed" {
		t.Fatalf("resumed socket state = %q, want closed", state)
	}
	initializationTimeoutEmpty(t, s)
	if count := connections.Load(); count != 0 {
		t.Fatalf("timed-out OPEN initialization published %d connections", count)
	}
	if count := closeCount.Load(); count != 1 {
		t.Fatalf("closed initialization emitted %d close events, want 1", count)
	}
}

func TestInitializationTimeoutDuringConnectionCallback(t *testing.T) {
	s := NewServer(initializationTimeoutOptions())
	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	initializing := make(chan Socket, 1)
	closed := make(chan struct{})
	markClosed := sync.OnceFunc(func() { close(closed) })
	var closeCount atomic.Int32
	_ = s.On("connection", func(args ...any) {
		conn := args[0].(Socket)
		_ = conn.On("close", func(...any) {
			closeCount.Add(1)
			markClosed()
		})
		initializing <- conn
		<-resume
	})
	client, handled := initializationTimeoutClient(t, s, release)
	var conn Socket
	select {
	case conn = <-initializing:
	case <-time.After(2 * time.Second):
		t.Fatal("connection callback was not invoked")
	}
	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, data, err := client.ReadMessage(); err != nil || !strings.HasPrefix(string(data), "0{") {
		t.Fatalf("expected OPEN before connection callback timeout, received %q, error = %v", data, err)
	}
	readinessWait(t, closed, "socket cancellation while connection callback remains blocked")
	// Heartbeats start only after the connection callback returns; this ordering
	// is covered by TestHeartbeatWaitsForStreamInitialization.
	initializationTimeoutDisconnected(t, client)
	initializationTimeoutEmpty(t, s)
	release()
	readinessWait(t, handled, "connection callback return")
	if conn.Request().TakeTransportReady() != nil {
		t.Error("failed initialization retained its readiness callback")
	}
	if state := conn.ReadyState(); state != "closed" {
		t.Fatalf("resumed connection state = %q, want closed", state)
	}
	initializationTimeoutEmpty(t, s)
	if count := closeCount.Load(); count != 1 {
		t.Fatalf("connection emitted %d close events, want 1", count)
	}
}
