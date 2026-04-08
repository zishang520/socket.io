// Package queue provides a sequential, non-blocking, unbounded task execution queue.
// It mimics Node.js's event loop and is deeply inspired by Kubernetes workqueue.
package queue

import (
	"log"
	"runtime"
	"sync"
)

// Queue serializes function execution through a single goroutine.
// It uses an unbounded slice backed by a condition variable to ensure
// Enqueue never blocks the caller.
type Queue struct {
	mu           sync.Mutex
	cond         *sync.Cond
	tasks        []func()
	shuttingDown bool
	done         chan struct{}
}

// New creates a new Queue and starts the internal consumer goroutine.
func New() *Queue {
	q := &Queue{
		tasks: make([]func(), 0, 1024),
		done:  make(chan struct{}),
	}
	q.cond = sync.NewCond(&q.mu)

	go q.loop()
	runtime.SetFinalizer(q, func(q *Queue) { q.TryClose() })
	return q
}

// Enqueue adds a task to the queue for sequential execution.
// It returns nil on success, or ErrQueueFull if a max size is set and the queue is at capacity.
// It returns immediately and NEVER blocks.
func (q *Queue) Enqueue(task func()) {
	if task == nil {
		return
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if q.shuttingDown {
		return
	}

	q.tasks = append(q.tasks, task)
	q.cond.Signal()
}

// Size returns the number of pending tasks in the queue.
func (q *Queue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tasks)
}

// loop is the main consumer goroutine.
func (q *Queue) loop() {
	defer close(q.done)

	for {
		task, ok := q.get()
		if !ok {
			// Queue is empty and shutting down
			return
		}
		q.execute(task)
	}
}

// get safely retrieves the next task from the queue, blocking if necessary.
func (q *Queue) get() (func(), bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for len(q.tasks) == 0 && !q.shuttingDown {
		q.cond.Wait()
	}

	if len(q.tasks) == 0 && q.shuttingDown {
		return nil, false
	}

	task := q.tasks[0]

	q.tasks[0] = nil
	q.tasks = q.tasks[1:]

	return task, true
}

// execute runs the task with built-in panic recovery.
func (q *Queue) execute(task func()) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("queue task panic recovered: %v", err)
		}
	}()
	task()
}

// Close shuts down the Queue gracefully.
// It waits for all previously enqueued tasks to complete before returning.
func (q *Queue) Close() {
	q.mu.Lock()
	q.shuttingDown = true
	q.cond.Broadcast()
	q.mu.Unlock()

	<-q.done
}

// TryClose shuts down the Queue without waiting for completion.
func (q *Queue) TryClose() {
	q.mu.Lock()
	q.shuttingDown = true
	q.cond.Broadcast()
	q.mu.Unlock()
}
