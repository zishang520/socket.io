package engine

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type failedReadinessServer struct {
	Server
	closed chan struct{}
}

func (s *failedReadinessServer) CreateTransport(name string, ctx *types.HttpContext) (transports.Transport, error) {
	transport, err := s.Server.CreateTransport(name, ctx)
	if err != nil {
		return nil, err
	}
	_ = transport.Once("close", func(...any) { close(s.closed) })
	return nil, errors.New("construction failed after creating the transport")
}

func TestFailedHandshakeClosesDeferredTransport(t *testing.T) {
	server := &failedReadinessServer{Server: MakeServer(), closed: make(chan struct{})}
	server.Prototype(server)
	server.Construct(nil)
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	t.Cleanup(func() { server.Close() })

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(httpServer.URL, "http")+"/engine.io/?EIO=4&transport=websocket", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	initializationTimeoutDisconnected(t, client)
	readinessWait(t, server.closed, "failed construction closing the deferred transport")
	if got := server.ClientsCount(); got != 0 {
		t.Fatalf("failed handshake registered %d clients", got)
	}
}
