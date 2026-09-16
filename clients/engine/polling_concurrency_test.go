package engine

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestPollingWriteDoesNotWaitForPendingGet(t *testing.T) {
	pollStarted := make(chan struct{})
	releasePoll := make(chan struct{})
	posted := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			data, _ := io.ReadAll(r.Body)
			posted <- string(data)
			_, _ = io.WriteString(w, "ok")
			return
		}
		if r.URL.Query().Get("sid") == "" {
			_, _ = io.WriteString(w, `0{"sid":"test","upgrades":[],"pingInterval":60000,"pingTimeout":60000,"maxPayload":1000000}`)
			return
		}
		close(pollStarted)
		select {
		case <-releasePoll:
		case <-r.Context().Done():
		}
		_, _ = io.WriteString(w, "1")
	}))
	defer server.Close()
	opts := DefaultSocketOptions()
	opts.SetTransports(types.NewSet[TransportCtor](&PollingBuilder{}))
	client := NewSocket(server.URL, opts)
	defer func() {
		close(releasePoll)
		client.Close()
	}()
	select {
	case <-pollStarted:
	case <-time.After(time.Second):
		t.Fatal("client did not start its long poll")
	}
	client.Write(strings.NewReader("hello"), nil, nil)
	select {
	case body := <-posted:
		if body != "4hello" {
			t.Fatalf("POST payload = %q, want 4hello", body)
		}
	case <-time.After(time.Second):
		t.Fatal("POST waited for the pending GET to complete")
	}
}
