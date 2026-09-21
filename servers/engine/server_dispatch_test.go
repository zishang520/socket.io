package engine_test

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/servers/engine/v3"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type rejectingHandshakeServer struct {
	engine.Server
	calls atomic.Int32
}

func (s *rejectingHandshakeServer) Handshake(string, *types.HttpContext) (*types.CodeMessage, transports.Transport) {
	s.calls.Add(1)
	return engine.FORBIDDEN, nil
}

func TestServerDispatchesHandshakeOverride(t *testing.T) {
	for _, name := range []string{"polling", "websocket"} {
		t.Run(name, func(t *testing.T) {
			s := &rejectingHandshakeServer{Server: engine.MakeServer()}
			s.Prototype(s)
			s.Construct(nil)
			t.Cleanup(func() { s.Close() })
			network := httptest.NewServer(s)
			t.Cleanup(network.Close)
			endpoint := network.URL + "/engine.io/?EIO=4&transport=" + name
			if name == "polling" {
				client := network.Client()
				client.Timeout = 2 * time.Second
				response, err := client.Get(endpoint)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = response.Body.Close() }()
				if response.StatusCode != http.StatusForbidden {
					t.Errorf("handshake rejection status=%d, want 403", response.StatusCode)
				}
			} else {
				peer, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(endpoint, "http"), nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = peer.Close() })
				_ = peer.SetReadDeadline(time.Now().Add(2 * time.Second))
				_, _, err = peer.ReadMessage()
				var closed *websocket.CloseError
				if !errors.As(err, &closed) || closed.Code != websocket.CloseNormalClosure || closed.Text != engine.FORBIDDEN.Message {
					t.Errorf("handshake rejection close=%v, want normal close with Forbidden", err)
				}
			}
			if calls := s.calls.Load(); calls != 1 || s.ClientsCount() != 0 {
				t.Errorf("custom handshake calls=%d clients=%d, want 1 and 0", calls, s.ClientsCount())
			}
		})
	}
}

type noUpgradesServer struct {
	engine.Server
	calls atomic.Int32
}

func (s *noUpgradesServer) Upgrades(string) []string {
	s.calls.Add(1)
	return nil
}

func TestSocketRetainsServerPrototype(t *testing.T) {
	s := &noUpgradesServer{Server: engine.MakeServer()}
	s.Prototype(s)
	s.Construct(nil)
	t.Cleanup(func() { s.Close() })
	network := httptest.NewServer(s)
	t.Cleanup(network.Close)
	client := network.Client()
	client.Timeout = 2 * time.Second
	response, err := client.Get(network.URL + "/engine.io/?EIO=4&transport=polling")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK || len(data) < 2 || data[0] != '0' {
		t.Fatalf("polling OPEN=(%d, %q), error=%v", response.StatusCode, data, err)
	}
	var body struct {
		Upgrades []string `json:"upgrades"`
	}
	if err := json.Unmarshal(data[1:], &body); err != nil {
		t.Fatal(err)
	}
	if calls := s.calls.Load(); calls != 1 || len(body.Upgrades) != 0 {
		t.Fatalf("server Upgrades calls=%d advertised upgrades=%v, want 1 and []", calls, body.Upgrades)
	}
}
