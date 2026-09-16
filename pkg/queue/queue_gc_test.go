package queue_test

import (
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/zishang520/socket.io/v3/pkg/queue"
)

//go:noinline
func abandonedQueue(runTask bool) weak.Pointer[queue.Queue] {
	q := queue.New()
	if runTask {
		done := make(chan struct{})
		q.Enqueue(func() { close(done) })
		<-done
	}
	return weak.Make(q)
}

func TestIdleQueueCanBeCollectedWithoutClose(t *testing.T) {
	for _, runTask := range []bool{false, true} {
		name := "unused"
		if runTask {
			name = "drained"
		}
		t.Run(name, func(t *testing.T) {
			ref := abandonedQueue(runTask)
			t.Cleanup(func() {
				if q := ref.Value(); q != nil {
					q.Close()
				}
			})
			deadline := time.Now().Add(3 * time.Second)
			for {
				runtime.GC()
				if ref.Value() == nil {
					return
				}
				if time.Now().After(deadline) {
					t.Fatal("idle queue is still retained after its owner released it")
				}
				time.Sleep(10 * time.Millisecond)
			}
		})
	}
}
