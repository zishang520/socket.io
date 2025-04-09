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
		go t.fn()
	}

	return t
}

func (t *Timer) Unref() {
	runtime.AddCleanup(t, func(t *Timer) {
		if t.timer.Stop() {
			close(t.stopCh)
		}
	}, t)
}

// Deprecated: this method will be removed in the next major release, please use SetTimeout instead.
func SetTimeOut(fn func(), sleep time.Duration) *Timer {
	return SetTimeout(fn, sleep)
}

func SetTimeout(fn func(), sleep time.Duration) *Timer {
	timer := &Timer{
		timer:  time.NewTimer(sleep),
		sleep:  sleep,
		stopCh: make(chan struct{}),
	}
	timer.fn = func() {
		select {
		case <-timer.timer.C:
			fn()
		case <-timer.stopCh:
			return
		}
	}
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
		t.stopCh <- struct{}{}
	}
}

func SetInterval(fn func(), sleep time.Duration) *Timer {
	timer := &Timer{
		timer:  time.NewTimer(sleep),
		sleep:  sleep,
		stopCh: make(chan struct{}),
	}
	timer.fn = func() {
		for {
			select {
			case <-timer.timer.C:
				timer.timer.Reset(timer.sleep)
				go fn()
			case <-timer.stopCh:
				return
			}
		}
	}
	go timer.fn()
	return timer
}

func ClearInterval(timer *Timer) {
	ClearTimeout(timer)
}
