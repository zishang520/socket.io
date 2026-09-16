package queue

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestQueue_EnqueueNil(t *testing.T) {
	q := New()
	defer q.Close()

	// Enqueue nil should be silently ignored
	q.Enqueue(nil)

	// Enqueue a real task to verify queue still works
	done := make(chan struct{})
	q.Enqueue(func() { close(done) })

	select {
	case <-done:
		// success
	case <-time.After(time.Second):
		t.Fatal("Queue stuck after enqueueing nil")
	}
}

func TestQueue_PanicRecovery(t *testing.T) {
	q := New()
	defer q.Close()

	// Enqueue a panicking task
	q.Enqueue(func() {
		panic("test panic")
	})

	// Enqueue a normal task after the panic to verify queue continues
	done := make(chan struct{})
	q.Enqueue(func() {
		close(done)
	})

	select {
	case <-done:
		// Queue recovered from panic and processed next task
	case <-time.After(2 * time.Second):
		t.Fatal("Queue did not recover from panic")
	}
}

func TestQueue_ReleasesTasksOnClose(t *testing.T) {
	q := New()

	const pendingTasks = 2048
	var executed atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	q.Enqueue(func() {
		close(started)
		<-release
		executed.Add(1)
	})
	<-started

	for range pendingTasks {
		q.Enqueue(func() {
			executed.Add(1)
		})
	}

	closed := make(chan struct{})
	go func() {
		q.Close()
		close(closed)
	}()

	deadline := time.Now().Add(time.Second)
	for !q.IsShuttingDown() {
		if time.Now().After(deadline) {
			t.Fatal("Queue did not start shutting down")
		}
		runtime.Gosched()
	}
	select {
	case <-closed:
		t.Fatal("Close returned before queued tasks completed")
	default:
	}

	close(release)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Queue did not close")
	}

	if got, want := executed.Load(), int32(pendingTasks+1); got != want {
		t.Fatalf("Queue executed %d tasks, want %d", got, want)
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.tasks != nil {
		t.Fatalf("Queue retained task storage after close: len=%d cap=%d", len(q.tasks), cap(q.tasks))
	}
}

func TestQueue_ReleasesTasksWhenIdle(t *testing.T) {
	q := New()

	const taskCount = 2048
	var executed atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	q.Enqueue(func() {
		close(started)
		<-release
	})
	<-started

	drained := make(chan struct{})
	for range taskCount {
		q.Enqueue(func() {
			executed.Add(1)
		})
	}
	q.Enqueue(func() {
		close(drained)
	})
	close(release)

	select {
	case <-drained:
	case <-time.After(time.Second):
		t.Fatal("Queue did not drain")
	}

	waitForQueueIdle(t, q)

	if got, want := executed.Load(), int32(taskCount); got != want {
		t.Fatalf("Queue executed %d tasks, want %d", got, want)
	}

	restarted := make(chan struct{})
	q.Enqueue(func() {
		close(restarted)
	})
	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("Queue did not restart after becoming idle")
	}
	q.Close()
}

func TestQueue_TryCloseDrainsAndReleasesTasks(t *testing.T) {
	q := New()

	const pendingTasks = 2048
	var executed atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	q.Enqueue(func() {
		close(started)
		<-release
		executed.Add(1)
	})
	<-started

	for range pendingTasks {
		q.Enqueue(func() {
			executed.Add(1)
		})
	}

	returned := make(chan struct{})
	go func() {
		q.TryClose()
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("TryClose blocked while a task was running")
	}
	if !q.IsShuttingDown() {
		t.Fatal("Queue is not shutting down after TryClose")
	}

	q.Enqueue(func() {
		executed.Add(1)
	})

	close(release)
	select {
	case <-q.done:
	case <-time.After(time.Second):
		t.Fatal("Queue did not shut down")
	}

	if got, want := executed.Load(), int32(pendingTasks+1); got != want {
		t.Fatalf("Queue executed %d tasks, want %d", got, want)
	}

	q.mu.Lock()
	defer q.mu.Unlock()
	if q.tasks != nil {
		t.Fatalf("Queue retained task storage after close: len=%d cap=%d", len(q.tasks), cap(q.tasks))
	}
}

func TestQueue_SizeEmpty(t *testing.T) {
	q := New()
	defer q.Close()

	if got := q.Size(); got != 0 {
		t.Errorf("Empty queue Size() = %d, want 0", got)
	}
}

func TestQueue_SizeAfterConsumption(t *testing.T) {
	q := New()
	defer q.Close()

	done := make(chan struct{})
	q.Enqueue(func() {
		close(done)
	})

	<-done
	waitForQueueIdle(t, q)
	if got := q.Size(); got != 0 {
		t.Errorf("Size() after consumption = %d, want 0", got)
	}
}

func waitForQueueIdle(t *testing.T, q *Queue) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for {
		q.mu.Lock()
		running := q.running
		tasksNil := q.tasks == nil
		q.mu.Unlock()
		if !running {
			if !tasksNil {
				t.Fatal("Queue retained task storage while idle")
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("Queue worker did not stop when idle")
		}
		runtime.Gosched()
	}
}

func TestQueue_MultiplePanics(t *testing.T) {
	q := New()
	defer q.Close()

	// Multiple panics shouldn't break the queue
	for i := range 5 {
		q.Enqueue(func() {
			panic("panic " + string(rune('0'+i)))
		})
	}

	done := make(chan struct{})
	q.Enqueue(func() {
		close(done)
	})

	select {
	case <-done:
		// Queue survived multiple panics
	case <-time.After(2 * time.Second):
		t.Fatal("Queue did not survive multiple panics")
	}
}
