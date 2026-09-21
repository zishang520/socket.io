package engine

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
)

func TestNonPositiveInitializationTimeoutRejectsConnection(t *testing.T) {
	previousProcs := runtime.GOMAXPROCS(1)
	t.Cleanup(func() { runtime.GOMAXPROCS(previousProcs) })
	for _, budget := range []time.Duration{0, -time.Nanosecond} {
		t.Run(budget.String(), func(t *testing.T) {
			opts := config.DefaultServerOptions()
			opts.SetUpgradeTimeout(budget)
			server := NewServer(opts)
			t.Cleanup(func() { server.Close() })
			var published atomic.Int32
			_ = server.On("connection", func(...any) { published.Add(1) })
			handled := make(chan struct{})
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				defer close(handled)
				server.ServeHTTP(w, r)
			}))
			t.Cleanup(httpServer.Close)
			peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/engine.io/?EIO=4&transport=websocket", nil)
			if err == nil {
				t.Cleanup(func() { _ = peer.Close() })
				_ = peer.SetReadDeadline(time.Now().Add(time.Second))
				_, data, readErr := peer.ReadMessage()
				if readErr == nil {
					t.Fatalf("expired initialization delivered %q", data)
				}
				var timeout net.Error
				if errors.As(readErr, &timeout) && timeout.Timeout() {
					t.Fatal("expired initialization waited for the test read deadline")
				}
			}
			readinessWait(t, handled, "rejected initialization")
			if published.Load() != 0 || server.ClientsCount() != 0 || server.Clients().Len() != 0 {
				t.Fatal("expired initialization published or retained a connection")
			}
		})
	}
}
