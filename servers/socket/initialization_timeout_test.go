package socket

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
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type initializationCountingParser struct {
	parser.Parser
	created atomic.Int32
	decode  func() parser.Decoder
}

func (p *initializationCountingParser) NewDecoder() parser.Decoder {
	p.created.Add(1)
	if p.decode != nil {
		return p.decode()
	}
	return p.Parser.NewDecoder()
}

type initializationCountingDecoder struct {
	parser.Decoder
	destroyed atomic.Int32
}

func (d *initializationCountingDecoder) Destroy() {
	d.destroyed.Add(1)
	d.Decoder.Destroy()
}

func initializationSocketWait(t *testing.T, done <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for " + description)
	}
}

func initializationSocketEngine() engine.Server {
	opts := config.DefaultServerOptions()
	opts.SetUpgradeTimeout(50 * time.Millisecond)
	opts.SetPingInterval(time.Second)
	return engine.NewServer(opts)
}

func initializationSocketPeer(t *testing.T, eio engine.Server, server *Server, release func()) (*websocket.Conn, <-chan struct{}) {
	t.Helper()
	handled := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(handled)
		eio.ServeHTTP(w, r)
	}))
	var peer *websocket.Conn
	t.Cleanup(func() {
		release()
		if peer != nil {
			_ = peer.Close()
		}
		select {
		case <-handled:
		case <-time.After(2 * time.Second):
			t.Error("initialization did not return after releasing the test callback")
		}
		server.Close(nil)
		eio.Close()
		httpServer.Close()
	})
	var err error
	peer, _, err = websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/socket.io/?EIO=4&transport=websocket", nil)
	if err != nil {
		t.Fatal(err)
	}
	return peer, handled
}

func initializationSocketClosed(t *testing.T, peer *websocket.Conn, conn engine.Socket) {
	t.Helper()
	if state := conn.ReadyState(); state != "closed" {
		t.Fatalf("Engine socket state = %q, want closed", state)
	}
	_ = peer.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, data, err := peer.ReadMessage(); err != nil || !strings.HasPrefix(string(data), "0{") {
		t.Fatalf("expected OPEN before timeout, received %q, error = %v", data, err)
	}
	_, data, err := peer.ReadMessage()
	if err == nil {
		t.Fatalf("expected initialization timeout to close the network connection, received %q", data)
	}
	var timeout net.Error
	if errors.As(err, &timeout) && timeout.Timeout() {
		t.Fatalf("network connection remained open until the test read deadline: %v", err)
	}
}

func initializationSocketNoListeners(t *testing.T, conn engine.Socket) {
	t.Helper()
	for _, event := range []types.EventName{"data", "error", "close"} {
		if count := conn.ListenerCount(event); count != 0 {
			t.Errorf("closed Engine socket retained %d %q listeners", count, event)
		}
	}
}

func TestInitializationTimeoutBeforeSocketIOBinding(t *testing.T) {
	eio := initializationSocketEngine()
	resume, entered, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	var conn engine.Socket
	_ = eio.On("connection", func(args ...any) {
		conn = args[0].(engine.Socket)
		_ = conn.Once("close", func(...any) { close(closed) })
		close(entered)
		<-resume
	})
	p := &initializationCountingParser{Parser: parser.NewParser()}
	opts := DefaultServerOptions()
	opts.SetParser(p)
	server := NewServer(nil, opts)
	server.Bind(eio)
	peer, handled := initializationSocketPeer(t, eio, server, release)
	initializationSocketWait(t, entered, "callback before Socket.IO binding")
	initializationSocketWait(t, closed, "Engine timeout before Socket.IO binding")
	initializationSocketClosed(t, peer, conn)
	release()
	initializationSocketWait(t, handled, "late Socket.IO binding")
	if count := p.created.Load(); count != 0 {
		t.Fatalf("closed Engine socket created %d Socket.IO decoders", count)
	}
	initializationSocketNoListeners(t, conn)
	if count := eio.ClientsCount(); count != 0 {
		t.Fatalf("closed Engine socket remained registered: %d", count)
	}
}

func TestInitializationTimeoutDuringSocketIOClientConstruction(t *testing.T) {
	eio := initializationSocketEngine()
	resume, entered, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	decoder := &initializationCountingDecoder{Decoder: parser.NewParser().NewDecoder()}
	p := &initializationCountingParser{Parser: parser.NewParser()}
	p.decode = func() parser.Decoder {
		close(entered)
		<-resume
		return decoder
	}
	opts := DefaultServerOptions()
	opts.SetParser(p)
	server := NewServer(nil, opts)
	var conn engine.Socket
	var client *Client
	_ = eio.On("connection", func(args ...any) {
		conn = args[0].(engine.Socket)
		_ = conn.Once("close", func(...any) { close(closed) })
		// Use the same constructor as Bind, retaining its result so the test
		// can verify that the late Client also releases its connect timer.
		client = NewClient(server, conn)
	})
	peer, handled := initializationSocketPeer(t, eio, server, release)
	initializationSocketWait(t, entered, "custom decoder construction")
	initializationSocketWait(t, closed, "Engine timeout during decoder construction")
	initializationSocketClosed(t, peer, conn)
	release()
	initializationSocketWait(t, handled, "late Client construction")
	if client.connectTimeout.Load() != nil {
		t.Error("late Client retained its connect timer")
	}
	initializationSocketNoListeners(t, conn)
	if count := decoder.ListenerCount("decoded"); count != 0 {
		t.Errorf("destroyed decoder retained %d decoded listeners", count)
	}
	if count := p.created.Load(); count != 1 {
		t.Errorf("created %d decoders, want 1", count)
	}
	if count := decoder.destroyed.Load(); count != 1 {
		t.Errorf("destroyed decoder %d times, want 1", count)
	}
	// A later close notification must not repeat teardown.
	client.onclose("transport close")
	if count := decoder.destroyed.Load(); count != 1 {
		t.Errorf("repeated close destroyed decoder %d times, want 1", count)
	}
	if count := server.Sockets().Sockets().Len(); count != 0 {
		t.Errorf("closed Client joined %d namespace sockets", count)
	}
}
