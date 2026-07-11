// Package queue provides a sequential, non-blocking, unbounded task execution queue.
// It mimics Node.js's event loop and is deeply inspired by Kubernetes workqueue.
package queue

import (
	"runtime"
	"runtime/debug"
	"sync"

	"github.com/zishang520/socket.io/v3/pkg/log"
)

var queueLog = log.NewLog("engine:events")

const initialQueueCapacity = 1024

// Queue serializes function execution through a single goroutine.
// It uses an unbounded slice backed by a condition variable to ensure
// Enqueue never blocks the caller.
type Queue struct {
	mu           sync.Mutex
	cond         *sync.Cond
	tasks        []func()
	head         int
	size         int
	shuttingDown bool
	done         chan struct{}
}

// New creates a new Queue and starts the internal consumer goroutine.
func New() *Queue {
	q := &Queue{
		tasks: make([]func(), initialQueueCapacity),
		done:  make(chan struct{}),
	}
	q.cond = sync.NewCond(&q.mu)

	go q.loop()
	runtime.SetFinalizer(q, func(q *Queue) { q.TryClose() })
	return q
}

// Enqueue adds a task to the queue for sequential execution.
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

	q.push(task)
	q.cond.Signal()
}

// Size returns the number of pending tasks in the queue.
func (q *Queue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.size
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

	for q.size == 0 && !q.shuttingDown {
		q.cond.Wait()
	}

	if q.size == 0 && q.shuttingDown {
		return nil, false
	}

	return q.pop(), true
}

func (q *Queue) push(task func()) {
	if q.size == len(q.tasks) {
		q.grow()
	}

	tail := (q.head + q.size) % len(q.tasks)
	q.tasks[tail] = task
	q.size++
}

func (q *Queue) pop() func() {
	task := q.tasks[q.head]
	q.tasks[q.head] = nil
	q.head = (q.head + 1) % len(q.tasks)
	q.size--

	if q.size == 0 {
		q.head = 0
		if len(q.tasks) > initialQueueCapacity {
			q.tasks = make([]func(), initialQueueCapacity)
		}
	}

	return task
}

func (q *Queue) grow() {
	next := make([]func(), len(q.tasks)*2)
	for i := range q.size {
		next[i] = q.tasks[(q.head+i)%len(q.tasks)]
	}
	q.tasks = next
	q.head = 0
}

// execute runs the task with built-in panic recovery.
func (q *Queue) execute(task func()) {
	defer func() {
		if r := recover(); r != nil {
			queueLog.Errorf("queue task panic recovered: %v\n%s", r, debug.Stack())
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

// IsShuttingDown reports whether TryClose or Close has been called.
func (q *Queue) IsShuttingDown() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.shuttingDown
}

// TryClose shuts down the Queue without waiting for completion.
func (q *Queue) TryClose() {
	q.mu.Lock()
	q.shuttingDown = true
	q.cond.Broadcast()
	q.mu.Unlock()
}
