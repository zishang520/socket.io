package socket

import (
	"testing"
	"time"
)

func TestBufferedEventCanInspectReceiveBuffer(t *testing.T) {
	manager := newTestManager()
	t.Cleanup(manager.taskQueue.Close)
	socket := manager.Socket("/buffered", nil)
	t.Cleanup(socket.destroy)
	socket.ReceiveBuffer().Push([]any{"buffered", "payload"})
	remaining := make(chan int, 1)
	var received any
	_ = socket.On("buffered", func(args ...any) {
		received = args[0]
		remaining <- socket.ReceiveBuffer().Len()
		// Data added by a callback belongs to the next batch.
		socket.ReceiveBuffer().Push([]any{"later"})
	})
	completed := make(chan struct{})
	go func() {
		socket.emitBuffered()
		close(completed)
	}()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("buffered event deadlocked when its callback inspected ReceiveBuffer")
	}
	if got := <-remaining; got != 0 {
		t.Fatalf("callback observed %d pending messages, want 0", got)
	}
	if received != "payload" {
		t.Fatalf("received %v, want payload", received)
	}
	if got := socket.ReceiveBuffer().All(); len(got) != 1 || len(got[0]) != 1 || got[0][0] != "later" {
		t.Fatalf("callback's new receive buffer data was lost: %v", got)
	}
}
