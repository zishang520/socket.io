package socket

import (
	"io"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zishang520/socket.io/clients/engine/v3"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

type closeTestEngine struct {
	Engine
	write func()
}

func (e *closeTestEngine) Write(io.Reader, *packet.Options, func()) engine.SocketWithoutUpgrade {
	if e.write != nil {
		e.write()
	}
	return e
}

func (e *closeTestEngine) Close() engine.SocketWithoutUpgrade { return e }

func TestSocketDisconnectConcurrentAndReentrant(t *testing.T) {
	m := newTestManager()
	t.Cleanup(m.taskQueue.Close)
	s := m.Socket("/", nil)
	s.subEvents()
	s.onconnect("first", "")
	entered, release := make(chan struct{}), make(chan struct{})
	var writes, notifications atomic.Int32
	e := &closeTestEngine{write: func() {
		if writes.Add(1) == 1 {
			close(entered)
			<-release
			s.Disconnect() // a reentrant error/encoding callback must also return
		}
	}}
	m.engine.Store(new(Engine(e)))
	_ = s.On("disconnect", func(...any) { notifications.Add(1) })
	firstDone := make(chan struct{})
	go func() { s.Disconnect(); close(firstDone) }()
	<-entered
	secondDone := make(chan struct{})
	go func() { s.Disconnect(); close(secondDone) }()
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("concurrent Disconnect blocked")
	}
	// A transport close arriving while the manual close is pending must not
	// emit a second notification or take over the first caller's cleanup.
	s.onclose("transport close", nil)
	close(release)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("reentrant Disconnect deadlocked")
	}
	if writes.Load() != 1 || notifications.Load() != 1 || s.Connected() {
		t.Fatalf("writes=%d disconnects=%d connected=%v", writes.Load(), notifications.Load(), s.Connected())
	}
	// The same namespace Socket may reconnect and then disconnect again.
	s.subEvents()
	s.onconnect("second", "")
	s.Disconnect()
	if writes.Load() != 2 || notifications.Load() != 2 || s.Active() {
		t.Fatalf("after reconnect: writes=%d disconnects=%d active=%v", writes.Load(), notifications.Load(), s.Active())
	}
}

func TestSocketAckRacesDisconnectCleanup(t *testing.T) {
	for iteration := range 100 {
		s := MakeSocket()
		var calls atomic.Int32
		s.acks.Store(1, func([]any, error) { calls.Add(1) })
		locked, release, writerDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
		go func() {
			s.sendBuffer.DoWrite(func(v []*Packet) []*Packet { close(locked); <-release; return v })
			close(writerDone)
		}()
		<-locked
		started, cleared := make(chan struct{}), make(chan struct{})
		go func() { close(started); s._clearAcks(); close(cleared) }()
		<-started
		// Let cleanup reach the buffer lock before delivering the competing ACK.
		runtime.Gosched()
		s.onack(&parser.Packet{Id: new(uint64(1)), Data: []any{"ok"}})
		close(release)
		<-cleared
		<-writerDone
		if got := calls.Load(); got != 1 {
			t.Fatalf("iteration %d: ACK callback invoked %d times, want 1", iteration, got)
		}
	}
}

func TestSocketDisconnectCleanupPreservesBufferedAck(t *testing.T) {
	s := MakeSocket()
	s.sendBuffer.Push(&Packet{Packet: &parser.Packet{Id: new(uint64(1))}})
	calls := 0
	s.acks.Store(1, func([]any, error) { calls++ })
	s._clearAcks()
	if calls != 0 || s.acks.Len() != 1 {
		t.Fatal("cleanup discarded an unsent packet's ACK")
	}
	s.onack(&parser.Packet{Id: new(uint64(1)), Data: []any{"ok"}})
	if calls != 1 || s.acks.Len() != 0 {
		t.Fatal("buffered ACK was not available for later delivery")
	}
}

func TestSocketServerDisconnectStopsReconnectAfterTransportClose(t *testing.T) {
	m := newTestManager()
	t.Cleanup(m.taskQueue.Close)
	m.engine.Store(new(Engine(&closeTestEngine{})))
	s := m.Socket("/", nil)
	s.subEvents()
	s.onconnect("connected", "")
	notifications := 0
	_ = s.On("disconnect", func(...any) { notifications++ })
	s.onclose("transport close", nil)
	if !s.Active() {
		t.Fatal("transport close must retain subscriptions for reconnection")
	}
	s.ondisconnect()
	if s.Active() || !m.skipReconnect.Load() || notifications != 1 {
		t.Fatalf("active=%v skipReconnect=%v disconnects=%d", s.Active(), m.skipReconnect.Load(), notifications)
	}
}
