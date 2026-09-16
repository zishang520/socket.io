// Package queue provides a sequential, non-blocking, unbounded task execution queue.
// It mimics Node.js's event loop and is deeply inspired by Kubernetes workqueue.
package queue

import (
	"runtime/debug"
	"sync"

	"github.com/zishang520/socket.io/v3/pkg/log"
)

var queueLog = log.NewLog("engine:events")

// Queue serializes function execution through at most one worker goroutine.
// The worker is started on demand and exits when the queue becomes idle.
type Queue struct {
	mu           sync.Mutex
	tasks        []func()
	running      bool
	shuttingDown bool
	done         chan struct{}
}

// New creates a new Queue. Its worker starts with the first task.
func New() *Queue {
	return &Queue{done: make(chan struct{})}
}

// Enqueue adds a task to the queue for sequential execution.
// It returns immediately and NEVER blocks.
func (q *Queue) Enqueue(task func()) {
	if task == nil {
		return
	}

	q.mu.Lock()
	if q.shuttingDown {
		q.mu.Unlock()
		return
	}

	if !q.running {
		q.running = true
		q.mu.Unlock()
		go q.loop(task)
		return
	}

	q.tasks = append(q.tasks, task)
	q.mu.Unlock()
}

// Size returns the number of pending tasks in the queue.
func (q *Queue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tasks)
}

// loop drains the queue and exits as soon as it becomes idle.
func (q *Queue) loop(task func()) {
	for {
		q.execute(task)

		q.mu.Lock()
		if len(q.tasks) == 0 {
			q.tasks = nil
			q.running = false
			if q.shuttingDown {
				close(q.done)
			}
			q.mu.Unlock()
			return
		}

		task = q.tasks[0]
		q.tasks[0] = nil
		q.tasks = q.tasks[1:]
		q.mu.Unlock()
	}
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
	q.shutdown()
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
	q.shutdown()
}

func (q *Queue) shutdown() {
	q.mu.Lock()
	if !q.shuttingDown {
		q.shuttingDown = true
		if !q.running {
			q.tasks = nil
			close(q.done)
		}
	}
	q.mu.Unlock()
}
