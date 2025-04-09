package events

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var _event = New()

var testEvents = Events{
	"user_created": []Listener{
		func(payload ...any) {
			fmt.Printf("A new User just created!\n")
		},
		func(payload ...any) {
			fmt.Printf("A new User just created, *from second event listener\n")
		},
	},
	"user_joined": []Listener{func(payload ...any) {
		user := payload[0].(string)
		room := payload[1].(string)
		fmt.Printf("%s joined to room: %s\n", user, room)
	}},
	"user_left": []Listener{func(payload ...any) {
		user := payload[0].(string)
		room := payload[1].(string)
		fmt.Printf("%s left from the room: %s\n", user, room)
	}},
}

func createUser(user string) {
	_event.Emit("user_created", user)
}

func joinUserTo(user string, room string) {
	_event.Emit("user_joined", user, room)
}

func leaveFromRoom(user string, room string) {
	_event.Emit("user_left", user, room)
}

func ExampleEvents() {
	// regiter our events to the default event emmiter
	for evt, listeners := range testEvents {
		_event.On(evt, listeners...)
	}

	user := "user1"
	room := "room1"

	createUser(user)
	joinUserTo(user, room)
	leaveFromRoom(user, room)

	// Output:
	// A new User just created!
	// A new User just created, *from second event listener
	// user1 joined to room: room1
	// user1 left from the room: room1
}

func TestEvents(t *testing.T) {
	e := New()
	expectedPayload := "this is my payload"

	e.On("my_event", func(payload ...any) {
		if len(payload) <= 0 {
			t.Fatal("Expected payload but got nothing")
		}

		if s, ok := payload[0].(string); !ok {
			t.Fatalf("Payload is not the correct type, got: %#v", payload[0])
		} else if s != expectedPayload {
			t.Fatalf("Eexpected %s, got: %s", expectedPayload, s)
		}
	})

	e.Emit("my_event", expectedPayload)
	if e.Len() != 1 {
		t.Fatalf("Length of the events is: %d, while expecting: %d", e.Len(), 1)
	}

	if e.Len() != 1 {
		t.Fatalf("Length of the listeners is: %d, while expecting: %d", e.ListenerCount("my_event"), 1)
	}

	e.RemoveAllListeners("my_event")
	if e.Len() != 0 {
		t.Fatalf("Length of the events is: %d, while expecting: %d", e.Len(), 0)
	}

	if e.Len() != 0 {
		t.Fatalf("Length of the listeners is: %d, while expecting: %d", e.ListenerCount("my_event"), 0)
	}
}

func TestEventsOnce(t *testing.T) {
	// on default
	_event.Clear()

	var count = 0
	_event.Once("my_event", func(payload ...any) {
		if count > 0 {
			t.Fatalf("Once's listener fired more than one time! count: %d", count)
		}
		if l := len(payload); l != 2 {
			t.Fatalf("Once's listeners (from Listeners) should be: %d but has: %d", 2, l)
		}
		count++
	})
	if l := _event.ListenerCount("my_event"); l != 1 {
		t.Fatalf("Real  event's listeners should be: %d but has: %d", 1, l)
	}

	if l := len(_event.Listeners("my_event")); l != 1 {
		t.Fatalf("Real  event's listeners (from Listeners) should be: %d but has: %d", 1, l)
	}

	for i := 0; i < 10; i++ {
		_event.Emit("my_event", "foo", "foo1")
	}

	time.Sleep(10 * time.Millisecond)

	if l := _event.ListenerCount("my_event"); l > 0 {
		t.Fatalf("Real event's listeners length count should be: %d but has: %d", 0, l)
	}

	if l := len(_event.Listeners("my_event")); l > 0 {
		t.Fatalf("Real event's listeners length count ( from Listeners) should be: %d but has: %d", 0, l)
	}

}

func TestRemoveListener(t *testing.T) {
	// on default
	e := New()

	var count = 0
	listener := func(payload ...any) {
		if count > 1 {
			t.Fatal("Event listener should be removed")
		}

		count++
	}

	once := func(payload ...any) {}

	e.Once("once_event", once)
	e.AddListener("my_event", listener)
	e.AddListener("my_event", func(payload ...any) {})
	e.AddListener("another_event", func(payload ...any) {})

	e.Emit("my_event")

	if e.RemoveListener("once_event", once) != true {
		t.Fatal("Should return 'true' when removes found once listener")
	}

	if e.ListenerCount("once_event") != 0 {
		t.Fatal("Length of 'once_event' event listeners must be 0")
	}

	if e.RemoveListener("my_event", listener) != true {
		t.Fatal("Should return 'true' when removes found listener")
	}

	if e.RemoveListener("foo_bar", listener) != false {
		t.Fatal("Should return 'false' when removes nothing")
	}

	if e.Len() != 3 {
		t.Fatal("Length of all listeners must be 2")
	}

	if e.ListenerCount("my_event") != 1 {
		t.Fatal("Length of 'my_event' event listeners must be 1")
	}

	e.Emit("my_event")
}

