package request

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type countedRequestBody struct {
	io.Reader
	closed atomic.Int32
}

func (b *countedRequestBody) Close() error {
	b.closed.Add(1)
	return nil
}

func cacheAlternative(t *testing.T, transport *Transport, requestURL, alternativeURL string) {
	t.Helper()
	origin, err := url.Parse(requestURL)
	if err != nil {
		t.Fatal(err)
	}
	alternative, err := url.Parse(alternativeURL)
	if err != nil {
		t.Fatal(err)
	}
	transport.altSvcCache.Store(getOrigin(origin), []*altSvc{{
		protocol: "h2", endpoint: alternative.Host, expires: time.Now().Add(time.Hour),
	}})
}

func TestTransportDoesNotRetryUnsafeRequests(t *testing.T) {
	for _, test := range []struct {
		name, method string
		replayable   bool
	}{
		{"post", http.MethodPost, true},
		{"patch", http.MethodPatch, true},
		{"one-shot body", http.MethodPut, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			const payload = "4message-with-side-effects"
			var alternateCalls, originalCalls atomic.Int32
			alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				alternateCalls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				_ = conn.Close()
			}))
			defer alternative.Close()
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				originalCalls.Add(1)
				_, _ = io.Copy(w, r.Body)
			}))
			defer origin.Close()
			transport := NewTransport(nil, nil)
			defer func() { _ = transport.Close() }()
			cacheAlternative(t, transport, origin.URL, alternative.URL)
			req, err := http.NewRequest(test.method, origin.URL, strings.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			if !test.replayable {
				req.GetBody = nil
			}
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil || string(body) != payload || alternateCalls.Load() != 0 || originalCalls.Load() != 1 {
				t.Fatalf("body=%q err=%v alternative calls=%d origin calls=%d", body, err, alternateCalls.Load(), originalCalls.Load())
			}
		})
	}
}

func TestTransportAlternativeBodyOwnership(t *testing.T) {
	for _, fails := range []bool{false, true} {
		name := "success"
		if fails {
			name = "fallback"
		}
		t.Run(name, func(t *testing.T) {
			const payload = "idempotent payload"
			var originalCalls atomic.Int32
			alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil || string(body) != payload {
					t.Errorf("alternative body=%q err=%v", body, err)
				}
				if fails {
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
					return
				}
				_, _ = w.Write(body)
			}))
			defer alternative.Close()
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				originalCalls.Add(1)
				_, _ = io.Copy(w, r.Body)
			}))
			defer origin.Close()
			transport := NewTransport(nil, nil)
			defer func() { _ = transport.Close() }()
			cacheAlternative(t, transport, origin.URL, alternative.URL)
			original := &countedRequestBody{Reader: strings.NewReader(payload)}
			copied := &countedRequestBody{Reader: strings.NewReader(payload)}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPut, origin.URL, original)
			if err != nil {
				t.Fatal(err)
			}
			req.ContentLength = int64(len(payload))
			var copies int
			req.GetBody = func() (io.ReadCloser, error) { copies++; return copied, nil }
			resp, err := transport.RoundTrip(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			if err != nil || string(body) != payload || copies != 1 {
				t.Fatalf("body=%q err=%v copies=%d", body, err, copies)
			}
			wantOriginCalls := int32(0)
			if fails {
				wantOriginCalls = 1
			}
			if originalCalls.Load() != wantOriginCalls {
				t.Fatalf("origin calls=%d, want %d", originalCalls.Load(), wantOriginCalls)
			}
			deadline := time.Now().Add(time.Second)
			for original.closed.Load() == 0 || copied.closed.Load() == 0 {
				if time.Now().After(deadline) {
					t.Fatalf("unclosed request bodies: original=%d copy=%d", original.closed.Load(), copied.closed.Load())
				}
				time.Sleep(time.Millisecond)
			}
			if original.closed.Load() != 1 || copied.closed.Load() != 1 {
				t.Fatalf("request bodies closed more than once: original=%d copy=%d", original.closed.Load(), copied.closed.Load())
			}
		})
	}
}

func TestTransportBodyCopyFailureKeepsOriginal(t *testing.T) {
	const payload = "original body"
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(w, r.Body) }))
	defer origin.Close()
	transport := NewTransport(nil, nil)
	defer func() { _ = transport.Close() }()
	cacheAlternative(t, transport, origin.URL, origin.URL)
	req, err := http.NewRequest(http.MethodPut, origin.URL, strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("cannot reopen body") }
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || string(body) != payload {
		t.Fatalf("body=%q err=%v", body, err)
	}
}

func TestTransportAlternativeUpdatesCache(t *testing.T) {
	for _, replace := range []bool{false, true} {
		name := "clear"
		if replace {
			name = "replace"
		}
		t.Run(name, func(t *testing.T) {
			var originCalls, alternateCalls, updatedCalls atomic.Int32
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				originCalls.Add(1)
				_, _ = io.WriteString(w, "origin")
			}))
			defer origin.Close()
			updated := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				updatedCalls.Add(1)
				_, _ = io.WriteString(w, "updated")
			}))
			defer updated.Close()
			header := "clear"
			if replace {
				endpoint, err := url.Parse(updated.URL)
				if err != nil {
					t.Fatal(err)
				}
				header = `h2="` + endpoint.Host + `"; ma=60`
			}
			alternative := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				alternateCalls.Add(1)
				w.Header().Set("Alt-Svc", header)
				_, _ = io.WriteString(w, "alternative")
			}))
			defer alternative.Close()
			transport := NewTransport(nil, nil)
			defer func() { _ = transport.Close() }()
			cacheAlternative(t, transport, origin.URL, alternative.URL)
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			for range 2 {
				response, err := client.Get(origin.URL)
				if err != nil {
					t.Fatal(err)
				}
				_, err = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			wantOrigin, wantUpdated := int32(1), int32(0)
			if replace {
				wantOrigin, wantUpdated = 0, 1
			}
			if alternateCalls.Load() != 1 || originCalls.Load() != wantOrigin || updatedCalls.Load() != wantUpdated {
				t.Fatalf("requests after %s: alternative=%d origin=%d updated=%d", name, alternateCalls.Load(), originCalls.Load(), updatedCalls.Load())
			}
		})
	}
}
