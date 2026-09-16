package transports

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
			case <-failure:
			case <-time.After(time.Second):
				t.Fatal("canceled GET did not report premature close")
			}
			if p.req.Load() != nil || p.Writable() {
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
	if !ctx.IsDone() || recorder.Code != http.StatusBadRequest || p.req.Load() != nil || p.Writable() {
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
	if !duplicate.IsDone() || recorder.Code != http.StatusBadRequest || p.req.Load() != first {
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
	if !ctx.IsDone() || p.req.Load() != nil || recorder.Body.String() != `___eio[0]("6:4hello");` {
		t.Fatalf("JSONP response = %q, pending request = %v", recorder.Body.String(), p.req.Load() != nil)
	}
}
