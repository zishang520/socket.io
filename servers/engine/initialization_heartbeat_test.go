package engine

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestHeartbeatWaitsForStreamInitialization(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		protocol int
		wt       bool
	}{
		{name: "websocket_eio4", protocol: 4},
		{name: "websocket_eio3", protocol: 3},
		{name: "webtransport", protocol: 4, wt: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			opts := config.DefaultServerOptions()
			opts.SetPingInterval(20 * time.Millisecond)
			opts.SetPingTimeout(80 * time.Millisecond)
			opts.SetUpgradeTimeout(2 * time.Second)
			opts.SetAllowEIO3(scenario.protocol == 3)
			if scenario.wt {
				opts.SetTransports(types.NewSet[transports.TransportCtor](WebTransport))
			}
			server := NewServer(opts)
			t.Cleanup(func() { server.Close() })

			resume := make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			defer release()
			initializing := make(chan Socket, 1)
			heartbeats := make(chan struct{}, 32)
			closed := make(chan string, 1)
			_ = server.On("connection", func(args ...any) {
				conn := args[0].(Socket)
				_ = conn.On("heartbeat", func(...any) {
					select {
					case heartbeats <- struct{}{}:
					default:
					}
				})
				_ = conn.Once("close", func(args ...any) { closed <- args[0].(string) })
				initializing <- conn
				<-resume
			})

			var peer readinessConnection
			var closePeer func()
			if scenario.wt {
				conn := readinessWebTransport(t, server)
				readinessWrite(t, conn, "0")
				peer = conn
				closePeer = func() { _ = conn.CloseWithError(0, "") }
			} else {
				httpServer := httptest.NewServer(server)
				t.Cleanup(httpServer.Close)
				url := fmt.Sprintf("ws%s/engine.io/?EIO=%d&transport=websocket", strings.TrimPrefix(httpServer.URL, "http"), scenario.protocol)
				conn, _, err := websocket.DefaultDialer.Dial(url, nil)
				if err != nil {
					t.Fatal(err)
				}
				peer = conn
				closePeer = func() { _ = conn.Close() }
			}
			_ = peer.SetWriteDeadline(time.Now().Add(3 * time.Second))
			_ = peer.SetReadDeadline(time.Now().Add(3 * time.Second))
			var respond, sawPing atomic.Bool
			respond.Store(true)
			stop := make(chan struct{})
			readDone, writeDone := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(readDone)
				for {
					_, data, err := peer.ReadMessage()
					if err != nil {
						return
					}
					if scenario.protocol == 4 && string(data) == "2" {
						sawPing.Store(true)
						if respond.Load() {
							if err := peer.WriteMessage(websocket.TextMessage, []byte("3")); err != nil {
								return
							}
						}
					}
				}
			}()
			if scenario.protocol == 3 {
				go func() {
					defer close(writeDone)
					ticker := time.NewTicker(opts.PingInterval())
					defer ticker.Stop()
					for {
						select {
						case <-stop:
							return
						case <-ticker.C:
							if respond.Load() {
								if err := peer.WriteMessage(websocket.TextMessage, []byte("2")); err != nil {
									return
								}
							}
						}
					}
				}()
			} else {
				close(writeDone)
			}
			t.Cleanup(func() {
				release()
				close(stop)
				closePeer()
				readinessWait(t, readDone, "heartbeat peer reader shutdown")
				readinessWait(t, writeDone, "heartbeat peer writer shutdown")
			})

			var conn Socket
			select {
			case conn = <-initializing:
			case <-time.After(time.Second):
				t.Fatal("connection did not enter initialization")
			}
			// Exceed both heartbeat intervals while staying inside the startup
			// budget. The client keeps answering PING (or sending EIO3 PING).
			select {
			case reason := <-closed:
				t.Fatalf("responsive peer closed during initialization: %s", reason)
			case <-time.After(250 * time.Millisecond):
			}
			if conn.ReadyState() != "open" || sawPing.Load() {
				t.Fatal("heartbeat started before stream reading was allowed")
			}
			release()
			for range 2 {
				readinessWait(t, heartbeats, "heartbeat after initialization")
			}
			// Startup must delay heartbeat, not disable it permanently.
			respond.Store(false)
			select {
			case reason := <-closed:
				if reason != "ping timeout" {
					t.Fatalf("close reason = %q, want ping timeout", reason)
				}
			case <-time.After(time.Second):
				t.Fatal("silent peer remained open after heartbeat started")
			}
		})
	}
}

