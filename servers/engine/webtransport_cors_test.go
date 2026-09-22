package engine

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go/http3"
	wt "github.com/quic-go/webtransport-go"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
	webtrans "github.com/zishang520/socket.io/v3/pkg/webtransport"
)

func TestWebTransportSharedServerPreservesOriginPolicies(t *testing.T) {
	const firstOrigin = "https://first.example"
	const secondOrigin = "https://second.example"
	const vetoedOrigin = "https://vetoed.example"
	servers := make(map[string]Server)
	for path, origin := range map[string]string{"/first": firstOrigin, "/second": secondOrigin} {
		opts := config.DefaultServerOptions()
		opts.SetTransports(types.NewSet[transports.TransportCtor](WebTransport))
		opts.SetCors(&types.Cors{Origin: []string{origin, vetoedOrigin}})
		s := NewServer(opts)
		t.Cleanup(func() { s.Close() })
		servers[path] = s
	}
	var checks atomic.Int32
	webServer := &wt.Server{
		H3: &http3.Server{},
		CheckOrigin: func(r *http.Request) bool {
			checks.Add(1)
			if origin := r.Header.Get("Origin"); origin != r.Header.Get("X-Expected-Origin") {
				t.Errorf("WebTransport origin checker received rewritten Origin %q", origin)
			}
			return r.Header.Get("Origin") != vetoedOrigin
		},
	}
	webServer.H3.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		servers[r.URL.Path].OnWebTransportSession(types.NewHttpContext(w, r), webServer)
	})
	baseURL, dialer := readinessWebTransportServer(t, webServer)
	t.Run("concurrent requests", func(t *testing.T) {
		for range 4 {
			for _, tc := range []struct {
				name   string
				path   string
				origin string
				status int
			}{
				{"first allowed", "/first", firstOrigin, http.StatusOK},
				{"second allowed", "/second", secondOrigin, http.StatusOK},
				{"first denied", "/first", secondOrigin, http.StatusForbidden},
				{"second denied", "/second", firstOrigin, http.StatusForbidden},
				{"missing origin", "/first", "", http.StatusOK},
				{"custom checker veto", "/first", vetoedOrigin, http.StatusBadRequest},
			} {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					webTransportCheckOriginHandshake(t, dialer, baseURL+tc.path, tc.origin, tc.status)
				})
			}
		}
	})
	// Only the requests accepted by Engine.IO's policy reach the owner's check.
	if got := checks.Load(); got != 16 {
		t.Fatalf("custom WebTransport origin checks = %d, want 16", got)
	}
}

func TestWebTransportPreservesDefaultOriginPolicy(t *testing.T) {
	opts := config.DefaultServerOptions()
	opts.SetTransports(types.NewSet[transports.TransportCtor](WebTransport))
	opts.SetCors(&types.Cors{Origin: "*"})
	s := NewServer(opts)
	t.Cleanup(func() { s.Close() })
	webServer := &wt.Server{H3: &http3.Server{}}
	webServer.H3.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.OnWebTransportSession(types.NewHttpContext(w, r), webServer)
	})
	baseURL, dialer := readinessWebTransportServer(t, webServer)
	for _, tc := range []struct {
		name   string
		origin string
		status int
	}{
		{"same origin", baseURL, http.StatusOK},
		{"missing origin", "", http.StatusOK},
		{"cross origin", "https://other.example", http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			webTransportCheckOriginHandshake(t, dialer, baseURL+"/engine.io/", tc.origin, tc.status)
		})
	}
}

func webTransportCheckOriginHandshake(t *testing.T, dialer *wt.Transport, url, origin string, status int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	header := make(http.Header)
	if origin != "" {
		header.Set("Origin", origin)
		header.Set("X-Expected-Origin", origin)
	}
	response, session, err := dialer.Dial(ctx, url, header)
	if session != nil {
		defer func() { _ = session.CloseWithError(0, "") }()
	}
	if response == nil || response.StatusCode != status {
		t.Fatalf("WebTransport response = %v, error = %v; want status %d", response, err, status)
	}
	if status != http.StatusOK {
		_ = response.Body.Close()
		if err == nil || session != nil {
			t.Fatalf("rejected WebTransport request: session = %v, error = %v", session, err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	stream, err := session.OpenStreamSync(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conn := webtrans.NewConn(session, stream, false, 0, 0, nil, nil, nil)
	readinessWrite(t, conn, "0")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil || !strings.HasPrefix(string(data), `0{"`) {
		t.Fatalf("WebTransport OPEN = %q, error = %v", data, err)
	}
}
