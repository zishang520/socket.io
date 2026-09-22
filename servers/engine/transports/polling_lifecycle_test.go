package transports

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestPollingCancelledRequestRegistration(t *testing.T) {
	for _, alreadyClosed := range []bool{true, false} {
		for range 50 {
			requestCtx, cancel := context.WithCancel(t.Context())
			request := httptest.NewRequest(http.MethodGet, "/engine.io/?EIO=4", nil).WithContext(requestCtx)
			ctx := types.NewHttpContext(httptest.NewRecorder(), request)
			p := NewPolling(ctx).(*polling)
			failure := make(chan struct{}, 1)
			_ = p.On("error", func(...any) { failure <- struct{}{} })
			if alreadyClosed {
				closed := make(chan struct{})
				_ = ctx.Once("close", func(...any) { close(closed) })
				cancel()
				<-closed
			} else {
				go cancel()
			}
			p.OnRequest(ctx)
			select {
			case <-ctx.Done():
			case <-time.After(time.Second):
				t.Fatal("canceled GET did not finalize its request")
			}
			p.reqMu.Lock()
			active := p.req != nil || p.Writable()
			p.reqMu.Unlock()
			// A request canceled before admission can be skipped. If registration
			// won the race, its close callback must still retire the request.
			if active {
				select {
				case <-failure:
				case <-time.After(time.Second):
					t.Fatal("registered canceled GET did not report premature close")
				}
			}
			p.reqMu.Lock()
			active = p.req != nil || p.Writable()
			p.reqMu.Unlock()
			if active {
				t.Fatal("canceled GET remained writable or retained its request")
			}
			p.writeQueue.Close()
		}
	}
}

func TestDiscardedPollingRejectsLateGet(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := types.NewHttpContext(recorder, httptest.NewRequest(http.MethodGet, "/engine.io/?EIO=4", nil))
	defer ctx.Flush()
	p := NewPolling(ctx).(*polling)
	p.Discard()
	p.Close()
	p.OnRequest(ctx)
	p.reqMu.Lock()
	pending := p.req != nil
	p.reqMu.Unlock()
	if !ctx.IsDone() || recorder.Code != http.StatusBadRequest || pending || p.Writable() {
		t.Fatal("discarded polling accepted a late GET")
	}
}

type blockedPollingResponse struct {
	Polling
	started chan struct{}
	release chan struct{}
}

func (p *blockedPollingResponse) DoWrite(ctx *types.HttpContext, data types.BufferInterface, options *packet.Options, callback func(error)) {
	close(p.started)
	<-p.release
	p.Polling.DoWrite(ctx, data, options, callback)
}

func TestPollingRejectsOverlapDuringResponsePreparation(t *testing.T) {
	firstRecorder := httptest.NewRecorder()
	first := types.NewHttpContext(firstRecorder, httptest.NewRequest(http.MethodGet, "/engine.io/?EIO=4", nil))
	defer first.Flush()
	p := NewPolling(first).(*polling)
	blocked := &blockedPollingResponse{Polling: p, started: make(chan struct{}), release: make(chan struct{})}
	p.Prototype(blocked)
	defer func() { close(blocked.release); p.writeQueue.Close() }()
	p.OnRequest(first)
	p.Send([]*packet.Packet{{Type: packet.MESSAGE, Data: strings.NewReader("hello")}})
	select {
	case <-blocked.started:
	case <-time.After(time.Second):
		t.Fatal("response did not start")
	}
	recorder := httptest.NewRecorder()
	duplicate := types.NewHttpContext(recorder, httptest.NewRequest(http.MethodGet, "/engine.io/?EIO=4", nil))
	defer duplicate.Flush()
	p.OnRequest(duplicate)
	p.reqMu.Lock()
	pending := p.req
	p.reqMu.Unlock()
	if !duplicate.IsDone() || recorder.Code != http.StatusBadRequest || pending != first {
		t.Fatal("accepted a duplicate GET while the first response was still being written")
	}
}

func TestJSONPResponseReleasesPollingRequest(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx := types.NewHttpContext(recorder, httptest.NewRequest(http.MethodGet, "/engine.io/?EIO=3&j=0&b64=1", nil))
	defer ctx.Flush()
	j := NewJSONP(ctx).(*jsonp)
	p := j.Polling.(*polling)
	defer func() { j.Discard(); j.Close() }()
	j.OnRequest(ctx)
	j.Send([]*packet.Packet{{Type: packet.MESSAGE, Data: strings.NewReader("hello")}})
	p.writeQueue.Close()
	p.reqMu.Lock()
	pending := p.req != nil
	p.reqMu.Unlock()
	if !ctx.IsDone() || pending || recorder.Body.String() != `___eio[0]("6:4hello");` {
		t.Fatalf("JSONP response = %q, pending request = %v", recorder.Body.String(), pending)
	}
}

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