func BenchmarkConcurrentEmit(b *testing.B) {
	emitter := New()
	emitter.On("bench", func(...any) {})

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			emitter.Emit("bench")
		}
	})
}

// Test concurrent addition of listeners
func TestConcurrentAddListeners(t *testing.T) {
	emitter := New()
	const numListeners = 100
	var wg sync.WaitGroup

	wg.Add(numListeners)
	for i := 0; i < numListeners; i++ {
		go func() {
			defer wg.Done()
			emitter.On("test", func(...any) {})
		}()
	}
	wg.Wait()

	if count := emitter.ListenerCount("test"); count != numListeners {
		t.Fatalf("Expected %d listeners, got %d", numListeners, count)
	}
}

// Test concurrent event emission and listener removal
func TestConcurrentEmitRemoveListener(t *testing.T) {
	emitter := New()
	var wg sync.WaitGroup

	const numListeners = 50
	var listener func(...any)
	listener = func(...any) {
		emitter.RemoveListener("inc", listener)
	}
	for i := 0; i < numListeners; i++ {
		emitter.On("inc", listener)
	}

	const numEmits = 100
	wg.Add(numEmits)
	for i := 0; i < numEmits; i++ {
		go func() {
			defer wg.Done()
			emitter.Emit("inc")
		}()
	}
	wg.Wait()

	if count := emitter.ListenerCount("inc"); count != 0 {
		t.Fatalf("Expected %d listeners, got %d", 0, count)
	}

	// Stress test verification
	for i := 0; i < 1000; i++ {
		emitter.Emit("inc")
		if c := emitter.ListenerCount("inc"); c > 0 {
			t.Fatalf("Found %d dangling listeners after cleanup", c)
		}
	}
}

// Test concurrent event emission
func TestConcurrentEmit(t *testing.T) {
	emitter := New()
	var (
		counter int32
		wg      sync.WaitGroup
	)

	const numListeners = 50
	for i := 0; i < numListeners; i++ {
		emitter.On("inc", func(...any) {
			atomic.AddInt32(&counter, 1)
		})
	}

	const numEmits = 100
	wg.Add(numEmits)
	for i := 0; i < numEmits; i++ {
		go func() {
			defer wg.Done()
			emitter.Emit("inc")
		}()
	}
	wg.Wait()

	expected := int32(numListeners * numEmits)
	if actual := atomic.LoadInt32(&counter); actual != expected {
		t.Fatalf("Expected counter %d, got %d", expected, actual)
	}
}

// Test concurrent execution of a one-time listener
func TestConcurrentOnce(t *testing.T) {
	emitter := New()
	var (
		counter int32
		wg      sync.WaitGroup
	)

	emitter.Once("once", func(...any) {
		atomic.AddInt32(&counter, 1)
	})

	const numEmits = 100
	wg.Add(numEmits)
	for i := 0; i < numEmits; i++ {
		go func() {
			defer wg.Done()
			emitter.Emit("once")
		}()
	}
	wg.Wait()

	if atomic.LoadInt32(&counter) != 1 {
		t.Fatalf("Expected Once listener called once, got %d", counter)
	}
}

// Test concurrent removal of all listeners
func TestConcurrentRemoveAll(t *testing.T) {
	emitter := New()
	var wg sync.WaitGroup

	// Add initial listeners
	const numListeners = 50
	for i := 0; i < numListeners; i++ {
		emitter.On("test", func(...any) {})
	}

	// Concurrently trigger and remove listeners
	wg.Add(2)
	go func() {
		defer wg.Done()
		emitter.Emit("test")
	}()
	go func() {
		defer wg.Done()
		emitter.RemoveAllListeners("test")
	}()
	wg.Wait()

	// Ensure all listeners are removed
	if count := emitter.ListenerCount("test"); count != 0 {
		t.Fatalf("Expected 0 listeners after removal, got %d", count)
	}
}
