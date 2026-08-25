package utils

import (
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func advanceTime(delay time.Duration) {
	time.Sleep(delay)
	synctest.Wait()
}

func assertCalls(t *testing.T, calls *atomic.Int32, want int32) {
	t.Helper()
	if got := calls.Load(); got != want {
		t.Fatalf("calls = %d, want %d", got, want)
	}
}

func TestTimerDelayNormalization(t *testing.T) {
	tests := []struct {
		name  string
		delay time.Duration
		want  time.Duration
	}{
		{name: "negative", delay: -time.Second, want: minTimerDuration},
		{name: "zero", want: minTimerDuration},
		{name: "below minimum", delay: time.Microsecond, want: minTimerDuration},
		{name: "fractional millisecond", delay: 1_500 * time.Microsecond, want: time.Millisecond},
		{name: "valid", delay: 2 * time.Millisecond, want: 2 * time.Millisecond},
		{name: "maximum", delay: maxTimerDuration, want: maxTimerDuration},
		{name: "above maximum", delay: maxTimerDuration + time.Millisecond, want: minTimerDuration},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var calls atomic.Int32
				timer := SetTimeout(func() {
					calls.Add(1)
				}, test.delay)
				defer timer.Stop()

				advanceTime(test.want - time.Nanosecond)
				assertCalls(t, &calls, 0)

				advanceTime(time.Nanosecond)
				assertCalls(t, &calls, 1)
			})
		})
	}
}

func TestNormalizeTimerMilliseconds(t *testing.T) {
	tests := []struct {
		name  string
		delay int64
		want  time.Duration
	}{
		{name: "negative", delay: -1, want: time.Millisecond},
		{name: "zero", want: time.Millisecond},
		{name: "normal", delay: 25, want: 25 * time.Millisecond},
		{name: "maximum", delay: maxTimerMilliseconds, want: maxTimerDuration},
		{name: "above maximum", delay: maxTimerMilliseconds + 1, want: time.Millisecond},
		{name: "maximum int64", delay: int64(^uint64(0) >> 1), want: time.Millisecond},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeTimerMilliseconds(test.delay); got != test.want {
				t.Fatalf("NormalizeTimerMilliseconds(%d) = %v, want %v", test.delay, got, test.want)
			}
		})
	}

	if allocations := testing.AllocsPerRun(1_000, func() {
		_ = NormalizeTimerMilliseconds(maxTimerMilliseconds)
	}); allocations != 0 {
		t.Fatalf("NormalizeTimerMilliseconds allocations = %v, want 0", allocations)
	}
}

func TestSetTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = time.Second
		var calls atomic.Int32

		timer := SetTimeout(func() {
			calls.Add(1)
		}, delay)
		defer timer.Stop()

		advanceTime(delay - time.Nanosecond)
		assertCalls(t, &calls, 0)

		advanceTime(time.Nanosecond)
		assertCalls(t, &calls, 1)

		advanceTime(delay)
		assertCalls(t, &calls, 1)
	})
}

func TestTimerRefresh(t *testing.T) {
	t.Run("before timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const delay = time.Second
			var calls atomic.Int32

			timer := SetTimeout(func() {
				calls.Add(1)
			}, delay)
			defer timer.Stop()

			time.Sleep(delay / 2)
			if got := timer.Refresh(); got != timer {
				t.Fatal("Refresh() did not return the timer")
			}

			advanceTime(delay - time.Nanosecond)
			assertCalls(t, &calls, 0)

			advanceTime(time.Nanosecond)
			assertCalls(t, &calls, 1)
		})
	})

	t.Run("after timeout", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const delay = time.Second
			var calls atomic.Int32

			timer := SetTimeout(func() {
				calls.Add(1)
			}, delay)
			defer timer.Stop()

			advanceTime(delay)
			assertCalls(t, &calls, 1)

			timer.Refresh()
			advanceTime(delay)
			assertCalls(t, &calls, 2)
		})
	})

	t.Run("from callback", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const delay = time.Second
			var calls atomic.Int32
			var timer atomic.Pointer[Timer]

			timer.Store(SetTimeout(func() {
				if calls.Add(1) == 1 {
					timer.Load().Refresh()
				}
			}, delay))
			defer timer.Load().Stop()

			advanceTime(2 * delay)
			assertCalls(t, &calls, 2)
		})
	})

	t.Run("after timeout and stop", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const delay = time.Second
			var calls atomic.Int32

			timer := SetTimeout(func() {
				calls.Add(1)
			}, delay)
			advanceTime(delay)

			timer.Stop()
			timer.Refresh()
			advanceTime(delay)
			assertCalls(t, &calls, 1)
		})
	})
}

