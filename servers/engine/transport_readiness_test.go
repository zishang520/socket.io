package engine

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func newTransportReadTest(t *testing.T) (*types.HttpContext, *atomic.Int32) {
	t.Helper()
	contexts := make(chan *types.HttpContext, 1)
	closed := new(atomic.Int32)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		ctx := types.NewHttpContext(w, r)
		ctx.Websocket = &types.WebSocketConn{EventEmitter: types.NewEventEmitter(), Conn: conn}
		_ = ctx.Websocket.On("close", func(...any) { closed.Add(1) })
		contexts <- ctx
		<-release
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })
	peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	select {
	case ctx := <-contexts:
		return ctx, closed
	case <-time.After(2 * time.Second):
		t.Fatal("WebSocket upgrade did not complete")
		return nil, nil
	}
}

func readTransportPermission(t *testing.T, ctx *types.HttpContext) bool {
	t.Helper()
	select {
	case allowed := <-ctx.TransportReadPermission():
		return allowed
	case <-time.After(2 * time.Second):
		t.Fatal("initialization did not resolve read permission")
		return false
	}
}

func TestTransportReadinessPollingNeedsNoBarrier(t *testing.T) {
	ctx := &types.HttpContext{}
	if !withTransportInitialization(ctx, 0, func(aborted func() bool) bool {
		return !aborted() && ctx.TransportReadPermission() == nil
	}) {
		t.Fatal("polling unexpectedly requires stream initialization")
	}
}

func TestTransportReadinessCompletion(t *testing.T) {
	for _, success := range []bool{false, true} {
		t.Run(map[bool]string{false: "failure", true: "success"}[success], func(t *testing.T) {
			ctx, closed := newTransportReadTest(t)
			completed := false
			ctx.SetTransportReady(func() {
				completed = true
				if got := ctx.Websocket.ListenerCount("close"); got != 1 {
					t.Errorf("completion ran before initialization released its close listener: got %d listeners", got)
				}
			})
			if got := withTransportInitialization(ctx, 20*time.Millisecond, func(aborted func() bool) bool {
				if aborted() {
					t.Fatal("initialization started canceled")
				}
				return success
			}); got != success {
				t.Fatalf("initialization result = %v, want %v", got, success)
			}
			if completed != success || ctx.TakeTransportReady() != nil {
				t.Fatal("initialization did not invoke or discard its completion callback")
			}
			if readTransportPermission(t, ctx) != success {
				t.Fatal("read permission disagrees with initialization result")
			}
			if permission, open := <-ctx.TransportReadPermission(); permission || open {
				t.Fatal("more than one read permission was granted")
			}
			// A completed scope must not retain its timeout or close subscription.
			time.Sleep(40 * time.Millisecond)
			if success && closed.Load() != 0 || !success && closed.Load() != 1 {
				t.Fatalf("success = %v, connection closes = %d", success, closed.Load())
			}
			if got := ctx.Websocket.ListenerCount("close"); got != 1 {
				t.Fatalf("initialization retained a close subscription: got %d listeners", got)
			}
		})
	}
}

func TestTransportReadinessRawCloseCancelsWithoutRecursion(t *testing.T) {
	ctx, closed := newTransportReadTest(t)
	if withTransportInitialization(ctx, time.Hour, func(aborted func() bool) bool {
		_ = ctx.Websocket.Close()
		if !aborted() {
			t.Fatal("raw close did not cancel initialization")
		}
		return true
	}) || readTransportPermission(t, ctx) || closed.Load() != 1 {
		t.Fatal("raw close was ignored or recursively closed the connection")
	}
}

func TestTransportReadinessTimeoutReleasesReaderBeforeCloseReturns(t *testing.T) {
	ctx, _ := newTransportReadTest(t)
	entered, blocked, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(blocked) })
	t.Cleanup(release)
	_ = ctx.Websocket.Once("close", func(...any) {
		close(entered)
		<-blocked
		close(finished)
	})
	if withTransportInitialization(ctx, 20*time.Millisecond, func(aborted func() bool) bool {
		readinessWait(t, entered, "blocking raw close callback")
		if readTransportPermission(t, ctx) || !aborted() {
			t.Fatal("timeout did not cancel reading before close returned")
		}
		return true
	}) {
		t.Fatal("timed-out initialization succeeded")
	}
	release()
	readinessWait(t, finished, "raw close completion")
}

func TestTransportReadinessExpiredBeforeTimerCallback(t *testing.T) {
	ctx, closed := newTransportReadTest(t)
	previousProcs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(previousProcs) })
	// Stay on this goroutine for a short budget. Even if a timer callback is
	// delayed by the scheduler, the final decision must check elapsed time.
	const budget = 100 * time.Microsecond
	if withTransportInitialization(ctx, budget, func(func() bool) bool {
		until := time.Now().Add(2 * budget)
		for time.Now().Before(until) {
		}
		return true
	}) || readTransportPermission(t, ctx) {
		t.Fatal("expired initialization granted read permission")
	}
	if closed.Load() != 1 {
		t.Fatal("expired initialization did not close the connection")
	}
}

func TestTransportReadinessCompletionRacesClose(t *testing.T) {
	for range 32 {
		ctx, _ := newTransportReadTest(t)
		closed := make(chan struct{})
		completed := withTransportInitialization(ctx, time.Hour, func(func() bool) bool {
			go func() {
				_ = ctx.Websocket.Close()
				close(closed)
			}()
			return true
		})
		readinessWait(t, closed, "concurrent close")
		if readTransportPermission(t, ctx) != completed {
			t.Fatal("concurrent close disagrees with the initialization result")
		}
	}
}
