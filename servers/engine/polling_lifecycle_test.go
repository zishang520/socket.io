package engine

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/v3/pkg/types"
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

func newPollingLifecycleHTTPClient(t *testing.T) (*http.Client, string, <-chan struct{}, <-chan string) {
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
	client, url, ready, closed := newPollingLifecycleHTTPClient(t)
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
	client, url, ready, closed := newPollingLifecycleHTTPClient(t)
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

func TestPollingCanceledBeforeMiddlewareReturns(t *testing.T) {
	opts := config.DefaultServerOptions()
	opts.SetPingInterval(time.Hour)
	opts.SetPingTimeout(time.Hour)
	server := NewServer(opts)
	t.Cleanup(func() { server.Close() })
	connected := make(chan Socket, 1)
	_ = server.Once("connection", func(args ...any) { connected <- args[0].(Socket) })
	entered := make(chan *types.HttpContext, 1)
	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	const marker types.EventName = "test:polling_context_clear"
	server.Use(func(ctx *types.HttpContext, next func(error)) {
		if ctx.Query().Peek("case") == "cancel" {
			_ = ctx.Once(marker, func(...any) {})
			entered <- ctx
			<-resume
		}
		next(nil)
	})
	handled := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("case") == "cancel" {
			defer close(handled)
		}
		server.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	client := httpServer.Client()
	client.Timeout = 2 * time.Second
	endpoint := httpServer.URL + "/engine.io/?EIO=4&transport=polling"
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("initial polling response: status=%d err=%v", response.StatusCode, readErr)
	}
	var socket Socket
	select {
	case socket = <-connected:
	case <-time.After(time.Second):
		t.Fatal("polling handshake did not publish a socket")
	}
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint+"&sid="+socket.Id()+"&case=cancel", nil)
	if err != nil {
		t.Fatal(err)
	}
	requestDone := make(chan error, 1)
	go func() {
		pollResponse, requestErr := client.Do(request)
		if pollResponse != nil {
			_ = pollResponse.Body.Close()
		}
		requestDone <- requestErr
	}()
	var pending *types.HttpContext
	select {
	case pending = <-entered:
	case <-time.After(time.Second):
		t.Fatal("poll request did not enter middleware")
	}
	cancel()
	select {
	case <-pending.Done():
	case <-time.After(time.Second):
		t.Fatal("peer cancellation did not finalize HTTP context")
	}
	deadline := time.Now().Add(time.Second)
	for pending.ListenerCount(marker) != 0 {
		if time.Now().After(deadline) {
			t.Fatal("canceled HTTP context did not clear its emitter")
		}
		runtime.Gosched()
	}
	if requestErr := <-requestDone; requestErr == nil {
		t.Fatal("canceled polling request unexpectedly succeeded")
	}
	release()
	select {
	case <-handled:
	case <-time.After(time.Second):
		t.Fatal("canceled request did not finish after middleware returned")
	}
	if socket.ReadyState() == "open" && socket.Transport().Writable() {
		t.Error("middleware continuation published an already-canceled poll as writable")
	}
	if socket.ReadyState() == "closed" {
		if socket.Transport().ReadyState() != "closed" || server.ClientsCount() != 0 {
			t.Fatal("canceled request left a partially closed polling session")
		}
		return
	}
	socket.Send(types.NewStringBuffer([]byte("after-cancel")), nil, nil)
	response, err = client.Get(endpoint + "&sid=" + socket.Id())
	if err != nil {
		t.Fatalf("surviving session did not answer its next poll: %v", err)
	}
	defer func() { _ = response.Body.Close() }()
	body, bodyErr := io.ReadAll(response.Body)
	if bodyErr != nil || response.StatusCode != http.StatusOK || string(body) != "4after-cancel" {
		t.Fatalf("next poll: status=%d body=%q error=%v", response.StatusCode, body, bodyErr)
	}
}

type flushedPollingResponseWriter struct {
	http.ResponseWriter
	flushed chan struct{}
	resume  <-chan struct{}
}