func TestTimerCancellation(t *testing.T) {
	tests := []struct {
		name   string
		cancel func(*Timer)
	}{
		{name: "Stop", cancel: (*Timer).Stop},
		{name: "ClearTimeout", cancel: ClearTimeout},
		{name: "ClearInterval", cancel: ClearInterval},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				const delay = time.Second
				var calls atomic.Int32

				timer := SetTimeout(func() {
					calls.Add(1)
				}, delay)
				test.cancel(timer)
				test.cancel(timer)
				timer.Refresh()

				advanceTime(2 * delay)
				assertCalls(t, &calls, 0)
			})
		})
	}

	ClearTimeout(nil)
	ClearInterval(nil)
}

func TestSetInterval(t *testing.T) {
	t.Run("refresh", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const delay = time.Second
			var calls atomic.Int32

			timer := SetInterval(func() {
				calls.Add(1)
			}, delay)
			defer timer.Stop()

			time.Sleep(delay / 2)
			timer.Refresh()
			advanceTime(delay - time.Nanosecond)
			assertCalls(t, &calls, 0)

			advanceTime(time.Nanosecond)
			assertCalls(t, &calls, 1)

			advanceTime(delay)
			assertCalls(t, &calls, 2)
		})
	})

	t.Run("repeat and clear", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const delay = time.Second
			var calls atomic.Int32

			timer := SetInterval(func() {
				calls.Add(1)
			}, delay)
			defer timer.Stop()

			advanceTime(3 * delay)
			assertCalls(t, &calls, 3)

			ClearInterval(timer)
			ClearInterval(timer)
			advanceTime(2 * delay)
			assertCalls(t, &calls, 3)
		})
	})

	t.Run("stop from callback", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const delay = time.Second
			var calls atomic.Int32
			var timer atomic.Pointer[Timer]

			timer.Store(SetInterval(func() {
				calls.Add(1)
				timer.Load().Stop()
			}, delay))
			defer timer.Load().Stop()

			advanceTime(3 * delay)
			assertCalls(t, &calls, 1)
		})
	})

	t.Run("clear while callback is running", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			const delay = time.Second
			var calls atomic.Int32
			started := make(chan struct{})
			release := make(chan struct{})

			timer := SetInterval(func() {
				if calls.Add(1) == 1 {
					close(started)
					<-release
				}
			}, delay)
			defer timer.Stop()

			advanceTime(delay)
			<-started
			ClearInterval(timer)
			close(release)
			synctest.Wait()

			advanceTime(3 * delay)
			assertCalls(t, &calls, 1)
		})
	})
}

func TestIntervalCallbacksDoNotOverlap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = time.Second
		var active, maxActive, calls atomic.Int32
		started := make(chan struct{})
		release := make(chan struct{})

		timer := SetInterval(func() {
			current := active.Add(1)
			for {
				peak := maxActive.Load()
				if current <= peak || maxActive.CompareAndSwap(peak, current) {
					break
				}
			}
			if calls.Add(1) == 1 {
				close(started)
				<-release
			}
			active.Add(-1)
		}, delay)
		defer timer.Stop()

		advanceTime(3 * delay)
		<-started
		assertCalls(t, &calls, 1)

		close(release)
		synctest.Wait()
		if got := maxActive.Load(); got != 1 {
			t.Fatalf("maximum concurrent callbacks = %d, want 1", got)
		}
		assertCalls(t, &calls, 2)
	})
}

func TestTimerUnref(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = time.Second
		var calls atomic.Int32

		timer := SetTimeout(func() {
			calls.Add(1)
		}, delay)
		defer timer.Stop()
		timer.Unref()
		timer.Unref()

		advanceTime(delay)
		assertCalls(t, &calls, 1)
	})
}

func TestTimerConcurrentRefreshAndStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = time.Second
		var calls atomic.Int32

		timer := SetTimeout(func() {
			calls.Add(1)
		}, delay)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for range 64 {
			wg.Go(func() {
				<-start
				timer.Refresh()
			})
		}
		wg.Go(func() {
			<-start
			timer.Stop()
		})

		close(start)
		wg.Wait()
		timer.Stop()
		timer.Refresh()

		advanceTime(2 * delay)
		assertCalls(t, &calls, 0)
	})
}

func TestTimerConcurrentRefreshDuringCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const delay = time.Second
		var calls atomic.Int32
		started := make(chan struct{})
		release := make(chan struct{})

		timer := SetTimeout(func() {
			if calls.Add(1) == 1 {
				close(started)
				<-release
			}
		}, delay)
		defer timer.Stop()

		advanceTime(delay)
		<-started

		var wg sync.WaitGroup
		for range 64 {
			wg.Go(func() {
				timer.Refresh()
			})
		}
		wg.Wait()

		advanceTime(delay)
		assertCalls(t, &calls, 1)

		close(release)
		synctest.Wait()
		assertCalls(t, &calls, 2)

		advanceTime(delay)
		assertCalls(t, &calls, 2)
	})
}
