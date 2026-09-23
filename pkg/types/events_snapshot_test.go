package types

import (
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"
)

func TestEmitKeepsListenerSnapshot(t *testing.T) {
	e := NewEventEmitter()
	var calls []string
	second := func(...any) { calls = append(calls, "second") }
	_ = e.On("event", func(...any) {
		calls = append(calls, "first")
		e.RemoveListener("event", second)
		_ = e.On("event", func(...any) { calls = append(calls, "new") })
	}, second)
	e.Emit("event")
	if !slices.Equal(calls, []string{"first", "second"}) {
		t.Fatalf("listeners changed during Emit: %v", calls)
	}
	calls = nil
	e.Emit("event")
	if !slices.Equal(calls, []string{"first", "new"}) {
		t.Fatalf("next Emit did not see listener changes: %v", calls)
	}
}

func TestEmitSnapshotIgnoresConcurrentRegistration(t *testing.T) {
	e := NewEventEmitter()
	started, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	first := true
	_ = e.On("event", func(...any) {
		if first {
			first = false
			close(started)
			<-release
		}
	}, func(...any) {}, func(...any) {})
	go func() {
		e.Emit("event")
		close(done)
	}()
	<-started
	calls := 0
	_ = e.On("event", func(...any) { calls++ })
	close(release)
	<-done
	if calls != 0 {
		t.Fatal("an in-flight Emit invoked a newly registered listener")
	}
	e.Emit("event")
	if calls != 1 {
		t.Fatal("the next Emit did not invoke the new listener")
	}
}

func TestEventEmitterEmptyResults(t *testing.T) {
	e := NewEventEmitter()
	if e.EventNames() != nil || e.Listeners("missing") != nil {
		t.Fatal("absent events must return nil slices")
	}
	listener := func(...any) {}
	_ = e.On("event", listener)
	e.RemoveListener("event", listener)
	if e.Len() != 1 || e.Listeners("event") == nil || len(e.Listeners("event")) != 0 {
		t.Fatal("removing the last listener must retain the empty event")
	}
	e.RemoveAllListeners("event")
	if e.EventNames() != nil {
		t.Fatal("removing all events must return nil names")
	}
	_ = e.On("event", listener)
	e.Clear()
	if e.EventNames() != nil {
		t.Fatal("clearing events must return nil names")
	}
}

func TestOnceReturnedListenerCanReenter(t *testing.T) {
	e := NewEventEmitter()
	var listener EventListener
	calls := 0
	_ = e.Once("event", func(...any) {
		calls++
		listener()
	})
	listener = e.Listeners("event")[0]
	done := make(chan struct{})
	go func() {
		e.Emit("event")
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reentering a saved Once listener deadlocked")
	}
	if calls != 1 || e.ListenerCount("event") != 0 {
		t.Fatalf("calls=%d listeners=%d", calls, e.ListenerCount("event"))
	}
}

func TestOnceRemovesItsOwnRegistration(t *testing.T) {
	e := NewEventEmitter()
	calls := 0
	listener := func(...any) { calls++ }
	_ = e.On("event", listener)
	_ = e.Once("event", listener)
	e.Emit("event")
	e.Emit("event")
	if calls != 3 {
		t.Fatalf("Once removed another registration: calls=%d, want 3", calls)
	}
}

type subscriptionListener struct {
	name  string
	calls *[]string
}

func (l *subscriptionListener) call(...any) {
	*l.calls = append(*l.calls, l.name)
}

func TestSubscribeDistinguishesMethodReceivers(t *testing.T) {
	e := NewEventEmitter()
	var calls []string
	first := &subscriptionListener{name: "first", calls: &calls}
	second := &subscriptionListener{name: "second", calls: &calls}
	stopFirst := Subscribe(e, "event", first.call)
	stopSecond := Subscribe(e, "event", second.call)
	stopSecond()
	stopSecond()
	e.Emit("event")
	if !slices.Equal(calls, []string{"first"}) {
		t.Fatalf("callbacks after removing second = %v, want [first]", calls)
	}
	stopFirst()
	if e.ListenerCount("event") != 0 {
		t.Fatal("first subscription was not removed")
	}
}

