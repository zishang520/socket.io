package engine

import (
	"errors"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type prototypeDispatchServer struct {
	Server
	factoryCalls atomic.Int32
}

func (*prototypeDispatchServer) GenerateId(*types.HttpContext) string {
	return "custom-session-id"
}

func (s *prototypeDispatchServer) CreateTransport(name string, ctx *types.HttpContext) (transports.Transport, error) {
	s.factoryCalls.Add(1)
	if ctx.Query().Has("sid") {
		return nil, errors.New("custom factory rejects the upgrade candidate")
	}
	return s.Server.CreateTransport(name, ctx)
}

func TestHandshakeUsesPrototypeGenerateId(t *testing.T) {
	s := &prototypeDispatchServer{Server: MakeServer()}
	s.Prototype(s)
	s.Construct(nil)
	t.Cleanup(func() { s.Close() })
	connected := make(chan struct{})
	_ = s.On("connection", func(args ...any) {
		readinessEcho(t, args[0].(Socket))
		close(connected)
	})
	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)
	sid := readinessPollingHandshake(t, httpServer)
	readinessWait(t, connected, "custom SID connection initialization")
	if sid != "custom-session-id" {
		t.Fatalf("polling OPEN sid=%q, want the overridden GenerateId result", sid)
	}
	readinessPollingEcho(t, httpServer, sid)
}

func TestUpgradeUsesPrototypeCreateTransport(t *testing.T) {
	s := &prototypeDispatchServer{Server: MakeServer()}
	s.Prototype(s)
	s.Construct(nil)
	t.Cleanup(func() { s.Close() })
	connected := make(chan struct{})
	_ = s.On("connection", func(args ...any) {
		readinessEcho(t, args[0].(Socket))
		close(connected)
	})
	httpServer := httptest.NewServer(s)
	t.Cleanup(httpServer.Close)
	sid := readinessPollingHandshake(t, httpServer)
	readinessWait(t, connected, "polling connection initialization")
	if calls := s.factoryCalls.Load(); calls != 1 {
		t.Fatalf("handshake called the custom factory %d times, want 1", calls)
	}

	url := "ws" + strings.TrimPrefix(httpServer.URL, "http") + "/engine.io/?EIO=4&transport=websocket&sid=" + sid
	client, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	initializationTimeoutDisconnected(t, client)
	if calls := s.factoryCalls.Load(); calls != 2 {
		t.Fatalf("handshake and upgrade called the custom factory %d times, want 2", calls)
	}
	readinessPollingEcho(t, httpServer, sid)
}
