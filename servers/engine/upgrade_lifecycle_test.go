package engine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestPollingRequestDuringUpgradeHandoff(t *testing.T) {
	server := NewServer(nil)
	defer server.Close()
	httpServer := httptest.NewServer(http.HandlerFunc(server.ServeHTTP))
	defer httpServer.Close()
	client := &http.Client{Timeout: time.Second}
	defer client.CloseIdleConnections()
	url := httpServer.URL + "/engine.io/?EIO=4&transport=polling"
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	response := readPollingResponse(client, req)
	if response.err != nil || response.status != http.StatusOK {
		t.Fatalf("handshake = %#v", response)
	}
	var handshake struct{ Sid string }
	if err := json.Unmarshal([]byte(response.body[1:]), &handshake); err != nil {
		t.Fatal(err)
	}
	socket, ok := server.Clients().Load(handshake.Sid)
	if !ok {
		t.Fatal("missing session")
	}
	closing, release := make(chan struct{}), make(chan struct{})
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	_ = socket.Transport().Once("close", func(...any) {
		close(closing)
		<-release
	})
	ws, _, err := websocket.DefaultDialer.Dial(strings.Replace(httpServer.URL, "http:", "ws:", 1)+"/engine.io/?EIO=4&transport=websocket&sid="+handshake.Sid, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ws.Close() }()
	_ = ws.SetReadDeadline(time.Now().Add(time.Second))
	if err := ws.WriteMessage(websocket.TextMessage, []byte("2probe")); err != nil {
		t.Fatal(err)
	}
	if _, probe, err := ws.ReadMessage(); err != nil || string(probe) != "3probe" {
		t.Fatalf("probe = %q, %v", probe, err)
	}
	if err := ws.WriteMessage(websocket.TextMessage, []byte("5")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-closing:
	case <-time.After(time.Second):
		t.Fatal("upgrade did not retire polling")
	}
	req, _ = http.NewRequest(http.MethodGet, url+"&sid="+handshake.Sid, nil)
	response = readPollingResponse(client, req)
	unblock()
	if response.err != nil || response.status != http.StatusBadRequest {
		t.Fatalf("poll during upgrade handoff = %#v, want 400", response)
	}
}