func TestHeartbeatStartsAfterUpgradeBeforeInitializationReady(t *testing.T) {
	for _, protocol := range []int{3, 4} {
		t.Run(fmt.Sprintf("EIO%d", protocol), func(t *testing.T) {
			ready, resume := make(chan struct{}), make(chan struct{})
			handled := make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			defer release()
			opts := config.DefaultServerOptions()
			opts.SetAllowEIO3(protocol == 3)
			opts.SetPingInterval(20 * time.Millisecond)
			opts.SetPingTimeout(80 * time.Millisecond)
			opts.SetUpgradeTimeout(time.Hour)
			opts.SetTransports(types.NewSet[transports.TransportCtor](WebSocket))
			server := NewServer(opts)
			t.Cleanup(func() { server.Close() })
			connected := make(chan Socket, 1)
			upgraded := make(chan struct{})
			heartbeat := make(chan struct{}, 1)
			closed := make(chan string, 1)
			_ = server.On("connection", func(args ...any) {
				conn := args[0].(Socket)
				ctx := conn.Request()
				startHeartbeat := ctx.TakeTransportReady()
				if startHeartbeat == nil {
					t.Error("stream initialization did not register heartbeat startup")
					return
				}
				// Pause after reading is released, allowing a real upgrade to
				// replace the transport before the Socket starts its heartbeat.
				ctx.SetTransportReady(func() {
					close(ready)
					<-resume
					startHeartbeat()
				})
				_ = conn.Once("upgrade", func(...any) { close(upgraded) })
				_ = conn.Once("heartbeat", func(...any) { heartbeat <- struct{}{} })
				_ = conn.Once("close", func(args ...any) { closed <- args[0].(string) })
				connected <- conn
			})
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Query().Get("sid") == "" {
					defer close(handled)
				}
				server.ServeHTTP(w, r)
			}))
			t.Cleanup(httpServer.Close)
			url := fmt.Sprintf("ws%s/engine.io/?EIO=%d&transport=websocket", strings.TrimPrefix(httpServer.URL, "http"), protocol)
			initial, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = initial.Close() })
			_ = initial.SetReadDeadline(time.Now().Add(time.Second))
			if _, data, readErr := initial.ReadMessage(); readErr != nil || !strings.HasPrefix(string(data), `0{"`) {
				t.Fatalf("initial OPEN = %q, error = %v", data, readErr)
			}
			readinessWait(t, ready, "initial transport readiness callback")
			conn := <-connected
			candidate, _, err := websocket.DefaultDialer.Dial(url+"&sid="+conn.Id(), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = candidate.Close() })
			readinessWrite(t, candidate, "2probe")
			readinessRead(t, candidate, "3probe")
			readinessWrite(t, candidate, "5")
			readinessWait(t, upgraded, "transport replacement before initialization notification")
			closeListeners := conn.ListenerCount("close")
			release()
			readinessWait(t, handled, "initial handshake return")

			_ = candidate.SetReadDeadline(time.Now().Add(time.Second))
			if protocol == 3 {
				readinessWrite(t, candidate, "2")
			}
			want := "2"
			if protocol == 3 {
				want = "3"
			}
			if _, data, readErr := candidate.ReadMessage(); readErr != nil || string(data) != want {
				t.Fatalf("heartbeat after transport replacement = %q, error = %v, want %q", data, readErr, want)
			}
			if protocol == 4 {
				readinessWrite(t, candidate, "3")
			}
			readinessWait(t, heartbeat, "heartbeat on upgraded transport")
			if conn.Request().TakeTransportReady() != nil {
				t.Error("completed initialization retained its readiness callback")
			}
			if got := conn.ListenerCount("close"); got != closeListeners {
				t.Errorf("completed initialization changed close listeners: got %d, want %d", got, closeListeners)
			}
			// A responsive upgraded connection must still close when the peer
			// stops answering EIO4 PING or stops sending EIO3 PING.
			select {
			case reason := <-closed:
				if reason != "ping timeout" {
					t.Fatalf("close reason = %q, want ping timeout", reason)
				}
			case <-time.After(time.Second):
				t.Fatal("upgraded silent peer remained open after initialization")
			}
		})
	}
}
