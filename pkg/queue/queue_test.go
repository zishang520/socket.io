package queue

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestQueue_FIFOOrder(t *testing.T) {
	q := New()
	defer q.Close()

	var mu sync.Mutex
	results := make([]int, 0, 100)

	var wg sync.WaitGroup
	wg.Add(1)

	for i := range 100 {
		v := i
		q.Enqueue(func() {
			mu.Lock()
			results = append(results, v)
			mu.Unlock()
			if v == 99 {
				wg.Done()
			}
		})
	}

	wg.Wait()

	if len(results) != 100 {
		t.Fatalf("expected 100 results, got %d", len(results))
	}
	for i, v := range results {
		if v != i {
			t.Fatalf("expected results[%d] = %d, got %d", i, i, v)
		}
	}
}

func TestQueue_ConcurrentEnqueue(t *testing.T) {
	q := New()
	defer q.Close()

	var counter atomic.Int64
	var wg sync.WaitGroup

	n := 100
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			for range 100 {
				q.Enqueue(func() {
					counter.Add(1)
				})
			}
		}()
	}

	wg.Wait()

	// Give time for tasks to drain
	done := make(chan struct{})
	q.Enqueue(func() { close(done) })
	<-done

	if got := counter.Load(); got != int64(n*100) {
		t.Fatalf("expected %d, got %d", n*100, got)
	}
}

func TestQueue_CloseWaitsForCompletion(t *testing.T) {
	q := New()

	var completed atomic.Bool
	q.Enqueue(func() {
		completed.Store(true)
	})

	q.Close()

	if !completed.Load() {
		t.Fatal("expected task to complete before Close returns")
	}
}

func TestQueue_EnqueueAfterClose(t *testing.T) {
	q := New()
	q.Close()

	// Should not panic
	q.Enqueue(func() {
		t.Fatal("this should not execute")
	})
}

func TestQueue_DoubleClose(t *testing.T) {
	q := New()
	q.Close()
	q.Close() // should not panic
}

func TestQueue_Size(t *testing.T) {
	q := New()
	defer q.Close()

	// Block the consumer so pending tasks accumulate
	blocker := make(chan struct{})
	defer close(blocker)
	started := make(chan struct{})
	q.Enqueue(func() {
		close(started)
		<-blocker
	})
	<-started // wait for the blocker task to be consumed

	for range 10 {
		q.Enqueue(func() {})
	}

	if got := q.Size(); got != 10 {
		t.Fatalf("expected size 10, got %d", got)
	}
}
