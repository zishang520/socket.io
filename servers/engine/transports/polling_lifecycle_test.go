package transports

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestPollingRequestRegistrationConcurrentCancellation(t *testing.T) {
	for range 64 {
		requestCtx, cancel := context.WithCancel(context.Background())
		request := httptest.NewRequest(http.MethodGet, "/engine.io/?EIO=4&transport=polling", nil).WithContext(requestCtx)
		ctx := types.NewHttpContext(httptest.NewRecorder(), request)
		transport := NewPolling(ctx)
		start := make(chan struct{})
		registered, canceled := make(chan struct{}), make(chan struct{})
		go func() { <-start; transport.OnRequest(ctx); close(registered) }()
		go func() { <-start; cancel(); close(canceled) }()
		close(start)
		<-registered
		<-canceled
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
			t.Fatal("canceled polling request did not finalize")
		}
		transport.SetWritable(false)
		transport.Discard()
		transport.Close()
	}
}

func TestJSONPPollingResponsesReleasePendingRequest(t *testing.T) {
	var transport Jsonp
	t.Cleanup(func() {
		if transport != nil {
			transport.SetWritable(false)
			transport.OnClose()
		}
	})
	for _, message := range []string{"hello", "again"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, "/engine.io/?EIO=3&transport=polling&b64=1&j=7", nil)
		ctx := types.NewHttpContext(recorder, request)
		t.Cleanup(ctx.Flush)
		if transport == nil {
			transport = NewJSONP(ctx)
		}
		drained := make(chan struct{})
		_ = transport.Once("drain", func(...any) { close(drained) })
		transport.OnRequest(ctx)
		if ctx.Cleanup != nil {
			t.Fatal("JSONP polling installed a context Cleanup callback")
		}
		transport.Send([]*packet.Packet{{Type: packet.MESSAGE, Data: types.NewStringBufferString(message)}})
		select {
		case <-drained:
		case <-time.After(time.Second):
			t.Fatal("JSONP polling response did not finish")
		}
		if want := `___eio[7]("6:4` + message + `");`; recorder.Code != http.StatusOK || recorder.Body.String() != want {
			t.Fatalf("JSONP response: status=%d body=%q, want %q", recorder.Code, recorder.Body.String(), want)
		}
	}
}

func TestPollingOverlappingRequestDuringHeaderPreparation(t *testing.T) {
	firstRecorder := httptest.NewRecorder()
	first := types.NewHttpContext(firstRecorder, httptest.NewRequest(http.MethodGet, "/?EIO=4", nil))
	t.Cleanup(first.Flush)
	transport := NewPolling(first)
	t.Cleanup(func() {
		transport.SetWritable(false)
		transport.OnClose()
	})
	preparing, resume := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	defer release()
	_ = transport.Once("headers", func(...any) {
		close(preparing)
		<-resume
	})
	drained := make(chan struct{})
	_ = transport.Once("drain", func(...any) { close(drained) })
	transport.OnRequest(first)
	transport.Send([]*packet.Packet{{Type: packet.MESSAGE, Data: types.NewStringBufferString("first")}})
	select {
	case <-preparing:
	case <-time.After(time.Second):
		t.Fatal("polling response did not enter header preparation")
	}

	secondRecorder := httptest.NewRecorder()
	second := types.NewHttpContext(secondRecorder, httptest.NewRequest(http.MethodGet, "/?EIO=4", nil))
	t.Cleanup(second.Flush)
	transport.OnRequest(second)
	if !second.IsDone() || secondRecorder.Code != http.StatusBadRequest {
		t.Error("response preparation released its request slot before writing could begin")
	}
	release()
	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("original polling response did not finish after header preparation")
	}
	if firstRecorder.Code != http.StatusOK || firstRecorder.Body.String() != "4first" {
		t.Fatalf("original polling response: status=%d body=%q", firstRecorder.Code, firstRecorder.Body.String())
	}
}

func TestPollingCancellationErrorCanReenterWithNextRequest(t *testing.T) {
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request := httptest.NewRequest(http.MethodGet, "/?EIO=4", nil).WithContext(requestCtx)
	first := types.NewHttpContext(httptest.NewRecorder(), request)
	transport := NewPolling(first)
	t.Cleanup(func() {
		transport.SetWritable(false)
		transport.OnClose()
	})
	secondRecorder := httptest.NewRecorder()
	second := types.NewHttpContext(secondRecorder, httptest.NewRequest(http.MethodGet, "/?EIO=4", nil))
	t.Cleanup(second.Flush)
	returned := make(chan struct{})
	_ = transport.Once("error", func(...any) {
		transport.OnRequest(second)
		close(returned)
	})
	transport.OnRequest(first)
	cancel()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("cancellation error callback could not reenter OnRequest")
	}
	if !second.IsDone() || secondRecorder.Code != http.StatusBadRequest {
		t.Fatal("cancellation admitted a successor request through its error callback")
	}
	if !transport.Discarded() || transport.Writable() {
		t.Fatal("cancellation did not publish discard and non-writable state before its callback")
	}
}
