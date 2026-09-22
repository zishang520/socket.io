package types

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestHttpContextGettersCacheIndependentFirstReads(t *testing.T) {
	req := httptest.NewRequest("get", "/initial?value=initial", nil)
	req.Host = "initial.example"
	req.Header.Set("User-Agent", "initial-agent")
	rec := httptest.NewRecorder()
	ctx := NewHttpContext(rec, req)
	t.Cleanup(ctx.Flush)

	if got := ctx.Method(); got != http.MethodGet {
		t.Fatalf("Method = %q, want GET", got)
	}
	req.Method = http.MethodPost
	req.Host = " next.example:8443 "
	if got := ctx.Host(); got != "next.example" {
		t.Fatalf("Host cached before its first read: %q", got)
	}
	req.Host = "later.example"
	req.URL.Path = "/next/path/"
	if got := ctx.Path(); got != "next/path" {
		t.Fatalf("Path cached before its first read: %q", got)
	}
	req.URL.Path = "/later/path/"
	req.Header.Set("User-Agent", "next-agent")
	if got := ctx.UserAgent(); got != "next-agent" {
		t.Fatalf("UserAgent cached before its first read: %q", got)
	}
	req.Header.Set("User-Agent", "later-agent")
	req.URL.RawQuery = "value=next"
	query := ctx.Query()
	if got := query.Peek("value"); got != "next" {
		t.Fatalf("Query cached before its first read: %q", got)
	}
	req.URL.RawQuery = "value=later"
	req.Header = http.Header{"X-Value": {"next"}}
	headers := ctx.Headers()
	if got := headers.Peek("X-Value"); got != "next" {
		t.Fatalf("Headers cached before its first read: %q", got)
	}
	req.Header.Set("X-Value", "aliased")
	if got := headers.Peek("X-Value"); got != "aliased" {
		t.Fatalf("Headers lost its request map alias: %q", got)
	}
	req.Header = http.Header{"X-Value": {"replacement"}}
	rec.Header().Set("X-Response", "next")
	responseHeaders := ctx.ResponseHeaders()
	if got := responseHeaders.Peek("X-Response"); got != "next" {
		t.Fatalf("ResponseHeaders cached before its first read: %q", got)
	}
	responseHeaders.Set("X-Response", "aliased")
	if got := rec.Header().Get("X-Response"); got != "aliased" {
		t.Fatalf("ResponseHeaders lost its response map alias: %q", got)
	}

	if ctx.Method() != http.MethodGet || ctx.Host() != "next.example" || ctx.Path() != "next/path" || ctx.UserAgent() != "next-agent" {
		t.Error("request metadata changed after its first read")
	}
	if ctx.Query() != query || query.Peek("value") != "next" {
		t.Error("Query changed after its first read")
	}
	if ctx.Headers() != headers || headers.Peek("X-Value") != "aliased" || ctx.ResponseHeaders() != responseHeaders {
		t.Error("header getters replaced their cached bags")
	}
	if got := ctx.PathInfo(); got != "/later/path/" {
		t.Errorf("PathInfo must keep reading the current request path, got %q", got)
	}
}

type countedHttpContextHeaderWriter struct {
	http.ResponseWriter
	calls      atomic.Int32
	panicValue any
}

func (w *countedHttpContextHeaderWriter) Header() http.Header {
	w.calls.Add(1)
	if w.panicValue != nil {
		panic(w.panicValue)
	}
	return w.ResponseWriter.Header()
}

func TestHttpContextResponseHeadersInitializesOnceConcurrently(t *testing.T) {
	writer := &countedHttpContextHeaderWriter{ResponseWriter: httptest.NewRecorder()}
	ctx := NewHttpContext(writer, httptest.NewRequest(http.MethodGet, "/", nil))
	t.Cleanup(ctx.Flush)
	if count := writer.calls.Load(); count != 0 {
		t.Fatalf("constructor read response headers %d times", count)
	}
	var results [16]*ParameterBag
	start := make(chan struct{})
	var workers sync.WaitGroup
	for worker := range results {
		workers.Go(func() {
			<-start
			results[worker] = ctx.ResponseHeaders()
		})
	}
	close(start)
	workers.Wait()
	if count := writer.calls.Load(); count != 1 {
		t.Fatalf("response headers initialized %d times, want 1", count)
	}
	for _, result := range results {
		if result == nil || result != results[0] {
			t.Fatal("concurrent readers received different response header bags")
		}
	}
}

func TestHttpContextResponseHeadersReplaysInitializationPanic(t *testing.T) {
	panicValue := errors.New("test Header panic")
	writer := &countedHttpContextHeaderWriter{
		ResponseWriter: httptest.NewRecorder(),
		panicValue:     panicValue,
	}
	ctx := NewHttpContext(writer, httptest.NewRequest(http.MethodGet, "/", nil))
	t.Cleanup(ctx.Flush)
	for range 3 {
		func() {
			defer func() {
				if got := recover(); got != panicValue {
					t.Errorf("getter panic = %v, want the original value %v", got, panicValue)
				}
			}()
			ctx.ResponseHeaders()
			t.Error("getter did not replay its initialization panic")
		}()
	}
	if count := writer.calls.Load(); count != 1 {
		t.Fatalf("panicking initializer ran %d times, want 1", count)
	}
}
