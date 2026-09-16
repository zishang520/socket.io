package socket

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSocketClosePacketRacesTransportClose(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	transportClosed := make(chan struct{})
	var disconnecting, disconnected atomic.Int32
	var socket *Socket
	conn := connectAckTestSocket(t, func(s *Socket) {
		socket = s
		_ = s.On("disconnecting", func(...any) {
			if disconnecting.Add(1) == 1 {
				close(entered)
				<-release
			}
		})
		_ = s.On("disconnect", func(...any) { disconnected.Add(1) })
		_ = s.Conn().Once("close", func(...any) { close(transportClosed) })
	})
	t.Cleanup(unblock)

	// A namespace DISCONNECT runs on the socket queue, while a transport close
	// can arrive directly from the Engine.IO connection on another goroutine.
	if err := conn.WriteMessage(websocket.TextMessage, []byte("41")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("namespace disconnect did not reach disconnecting")
	}
	if !socket.Connected() || !socket.Rooms().Has(Room(socket.Id())) {
		t.Fatal("disconnecting must retain connected state and room membership")
	}
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-transportClosed:
	case <-time.After(time.Second):
		t.Fatal("transport close did not return while namespace close was pending")
	}
	unblock()
	socket.taskQueue.Close()

	if got := disconnecting.Load(); got != 1 {
		t.Errorf("disconnecting emitted %d times, want 1", got)
	}
	if got := disconnected.Load(); got != 1 {
		t.Errorf("disconnect emitted %d times, want 1", got)
	}
	if socket.Connected() || socket.Rooms().Len() != 0 {
		t.Fatal("socket was not fully cleaned up")
	}
}

func TestSocketCloseCanReenterFromDisconnecting(t *testing.T) {
	var disconnecting, disconnected atomic.Int32
	var socket *Socket
	connectAckTestSocket(t, func(s *Socket) {
		socket = s
		_ = s.On("disconnecting", func(...any) {
			if disconnecting.Add(1) == 1 {
				s._onclose("transport close")
			}
		})
		_ = s.On("disconnect", func(...any) { disconnected.Add(1) })
	})
	returned := make(chan struct{})
	go func() {
		socket._onclose("server namespace disconnect")
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("close reentered from disconnecting deadlocked")
	}
	if got := disconnecting.Load(); got != 1 {
		t.Errorf("disconnecting emitted %d times, want 1", got)
	}
	if got := disconnected.Load(); got != 1 {
		t.Errorf("disconnect emitted %d times, want 1", got)
	}
}
