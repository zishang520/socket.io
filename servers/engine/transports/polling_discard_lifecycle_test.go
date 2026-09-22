package transports

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type delayedPollingDiscardCheck struct {
	Transport
	paused  atomic.Bool
	entered chan struct{}
	resume  <-chan struct{}
}

func (t *delayedPollingDiscardCheck) Discarded() bool {
	// Pause after publishing writable, allowing the response and a successor
	// GET to finish registering before this request's discard check returns.
	if t.Writable() && t.paused.CompareAndSwap(false, true) {
		close(t.entered)
		<-t.resume
	}
	return t.Transport.Discarded()
}

func TestPollingLateDiscardCheckPreservesSuccessor(t *testing.T) {
	firstRecorder := httptest.NewRecorder()
	first := types.NewHttpContext(firstRecorder, httptest.NewRequest(http.MethodGet, "/?EIO=4", nil))
	t.Cleanup(first.Flush)
	p := NewPolling(first).(*polling)
	resume := make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	paused := &delayedPollingDiscardCheck{
		Transport: p.Transport,
		entered:   make(chan struct{}),
		resume:    resume,
	}
	p.Transport = paused
	t.Cleanup(func() {
		release()
		p.Discard()
		p.Close()
		p.writeQueue.Close()
	})
	registered := make(chan struct{})
	go func() {
		p.OnRequest(first)
		close(registered)
	}()
	select {
	case <-paused.entered:
	case <-time.After(time.Second):
		t.Fatal("first GET did not reach its final discard check")
	}
	p.Send([]*packet.Packet{{Type: packet.MESSAGE, Data: types.NewStringBufferString("first")}})
	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Fatal("first GET did not receive its response")
	}
	if got := firstRecorder.Body.String(); got != "4first" {
		t.Fatalf("first response = %q, want 4first", got)
	}

	secondRecorder := httptest.NewRecorder()
	second := types.NewHttpContext(secondRecorder, httptest.NewRequest(http.MethodGet, "/?EIO=4", nil))
	t.Cleanup(second.Flush)
	p.OnRequest(second)
	if second.IsDone() || !p.Writable() {
		t.Fatal("successor GET did not become pending")
	}
	p.Discard()
	release()
	select {
	case <-registered:
	case <-time.After(time.Second):
		t.Fatal("first GET registration did not return")
	}
	p.Close()
	select {
	case <-second.Done():
	case <-time.After(time.Second):
		t.Fatal("late discard check prevented the successor GET from receiving CLOSE")
	}
	if secondRecorder.Code != http.StatusOK || secondRecorder.Body.String() != "1" {
		t.Fatalf("successor response = (%d, %q), want (200, CLOSE)", secondRecorder.Code, secondRecorder.Body.String())
	}
	// Done precedes DoWrite's cleanup callback; wait for the write task to finish.
	p.writeQueue.Close()
	p.reqMu.Lock()
	pending := p.req != nil
	p.reqMu.Unlock()
	if pending {
		t.Fatal("closed transport retained its successor GET")
	}
}
