package engine

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
)

func TestHeartbeatPongBeforePingSendReturns(t *testing.T) {
	opts := config.DefaultServerOptions()
	// The timeout is shorter than the interval, as with the defaults, so a stale
	// timeout fires before the next ping's pong could clear it.
	opts.SetPingInterval(60 * time.Millisecond)
	opts.SetPingTimeout(30 * time.Millisecond)
	server := NewServer(opts)
	t.Cleanup(func() { server.Close() })

	closed := make(chan string, 1)
	_ = server.On("connection", func(args ...any) {
		conn := args[0].(*socket)
		_ = conn.On("packetCreate", func(args ...any) {
			if p, ok := args[0].(*packet.Packet); ok && p.Type == packet.PING {
				conn.onPacket(&packet.Packet{Type: packet.PONG})
			}
		})
		_ = conn.Once("close", func(args ...any) { closed <- args[0].(string) })
	})

	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	url := fmt.Sprintf("ws%s/engine.io/?EIO=4&transport=websocket", strings.TrimPrefix(httpServer.URL, "http"))
	peer, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Close() })
	go func() {
		for {
			if _, _, err := peer.ReadMessage(); err != nil {
				return
			}
		}
	}()

	select {
	case reason := <-closed:
		t.Fatalf("connection closed with %q although every ping was answered", reason)
	case <-time.After(20 * (opts.PingInterval() + opts.PingTimeout())):
	}
}
