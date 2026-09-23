package types

import (
	"runtime"
	"testing"
	"time"
	"weak"
)

//go:noinline
func subscriptionWithCapturedPayload() (Callable, weak.Pointer[[4096]byte]) {
	payload := new([4096]byte)
	emitter := NewEventEmitter()
	stop := Subscribe(emitter, "event", func(...any) { runtime.KeepAlive(payload) })
	return stop, weak.Make(payload)
}

func TestCanceledSubscriptionReleasesListenerCapture(t *testing.T) {
	stop, payload := subscriptionWithCapturedPayload()
	defer func() { runtime.KeepAlive(stop) }()
	stop()

	deadline := time.Now().Add(3 * time.Second)
	for {
		runtime.GC()
		if payload.Value() == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("canceled subscription retains the listener payload through its cleanup function")
		}
		time.Sleep(10 * time.Millisecond)
	}
	stop() // The retained cleanup must remain safe to call after collection.
}
