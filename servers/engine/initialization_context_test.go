package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestHeartbeatSurvivesRequestContextTimeoutDuringInitialization(t *testing.T) {
	for _, protocol := range []int{3, 4} {
		t.Run(fmt.Sprintf("EIO%d", protocol), func(t *testing.T) {
			opts := config.DefaultServerOptions()
			opts.SetAllowEIO3(protocol == 3)
			opts.SetPingInterval(20 * time.Millisecond)
			opts.SetPingTimeout(80 * time.Millisecond)
			opts.SetUpgradeTimeout(2 * time.Second)
			opts.SetIdleTimeout(0)
			server := NewServer(opts)
			t.Cleanup(func() { server.Close() })

			// An unrelated marker observes completion of HttpContext.Clear,
			// independently of where the initialization callback is stored.
			const cleanupMarker types.EventName = "test:request_cleanup"
			server.Use(func(ctx *types.HttpContext, next func(error)) {
				_ = ctx.Once(cleanupMarker, func(...any) {})
				next(nil)
			})
			resume := make(chan struct{})
			release := sync.OnceFunc(func() { close(resume) })
			defer release()
			initializing := make(chan Socket, 1)
			heartbeat := make(chan struct{}, 1)
			closed := make(chan string, 1)
			_ = server.On("connection", func(args ...any) {
				conn := args[0].(Socket)
				readinessEcho(t, conn)
				_ = conn.Once("heartbeat", func(...any) { heartbeat <- struct{}{} })
				_ = conn.Once("close", func(args ...any) { closed <- args[0].(string) })
				initializing <- conn
				<-resume
			})
			handlerDone := make(chan struct{})
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handlerDone)
				requestCtx, cancel := context.WithTimeout(r.Context(), 100*time.Millisecond)
				defer cancel()
				server.ServeHTTP(w, r.WithContext(requestCtx))
			}))
			t.Cleanup(httpServer.Close)
			url := fmt.Sprintf("ws%s/engine.io/?EIO=%d&transport=websocket", strings.TrimPrefix(httpServer.URL, "http"), protocol)
			peer, _, err := websocket.DefaultDialer.Dial(url, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = peer.Close() })
			_ = peer.SetReadDeadline(time.Now().Add(time.Second))
			if _, data, readErr := peer.ReadMessage(); readErr != nil || !strings.HasPrefix(string(data), `0{"`) {
				t.Fatalf("initial OPEN = %q, error = %v", data, readErr)
			}
			var conn Socket
			select {
			case conn = <-initializing:
			case <-time.After(time.Second):
				t.Fatal("connection did not enter initialization")
			}
			readinessWait(t, conn.Request().Done(), "HTTP request timeout")
			deadline := time.Now().Add(time.Second)
			for conn.Request().ListenerCount(cleanupMarker) != 0 {
				if time.Now().After(deadline) {
					t.Fatal("HTTP context did not clear its listeners")
				}
				time.Sleep(time.Millisecond)
			}
			if err := conn.Request().Context().Err(); err != context.DeadlineExceeded {
				t.Fatalf("request context error = %v, want deadline exceeded", err)
			}
			if conn.ReadyState() != "open" {
				t.Fatal("request timeout closed the upgraded stream")
			}
			closeListeners := conn.ListenerCount("close")
			release()
			readinessWait(t, handlerDone, "initialization completion after HTTP timeout")

			// The hijacked stream remains usable after the request expires.
			readinessWrite(t, peer, "4echo")
			sawPing := false
			for {
				_ = peer.SetReadDeadline(time.Now().Add(time.Second))
				_, data, readErr := peer.ReadMessage()
				if readErr != nil {
					t.Fatalf("stream echo after request timeout: %v", readErr)
				}
				if string(data) == "4echo" {
					break
				}
				if protocol != 4 || string(data) != "2" {
					t.Fatalf("unexpected packet before echo: %q", data)
				}
				sawPing = true
				readinessWrite(t, peer, "3")
			}
			if protocol == 3 {
				readinessWrite(t, peer, "2")
				readinessRead(t, peer, "3")
			} else if !sawPing {
				readinessRead(t, peer, "2")
				readinessWrite(t, peer, "3")
			}
			readinessWait(t, heartbeat, "heartbeat after HTTP request timeout")
			if conn.Request().TakeTransportReady() != nil {
				t.Error("completed initialization retained its readiness callback")
			}
			if got := conn.ListenerCount("close"); got != closeListeners {
				t.Errorf("completed initialization changed close listeners: got %d, want %d", got, closeListeners)
			}
			select {
			case reason := <-closed:
				if reason != "ping timeout" {
					t.Fatalf("close reason = %q, want ping timeout", reason)
				}
			case <-time.After(time.Second):
				t.Fatal("silent peer remained open after HTTP request timeout")
			}
		})
	}
}
