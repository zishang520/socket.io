package utils

import (
	"sync"
	"time"
)

const (
	minTimerDuration = time.Millisecond
	maxTimerDuration = time.Duration(1<<31-1) * time.Millisecond
)

type Timer struct {
	mu sync.Mutex

	timer    *time.Timer
	callback func()
	delay    time.Duration
	deadline time.Time
	interval bool
	running  bool
	stopped  bool
}

func newTimer(callback func(), delay time.Duration, interval bool) *Timer {
	if delay < minTimerDuration || delay > maxTimerDuration {
		delay = minTimerDuration
	} else {
		delay = delay.Truncate(time.Millisecond)
	}

	timer := &Timer{
		callback: callback,
		delay:    delay,
		interval: interval,
	}
	timer.mu.Lock()
	timer.deadline = time.Now().Add(delay)
	timer.timer = time.AfterFunc(delay, timer.run)
	timer.mu.Unlock()
	return timer
}

func (t *Timer) run() {
	t.mu.Lock()
	if t.stopped || t.running {
		t.mu.Unlock()
		return
	}
	if delay := time.Until(t.deadline); delay > 0 {
		t.timer.Reset(delay)
		t.mu.Unlock()
		return
	}

	t.running = true
	callback := t.callback
	deadline := t.deadline
	started := time.Now()
	t.mu.Unlock()

	callback()

	t.mu.Lock()
	t.running = false
	reset := false
	if !t.stopped {
		if t.interval {
			t.deadline = started.Add(t.delay)
			reset = true
		} else if t.deadline.After(deadline) {
			reset = true
		}
	}
	if reset {
		t.timer.Reset(max(time.Until(t.deadline), 0))
	}
	t.mu.Unlock()
}

func (t *Timer) Refresh() *Timer {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.stopped {
		t.deadline = time.Now().Add(t.delay)
		t.timer.Reset(t.delay)
	}
	return t
}

func (*Timer) Unref() {
	// Go timers do not keep the process alive, so no action is required.
}

func (t *Timer) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.stopped {
		t.stopped = true
		t.callback = nil
		t.timer.Stop()
	}
}

func SetTimeout(callback func(), delay time.Duration) *Timer {
	return newTimer(callback, delay, false)
}

func ClearTimeout(timer *Timer) {
	if timer != nil {
		timer.Stop()
	}
}

func SetInterval(callback func(), delay time.Duration) *Timer {
	return newTimer(callback, delay, true)
}

func ClearInterval(timer *Timer) {
	ClearTimeout(timer)
}
