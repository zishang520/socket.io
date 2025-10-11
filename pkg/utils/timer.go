package utils

import (
	"runtime"
	"time"
)

type Timer struct {
	timer  *time.Timer
	sleep  time.Duration
	fn     func()
	stopCh chan struct{}
}

func (t *Timer) Refresh() *Timer {
	defer t.timer.Reset(t.sleep)

	if !t.timer.Stop() {
		// Idempotent repeated calls
		go t.fn()
	}

	return t
}

func (t *Timer) Unref() {
	runtime.AddCleanup(t, func(timer *time.Timer) {
		if timer.Stop() {
			close(t.stopCh)
		}
	}, t.timer)
}

func SetTimeout(fn func(), sleep time.Duration) *Timer {
	timer := &Timer{
		timer:  time.NewTimer(sleep),
		sleep:  sleep,
		stopCh: make(chan struct{}),
	}
	timer.fn = func() {
		defer func() {
			// Ensure channel is drained to prevent leaks
			select {
			case <-timer.stopCh:
			default:
			}
		}()

		select {
		case <-timer.timer.C:
			fn()
		case <-timer.stopCh:
			return
		}
	}
	// Idempotent repeated calls
	go timer.fn()
	return timer
}

func ClearTimeout(timer *Timer) {
	if timer != nil {
		timer.Stop()
	}
}

func (t *Timer) Stop() {
	if t.timer.Stop() {
		// Use non-blocking send to avoid goroutine leak if no reader
		select {
		case t.stopCh <- struct{}{}:
		default:
			// Channel is full or no reader, timer already stopped
		}
	}
}

func SetInterval(fn func(), sleep time.Duration) *Timer {
	timer := &Timer{
		timer:  time.NewTimer(sleep),
		sleep:  sleep,
		stopCh: make(chan struct{}),
	}
	timer.fn = func() {
		defer func() {
			// Ensure channel is drained to prevent leaks
			select {
			case <-timer.stopCh:
			default:
			}
		}()

		for {
			select {
			case <-timer.timer.C:
				timer.timer.Reset(timer.sleep)
				// Idempotent repeated calls
				go fn()
			case <-timer.stopCh:
				return
			}
		}
	}
	// Idempotent repeated calls
	go timer.fn()
	return timer
}

func ClearInterval(timer *Timer) {
	ClearTimeout(timer)
}