func TestSubscribePreservesOtherRegistrations(t *testing.T) {
	e := NewEventEmitter()
	calls := 0
	listener := func(...any) { calls++ }
	_ = e.On("event", listener)
	_ = e.Once("event", listener)
	stopFirst := Subscribe(e, "event", listener)
	stopSecond := Subscribe(e, "event", listener)
	stopSecond()
	e.Emit("event")
	stopFirst()
	stopSecond()
	e.Emit("event")
	if calls != 4 || e.ListenerCount("event") != 1 {
		t.Fatalf("calls=%d listeners=%d, want 4 calls and the original On listener", calls, e.ListenerCount("event"))
	}
}

func TestSubscribeCleanupAfterClear(t *testing.T) {
	e := NewEventEmitter()
	calls := 0
	listener := func(...any) { calls++ }
	stop := Subscribe(e, "event", listener)
	e.Clear()
	_ = Subscribe(e, "event", listener)
	stop()
	e.Emit("event")
	if calls != 1 || e.ListenerCount("event") != 1 {
		t.Fatal("old cleanup removed a new registration")
	}
}

func TestSubscribeRemovalKeepsEmitSnapshot(t *testing.T) {
	e := NewEventEmitter()
	var calls []string
	var stopSecond Callable
	_ = Subscribe(e, "event", func(...any) {
		calls = append(calls, "first")
		stopSecond()
	})
	stopSecond = Subscribe(e, "event", func(...any) { calls = append(calls, "second") })
	e.Emit("event")
	e.Emit("event")
	if !slices.Equal(calls, []string{"first", "second", "first"}) {
		t.Fatalf("callbacks = %v, want [first second first]", calls)
	}
}

func TestSubscribeConcurrentCleanup(t *testing.T) {
	e := NewEventEmitter()
	stop := Subscribe(e, "event", func(...any) {})
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(stop)
	}
	wg.Wait()
	if e.ListenerCount("event") != 0 {
		t.Fatal("subscription remained after concurrent cleanup")
	}
	Subscribe(e, "missing", nil)()
	if e.Listeners("missing") != nil {
		t.Fatal("nil listener created an event")
	}
}

type subscriptionEmitter struct {
	EventEmitter
	registrations int
	removals      int
}

func (e *subscriptionEmitter) On(evt EventName, listeners ...EventListener) error {
	e.registrations++
	return e.EventEmitter.On(evt, listeners...)
}

func (e *subscriptionEmitter) RemoveListener(evt EventName, listener EventListener) bool {
	e.removals++
	return e.EventEmitter.RemoveListener(evt, listener)
}

func TestSubscribeUsesCustomEmitterMethods(t *testing.T) {
	e := &subscriptionEmitter{EventEmitter: NewEventEmitter()}
	stop := Subscribe(e, "event", func(...any) {})
	stop()
	if e.registrations != 1 || e.removals != 1 || e.ListenerCount("event") != 0 {
		t.Fatalf("registrations=%d removals=%d remaining=%d", e.registrations, e.removals, e.ListenerCount("event"))
	}
}

func BenchmarkEventEmitterSnapshot(b *testing.B) {
	for _, count := range []int{1, 8, 32} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			e := NewEventEmitter()
			listeners := make([]EventListener, count)
			for i := range listeners {
				listeners[i] = func(...any) {}
			}
			_ = e.On("event", listeners...)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				e.Emit("event")
			}
		})
	}
}

func BenchmarkEventEmitterRegister(b *testing.B) {
	for _, count := range []int{8, 128} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			listener := func(...any) {}
			b.ReportAllocs()
			for b.Loop() {
				e := NewEventEmitter()
				for range count {
					_ = e.On("event", listener)
				}
			}
		})
	}
}
