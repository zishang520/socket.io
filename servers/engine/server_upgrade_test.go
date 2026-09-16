package engine

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// countingResponseWriter counts calls to WriteHeader.
type countingResponseWriter struct {
	header       http.Header
	writeHeaders int
}

func (w *countingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *countingResponseWriter) WriteHeader(int) { w.writeHeaders++ }

func (w *countingResponseWriter) Write(b []byte) (int, error) { return len(b), nil }

func TestFailedUpgradeWritesHeaderOnce(t *testing.T) {
	s := NewServer(nil)

	// missing Sec-WebSocket-Version so the gorilla handshake fails
	req := httptest.NewRequest(http.MethodGet, "/engine.io/?EIO=4&transport=websocket", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")

	w := &countingResponseWriter{}
	s.ServeHTTP(w, req)

	if w.writeHeaders != 1 {
		t.Fatalf("WriteHeader called %d times on a failed upgrade, want 1", w.writeHeaders)
	}
}
