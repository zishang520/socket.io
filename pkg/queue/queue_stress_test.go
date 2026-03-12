package queue

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// BenchmarkQueue_Enqueue_SingleProducer measures the throughput of Enqueue
// from a single producer goroutine.
func BenchmarkQueue_Enqueue_SingleProducer(b *testing.B) {
	q := New() // Ensure buffer can hold all tasks to avoid blocking the test runner
	defer q.Close()

	var counter uint64
	task := func() {
		atomic.AddUint64(&counter, 1)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(task)
	}
	// Note: this only measures Enqueue time, not the processing time.
}

// BenchmarkQueue_Processing_Throughput measures the total throughput
// (Enqueue + processing) from a single producer.
func BenchmarkQueue_Processing_Throughput(b *testing.B) {
	q := New()

	var wg sync.WaitGroup
	wg.Add(b.N)

	task := func() {
		wg.Done()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		q.Enqueue(task)
	}
	wg.Wait() // Wait for all tasks to be processed
	q.TryClose()
}

// BenchmarkQueue_Concurrent_Producers measures the total throughput
// (Enqueue + processing) from multiple concurrent producers.
func BenchmarkQueue_Concurrent_Producers(b *testing.B) {
	q := New()

	var wg sync.WaitGroup
	wg.Add(b.N)

	task := func() {
		wg.Done()
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.Enqueue(task)
		}
	})
	wg.Wait()
	q.TryClose()
}

// TestQueue_Stress tests the queue under high concurrent load to ensure
// no tasks are lost and race conditions don't occur.
func TestQueue_Stress(t *testing.T) {
	q := New()

	numProducers := 100
	tasksPerProducer := 10000
	totalTasks := numProducers * tasksPerProducer

	var processedCount uint64
	var wg sync.WaitGroup
	wg.Add(totalTasks)

	task := func() {
		atomic.AddUint64(&processedCount, 1)
		wg.Done()
	}

	// Start producers
	for range numProducers {
		go func() {
			for range tasksPerProducer {
				q.Enqueue(task)
			}
		}()
	}

	wg.Wait() // Wait for all tasks to be processed
	q.Close()

	if finalCount := atomic.LoadUint64(&processedCount); finalCount != uint64(totalTasks) {
		t.Errorf("expected %d tasks to be processed, got %d", totalTasks, finalCount)
	}
}

// BenchmarkQueue_HeavyTasks measures throughput when the task itself takes some time.
func BenchmarkQueue_HeavyTasks(b *testing.B) {
	q := New()

	var wg sync.WaitGroup
	wg.Add(b.N)

	task := func() {
		// Simulate some CPU work
		for range 1000 {
			runtime.Gosched()
		}
		wg.Done()
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			q.Enqueue(task)
		}
	})
	wg.Wait()
	q.TryClose()
}
