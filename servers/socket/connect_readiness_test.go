package socket

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/servers/engine/v3"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Keep each initialization window open while a real client sends its first
// CONNECT. Observing packet/data here would mean it has no protocol listener yet.
type connectInitializationBarrier struct {
	entered    chan struct{}
	release    chan struct{}
	early      chan struct{}
	resumeOnce sync.Once
	held       atomic.Bool
}

func newConnectInitializationBarrier() *connectInitializationBarrier {
	b := &connectInitializationBarrier{
		entered: make(chan struct{}),
		release: make(chan struct{}),
		early:   make(chan struct{}, 1),
	}
	b.held.Store(true)
	return b
}

func (b *connectInitializationBarrier) observe(...any) {
	if b.held.Load() {
		select {
		case b.early <- struct{}{}:
		default:
		}
	}
}

func (b *connectInitializationBarrier) pause() {
	close(b.entered)
	<-b.release
}

func (b *connectInitializationBarrier) resume() {
	b.resumeOnce.Do(func() {
		b.held.Store(false)
		close(b.release)
	})
}

type connectReadinessEngine struct {
	engine.Server
	barrier *connectInitializationBarrier
}

func (s *connectReadinessEngine) CreateTransport(name string, ctx *types.HttpContext) (transports.Transport, error) {
	transport, err := s.Server.CreateTransport(name, ctx)
	if err == nil {
		_ = transport.On("packet", s.barrier.observe)
		s.barrier.pause()
	}
	return transport, err
}

func TestFirstConnectWaitsForListeners(t *testing.T) {
	for _, window := range []struct {
		name             string
		beforeEngine     bool
		connectAfterOpen bool
	}{
		{name: "before_engine_listeners", beforeEngine: true},
		{name: "before_socketio_listeners_before_open"},
		{name: "before_socketio_listeners_after_open", connectAfterOpen: true},
	} {
		for _, reject := range []bool{false, true} {
			result := "accepted"
			if reject {
				result = "rejected"
			}
			t.Run(window.name+"/"+result, func(t *testing.T) {
				barrier := newConnectInitializationBarrier()
				var eio engine.Server
				if window.beforeEngine {
					wrapped := &connectReadinessEngine{Server: engine.MakeServer(), barrier: barrier}
					wrapped.Prototype(wrapped)
					wrapped.Construct(nil)
					eio = wrapped
				} else {
					eio = engine.NewServer(nil)
					// This listener precedes Bind's Socket.IO client initialization.
					_ = eio.On("connection", func(args ...any) {
						conn := args[0].(engine.Socket)
						_ = conn.On("data", barrier.observe)
						barrier.pause()
					})
				}

				server := NewServer(nil, nil)
				var authCalls, connections atomic.Int32
				server.Use(func(socket *Socket, next func(*ExtendedError)) {
					authCalls.Add(1)
					if got := socket.Handshake().Auth["token"]; got != "first" {
						t.Errorf("first CONNECT auth token = %v, want first", got)
					}
					if reject {
						next(NewExtendedError("denied", nil))
						return
					}
					next(nil)
				})
				_ = server.On("connection", func(args ...any) {
					connections.Add(1)
					socket := args[0].(*Socket)
					_ = socket.On("ordered", func(values ...any) {
						_ = socket.Emit("ordered", values...)
					})
					_ = socket.Emit("ready")
				})
				server.Bind(eio)
				httpServer := httptest.NewServer(eio)
				var client *websocket.Conn
				t.Cleanup(func() {
					barrier.resume()
					if client != nil {
						_ = client.Close()
					}
					server.Close(nil)
					httpServer.Close()
				})
				var err error
				client, _, err = websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/socket.io/?EIO=4&transport=websocket", nil)
				if err != nil {
					t.Fatal(err)
				}
				_ = client.SetWriteDeadline(time.Now().Add(5 * time.Second))
				select {
				case <-barrier.entered:
				case <-time.After(5 * time.Second):
					t.Fatal("connection did not reach the initialization barrier")
				}

				read := func() string {
					t.Helper()
					_ = client.SetReadDeadline(time.Now().Add(5 * time.Second))
					kind, payload, err := client.ReadMessage()
					if err != nil {
						t.Fatal(err)
					}
					if kind != websocket.TextMessage {
						t.Fatalf("received frame type %d, want text", kind)
					}
					return string(payload)
				}
				readOpen := func() {
					t.Helper()
					if message := read(); !strings.HasPrefix(message, "0{") {
						t.Fatalf("expected Engine.IO OPEN, got %q", message)
					}
				}
				if window.connectAfterOpen {
					readOpen()
				}
				if err := client.WriteMessage(websocket.TextMessage, []byte(`40{"token":"first"}`)); err != nil {
					t.Fatal(err)
				}
				// The barrier makes the unsafe initialization state persistent;
				// this bounded observation checks that no packet escapes it.
				select {
				case <-barrier.early:
					t.Fatal("first CONNECT was dispatched before its listeners were installed")
				case <-time.After(100 * time.Millisecond):
				}
				if got := authCalls.Load(); got != 0 {
					t.Fatalf("authentication ran during initialization %d times", got)
				}
				barrier.resume()
				if !window.connectAfterOpen {
					readOpen()
				}

				response := read()
				if reject {
					if !strings.HasPrefix(response, "44") {
						t.Fatalf("first CONNECT response = %q, want CONNECT_ERROR", response)
					}
					var payload struct{ Message string }
					if err := json.Unmarshal([]byte(response[2:]), &payload); err != nil || payload.Message != "denied" {
						t.Fatalf("CONNECT_ERROR payload = %q, error = %v", response[2:], err)
					}
					if got := connections.Load(); got != 0 {
						t.Fatalf("rejected CONNECT published %d connections", got)
					}
				} else {
					if !strings.HasPrefix(response, "40{") {
						t.Fatalf("first CONNECT response = %q, want CONNECT", response)
					}
					if message := read(); message != `42["ready"]` {
						t.Fatalf("expected application readiness event, got %q", message)
					}
					// Send consecutive packets without waiting for each echo.
					// All must survive and retain their original order.
					for i := range 10 {
						if err := client.WriteMessage(websocket.TextMessage, fmt.Appendf(nil, `42["ordered",%d]`, i)); err != nil {
							t.Fatal(err)
						}
					}
					for i := range 10 {
						if got, want := read(), fmt.Sprintf(`42["ordered",%d]`, i); got != want {
							t.Fatalf("event %d = %q, want %q", i, got, want)
						}
					}
					if got := connections.Load(); got != 1 {
						t.Fatalf("accepted CONNECT published %d connections, want 1", got)
					}
				}
				if got := authCalls.Load(); got != 1 {
					t.Fatalf("first CONNECT authenticated %d times, want 1", got)
				}
			})
		}
	}
}
