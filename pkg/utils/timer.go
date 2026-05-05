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
	stopCh := t.stopCh
	runtime.AddCleanup(t, func(timer *time.Timer) {
		if timer.Stop() {
			close(stopCh)
		}
	}, t.timer)
}

func SetTimeout(fn func(), sleep time.Duration) *Timer {
	timer := &Timer{
		timer:  time.NewTimer(sleep),
		sleep:  sleep,
		stopCh: make(chan struct{}, 1),
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
	// Always stop the underlying timer regardless of whether it already fired.
	// In Go 1.23+, time.Timer.Stop() drains the C channel when returning false
	// (timer had already expired). Without the unconditional signal below, the
	// goroutine inside timer.fn would be permanently blocked: it can no longer
	// receive from the now-empty C, and stopCh was never signalled.
	// The buffered channel (cap 1) ensures the signal is queued even if the
	// goroutine hasn't reached its select yet.
	t.timer.Stop()
	select {
	case t.stopCh <- struct{}{}:
	default:
	}
}

func SetInterval(fn func(), sleep time.Duration) *Timer {
	timer := &Timer{
		timer:  time.NewTimer(sleep),
		sleep:  sleep,
		stopCh: make(chan struct{}, 1),
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