func (w *flushedPollingResponseWriter) Write(body []byte) (int, error) {
	n, err := w.ResponseWriter.Write(body)
	w.ResponseWriter.(http.Flusher).Flush()
	close(w.flushed)
	<-w.resume
	return n, err
}

func TestPollingConsumedResponsePreservesNextRequest(t *testing.T) {
	opts := config.DefaultServerOptions()
	opts.SetPingInterval(time.Hour)
	opts.SetPingTimeout(time.Hour)
	server := NewServer(opts)
	t.Cleanup(func() { server.Close() })
	connected := make(chan Socket, 1)
	_ = server.Once("connection", func(args ...any) { connected <- args[0].(Socket) })
	handshakeDone := make(chan struct{})
	flushed, resume := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("case") {
		case "first":
			w = &flushedPollingResponseWriter{ResponseWriter: w, flushed: flushed, resume: resume}
		case "":
			defer close(handshakeDone)
		}
		server.ServeHTTP(w, r)
	}))
	t.Cleanup(httpServer.Close)
	firstTransport, secondTransport := &http.Transport{}, &http.Transport{}
	defer firstTransport.CloseIdleConnections()
	defer secondTransport.CloseIdleConnections()
	firstClient := &http.Client{Transport: firstTransport, Timeout: 3 * time.Second}
	secondClient := &http.Client{Transport: secondTransport, Timeout: 3 * time.Second}
	endpoint := httpServer.URL + "/engine.io/?EIO=4&transport=polling"
	response, err := firstClient.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("handshake response: status=%d err=%v", response.StatusCode, readErr)
	}
	readinessWait(t, handshakeDone, "polling handshake completion")
	var socket Socket
	select {
	case socket = <-connected:
	case <-time.After(time.Second):
		t.Fatal("polling handshake did not publish a socket")
	}
	ready := make(chan struct{}, 2)
	_ = socket.Transport().On("ready", func(...any) { ready <- struct{}{} })
	transportErrors := make(chan any, 2)
	_ = socket.Transport().On("error", func(args ...any) { transportErrors <- args[0] })
	type result struct {
		body   string
		length int64
		err    error
	}
	fetch := func(client *http.Client, request *http.Request, resultChannel chan<- result) {
		response, requestErr := client.Do(request)
		if requestErr != nil {
			resultChannel <- result{err: requestErr}
			return
		}
		body, bodyErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		resultChannel <- result{body: string(body), length: response.ContentLength, err: bodyErr}
	}
	firstRequest, err := http.NewRequest(http.MethodGet, endpoint+"&sid="+socket.Id()+"&case=first", nil)
	if err != nil {
		t.Fatal(err)
	}
	firstResult := make(chan result, 1)
	go fetch(firstClient, firstRequest, firstResult)
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("first HTTP poll was not registered")
	}
	socket.Send(types.NewStringBuffer([]byte("first")), nil, nil)
	var first result
	select {
	case first = <-firstResult:
	case <-time.After(time.Second):
		t.Fatal("client could not consume the first flushed polling response")
	}
	if first.err != nil || first.body != "4first" || first.length != int64(len(first.body)) {
		t.Fatalf("first response not fully consumed with known Content-Length: body=%q length=%d err=%v", first.body, first.length, first.err)
	}
	readinessWait(t, flushed, "first polling response flush")
	drained := make(chan struct{})
	_ = socket.Transport().Once("drain", func(...any) { close(drained) })
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	secondRequest, err := http.NewRequestWithContext(secondCtx, http.MethodGet, endpoint+"&sid="+socket.Id()+"&case=second", nil)
	if err != nil {
		t.Fatal(err)
	}
	secondResult := make(chan result, 1)
	go fetch(secondClient, secondRequest, secondResult)
	select {
	case <-ready:
	case early := <-secondResult:
		t.Fatalf("successor was rejected before registration: %+v", early)
	case <-time.After(time.Second):
		t.Fatal("successor HTTP poll was not registered on its independent connection")
	}
	release()
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("first polling response did not finish after releasing its writer")
	}
	socket.Send(types.NewStringBuffer([]byte("second")), nil, nil)
	select {
	case failure := <-transportErrors:
		t.Errorf("legal successor GET lost after first response finalization: transport error=%v; first body fully consumed=%q", failure, first.body)
		cancelSecond()
		<-secondResult
	case response := <-secondResult:
		if response.err != nil || response.body != "4second" {
			t.Errorf("successor response=%q err=%v", response.body, response.err)
		}
	case <-time.After(time.Second):
		t.Error("legal successor GET did not receive the queued message")
		cancelSecond()
		<-secondResult
	}
}

