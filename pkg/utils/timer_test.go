package utils

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestSetTimeout(t *testing.T) {
	called := int32(0)

	timer := SetTimeout(func() {
		atomic.AddInt32(&called, 1)
	}, 50*time.Millisecond)

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("Expected timeout to be called once, got %d", called)
	}

	_ = timer
}

func TestSetTimeoutStop(t *testing.T) {
	called := int32(0)

	timer := SetTimeout(func() {
		atomic.AddInt32(&called, 1)
	}, 100*time.Millisecond)

	// Stop before timeout
	timer.Stop()
	time.Sleep(150 * time.Millisecond)

	if atomic.LoadInt32(&called) != 0 {
		t.Errorf("Expected timeout not to be called after stop, got %d", called)
	}
}

func TestClearTimeout(t *testing.T) {
	called := int32(0)

	timer := SetTimeout(func() {
		atomic.AddInt32(&called, 1)
	}, 100*time.Millisecond)

	// Clear timeout
	ClearTimeout(timer)
	time.Sleep(150 * time.Millisecond)

	if atomic.LoadInt32(&called) != 0 {
		t.Errorf("Expected timeout not to be called after clearTimeout, got %d", called)
	}
}

func TestClearTimeoutNil(t *testing.T) {
	// Should not panic
	ClearTimeout(nil)
}

func TestTimerRefresh(t *testing.T) {
	called := int32(0)

	timer := SetTimeout(func() {
		atomic.AddInt32(&called, 1)
	}, 50*time.Millisecond)

	// Refresh before timeout
	time.Sleep(25 * time.Millisecond)
	timer.Refresh()

	// Should not have fired yet
	if atomic.LoadInt32(&called) > 0 {
		t.Errorf("Timeout should not have fired yet")
	}

	// Wait for refreshed timeout
	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("Expected timeout to fire after refresh, got %d", called)
	}
}

func TestSetInterval(t *testing.T) {
	called := int32(0)

	timer := SetInterval(func() {
		atomic.AddInt32(&called, 1)
	}, 30*time.Millisecond)

	// Wait for multiple intervals
	time.Sleep(100 * time.Millisecond)

	count := atomic.LoadInt32(&called)
	if count < 2 {
		t.Errorf("Expected interval to fire at least twice, got %d", count)
	}

	// Stop interval
	timer.Stop()
	time.Sleep(50 * time.Millisecond)

	finalCount := atomic.LoadInt32(&called)
	if finalCount-count > 1 {
		t.Errorf("Interval should have stopped, but fired %d more times", finalCount-count)
	}
}

func TestClearInterval(t *testing.T) {
	called := int32(0)

	timer := SetInterval(func() {
		atomic.AddInt32(&called, 1)
	}, 30*time.Millisecond)

	// Wait for one interval
	time.Sleep(50 * time.Millisecond)

	count := atomic.LoadInt32(&called)

	// Clear interval
	ClearInterval(timer)
	time.Sleep(100 * time.Millisecond)

	finalCount := atomic.LoadInt32(&called)
	if finalCount-count > 0 {
		t.Errorf("Interval should have stopped after clearInterval, but fired %d more times", finalCount-count)
	}
}

func TestSetTimeoutMultipleStops(t *testing.T) {
	called := int32(0)

	timer := SetTimeout(func() {
		atomic.AddInt32(&called, 1)
	}, 100*time.Millisecond)

	// Multiple stops should not panic
	timer.Stop()
	timer.Stop()
	timer.Stop()

	time.Sleep(150 * time.Millisecond)

	if atomic.LoadInt32(&called) != 0 {
		t.Errorf("Expected timeout not to fire after stop, got %d", called)
	}
}

func TestTimerUnref(t *testing.T) {
	called := int32(0)

	timer := SetTimeout(func() {
		atomic.AddInt32(&called, 1)
	}, 50*time.Millisecond)

	// Unref should not prevent timeout from firing
	timer.Unref()

	time.Sleep(100 * time.Millisecond)

	if atomic.LoadInt32(&called) != 1 {
		t.Errorf("Expected timeout to fire even after Unref, got %d", called)
	}
}
