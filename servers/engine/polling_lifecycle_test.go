package engine

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zishang520/socket.io/servers/engine/v3/config"
)

type pollingHTTPResult struct {
	status int
	body   string
	err    error
}

func readPollingResponse(client *http.Client, req *http.Request) pollingHTTPResult {
	response, err := client.Do(req)
	if err != nil {
		return pollingHTTPResult{err: err}
	}
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	return pollingHTTPResult{status: response.StatusCode, body: string(body), err: err}
}

func newPollingLifecycleSession(t *testing.T) (*http.Client, string, <-chan struct{}, <-chan string) {
	t.Helper()
	opts := config.DefaultServerOptions()
	opts.SetPingInterval(time.Minute)
	opts.SetPingTimeout(time.Minute)
	server := NewServer(opts)
	t.Cleanup(func() { server.Close() })
	ready, closed := make(chan struct{}, 8), make(chan string, 1)
	_ = server.On("connection", func(args ...any) {
		socket := args[0].(Socket)
		_ = socket.Transport().On("ready", func(...any) { ready <- struct{}{} })
		_ = socket.Once("close", func(...any) { closed <- socket.Transport().ReadyState() })
	})
	httpServer := httptest.NewServer(http.HandlerFunc(server.ServeHTTP))
	t.Cleanup(httpServer.Close)
	client := &http.Client{Timeout: 2 * time.Second}
	t.Cleanup(client.CloseIdleConnections)
	url := httpServer.URL + "/engine.io/?EIO=4&transport=polling"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	response := readPollingResponse(client, req)
	if response.err != nil || response.status != http.StatusOK || len(response.body) == 0 || response.body[0] != '0' {
		t.Fatalf("handshake = %#v", response)
	}
	var handshake struct{ Sid string }
	if err := json.Unmarshal([]byte(response.body[1:]), &handshake); err != nil {
		t.Fatal(err)
	}
	return client, url + "&sid=" + handshake.Sid, ready, closed
}

func TestDuplicatePollingGetClosesBothRequests(t *testing.T) {
	client, url, ready, closed := newPollingLifecycleSession(t)
	first, _ := http.NewRequest(http.MethodGet, url, nil)
	pending := make(chan pollingHTTPResult, 1)
	go func() { pending <- readPollingResponse(client, first) }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("first GET did not become pending")
	}
	second, _ := http.NewRequest(http.MethodGet, url, nil)
	response := readPollingResponse(client, second)
	if response.err != nil || response.status != http.StatusBadRequest {
		t.Fatalf("duplicate GET = %#v, want 400", response)
	}
	response = <-pending
	if response.err != nil || response.status != http.StatusOK || response.body != "1" {
		t.Fatalf("pending GET = %#v, want 200 with CLOSE packet", response)
	}
	select {
	case state := <-closed:
		if state != "closed" {
			t.Fatalf("transport remained %q after the session closed", state)
		}
	case <-time.After(time.Second):
		t.Fatal("duplicate GET did not close the session")
	}
	third, _ := http.NewRequest(http.MethodGet, url, nil)
	if response := readPollingResponse(client, third); response.err != nil || response.status != http.StatusBadRequest {
		t.Fatalf("closed session accepted another GET: %#v", response)
	}
}

func TestCancelledPollingGetClosesSession(t *testing.T) {
	client, url, ready, closed := newPollingLifecycleSession(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	pending := make(chan pollingHTTPResult, 1)
	go func() { pending <- readPollingResponse(client, req) }()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("GET did not become pending")
	}
	cancel()
	if response := <-pending; response.err == nil {
		t.Fatal("canceled GET unexpectedly succeeded")
	}
	select {
	case state := <-closed:
		if state != "closed" {
			t.Fatalf("canceled transport remained %q waiting for another GET", state)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled GET did not close the session")
	}
	next, _ := http.NewRequest(http.MethodGet, url, nil)
	if response := readPollingResponse(client, next); response.err != nil || response.status != http.StatusBadRequest {
		t.Fatalf("canceled session accepted another GET: %#v", response)
	}
}