func newPollingLifecycleSession(t *testing.T) (Server, Socket, *http.Client, string) {
	t.Helper()
	opts := config.DefaultServerOptions()
	opts.SetPingInterval(time.Hour)
	opts.SetPingTimeout(time.Hour)
	server := NewServer(opts)
	t.Cleanup(func() { server.Close() })
	connected := make(chan Socket, 1)
	_ = server.Once("connection", func(args ...any) { connected <- args[0].(Socket) })
	httpServer := httptest.NewServer(server)
	t.Cleanup(httpServer.Close)
	client := httpServer.Client()
	client.Timeout = 2 * time.Second
	t.Cleanup(client.CloseIdleConnections)
	endpoint := httpServer.URL + "/engine.io/?EIO=4&transport=polling"
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK || len(body) < 2 || body[0] != '0' {
		t.Fatalf("polling handshake: status=%d body=%q error=%v", response.StatusCode, body, readErr)
	}
	select {
	case socket := <-connected:
		return server, socket, client, endpoint + "&sid=" + socket.Id()
	case <-time.After(time.Second):
		t.Fatal("polling handshake did not publish a socket")
		return nil, nil, nil, ""
	}
}

func TestPollingCanceledRegisteredRequestClosesSession(t *testing.T) {
	server, socket, client, endpoint := newPollingLifecycleSession(t)
	ready, closed := make(chan struct{}), make(chan struct{})
	_ = socket.Transport().Once("ready", func(...any) { close(ready) })
	_ = socket.Once("close", func(...any) { close(closed) })
	requestCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	returned := make(chan error, 1)
	go func() {
		response, requestErr := client.Do(request)
		if response != nil {
			_ = response.Body.Close()
		}
		returned <- requestErr
	}()
	readinessWait(t, ready, "pending polling request registration")
	cancel()
	select {
	case requestErr := <-returned:
		if !errors.Is(requestErr, context.Canceled) {
			t.Fatalf("canceled polling request error = %v, want context.Canceled", requestErr)
		}
	case <-time.After(time.Second):
		t.Fatal("client cancellation did not return")
	}
	readinessWait(t, closed, "session close after premature polling disconnect")
	if socket.ReadyState() != "closed" || socket.Transport().ReadyState() != "closed" || server.ClientsCount() != 0 {
		t.Fatal("premature polling disconnect did not release the session and transport")
	}
}

func TestPollingOverlappingRequestReleasesOriginal(t *testing.T) {
	server, socket, client, endpoint := newPollingLifecycleSession(t)
	ready := make(chan struct{})
	_ = socket.Transport().Once("ready", func(...any) { close(ready) })
	requestCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		status int
		body   string
		err    error
	}
	original := make(chan result, 1)
	go func() {
		response, requestErr := client.Do(request)
		if requestErr != nil {
			original <- result{err: requestErr}
			return
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		original <- result{status: response.StatusCode, body: string(body), err: readErr}
	}()
	readinessWait(t, ready, "original polling request registration")
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusBadRequest {
		t.Fatalf("overlapping polling request: status=%d error=%v", response.StatusCode, readErr)
	}
	select {
	case first := <-original:
		if first.err != nil || first.status != http.StatusOK || first.body != "1" {
			t.Fatalf("original polling request did not receive CLOSE: status=%d body=%q error=%v", first.status, first.body, first.err)
		}
	case <-time.After(time.Second):
		t.Fatal("overlapping request left the original GET pending")
	}
	if socket.ReadyState() != "closed" || socket.Transport().ReadyState() != "closed" || server.ClientsCount() != 0 {
		t.Fatal("overlapping polling requests did not release the session and transport")
	}
}
