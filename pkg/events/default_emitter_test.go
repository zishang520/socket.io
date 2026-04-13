package events

import (
	"sync"
	"testing"
)

func TestDefaultEventEmitterBasic(t *testing.T) {
	// Clear state before test
	Clear()

	called := false
	err := On("test_event", func(args ...any) {
		called = true
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	Emit("test_event", "data")

	if !called {
		t.Error("Expected listener to be called")
	}
}

func TestDefaultEventEmitterLen(t *testing.T) {
	Clear()

	if Len() != 0 {
		t.Errorf("Expected 0 events, got %d", Len())
	}

	_ = On("event1", func(...any) {})
	_ = On("event2", func(...any) {})

	if Len() != 2 {
		t.Errorf("Expected 2 events, got %d", Len())
	}
}

func TestDefaultEventEmitterEventNames(t *testing.T) {
	Clear()

	_ = On("alpha", func(...any) {})
	_ = On("beta", func(...any) {})
	_ = On("gamma", func(...any) {})

	names := EventNames()
	if len(names) != 3 {
		t.Errorf("Expected 3 event names, got %d", len(names))
	}
}

func TestDefaultEventEmitterListenerCount(t *testing.T) {
	Clear()

	_ = On("my_event", func(...any) {})
	_ = On("my_event", func(...any) {})

	count := ListenerCount("my_event")
	if count != 2 {
		t.Errorf("Expected 2 listeners, got %d", count)
	}

	count = ListenerCount("nonexistent")
	if count != 0 {
		t.Errorf("Expected 0 listeners for nonexistent event, got %d", count)
	}
}

func TestDefaultEventEmitterListeners(t *testing.T) {
	Clear()

	listener1 := func(...any) {}
	listener2 := func(...any) {}

	_ = On("test", listener1, listener2)

	listeners := Listeners("test")
	if len(listeners) != 2 {
		t.Errorf("Expected 2 listeners, got %d", len(listeners))
	}
}

func TestDefaultEventEmitterOnce(t *testing.T) {
	Clear()

	callCount := 0
	err := Once("once_event", func(...any) {
		callCount++
	})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Emit multiple times
	Emit("once_event")
	Emit("once_event")
	Emit("once_event")

	// Should only be called once
	if callCount != 1 {
		t.Errorf("Expected once listener to be called once, got %d", callCount)
	}
}

func TestDefaultEventEmitterRemoveListener(t *testing.T) {
	Clear()

	callCount := 0
	listener := func(...any) {
		callCount++
	}

	_ = On("removable", listener)
	_ = On("removable", func(...any) {
		callCount++
	})

	// Remove first listener
	result := RemoveListener("removable", listener)
	if !result {
		t.Error("Expected RemoveListener to return true")
	}

	// Emit event
	Emit("removable")

	// Only second listener should fire
	if callCount != 1 {
		t.Errorf("Expected 1 call after removal, got %d", callCount)
	}
}

func TestDefaultEventEmitterRemoveNonexistentListener(t *testing.T) {
	Clear()

	listener := func(...any) {}

	result := RemoveListener("nonexistent", listener)
	if result {
		t.Error("Expected RemoveListener to return false for nonexistent event")
	}
}

func TestDefaultEventEmitterRemoveAllListeners(t *testing.T) {
	Clear()

	_ = On("bulk", func(...any) {})
	_ = On("bulk", func(...any) {})
	_ = On("bulk", func(...any) {})

	result := RemoveAllListeners("bulk")
	if !result {
		t.Error("Expected RemoveAllListeners to return true")
	}

	count := ListenerCount("bulk")
	if count != 0 {
		t.Errorf("Expected 0 listeners after removal, got %d", count)
	}
}

func TestDefaultEventEmitterRemoveAllNonexistent(t *testing.T) {
	Clear()

	result := RemoveAllListeners("nonexistent")
	if result {
		t.Error("Expected RemoveAllListeners to return false for nonexistent event")
	}
}

func TestDefaultEventEmitterClear(t *testing.T) {
	Clear()

	_ = On("event1", func(...any) {})
	_ = On("event2", func(...any) {})
	_ = On("event3", func(...any) {})

	if Len() != 3 {
		t.Errorf("Expected 3 events before clear, got %d", Len())
	}

	Clear()

	if Len() != 0 {
		t.Errorf("Expected 0 events after clear, got %d", Len())
	}
}

func TestDefaultEventEmitterEmitWithData(t *testing.T) {
	Clear()

	var receivedData string
	var receivedNum int

	_ = On("data_event", func(args ...any) {
		receivedData = args[0].(string)
		receivedNum = args[1].(int)
	})

	Emit("data_event", "hello", 42)

	if receivedData != "hello" {
		t.Errorf("Expected 'hello', got %q", receivedData)
	}
	if receivedNum != 42 {
		t.Errorf("Expected 42, got %d", receivedNum)
	}
}

func TestDefaultEventEmitterMultipleListeners(t *testing.T) {
	Clear()

	var results []int

	listener1 := func(...any) {
		results = append(results, 1)
	}
	listener2 := func(...any) {
		results = append(results, 2)
	}
	listener3 := func(...any) {
		results = append(results, 3)
	}

	_ = On("multi", listener1, listener2, listener3)

	Emit("multi")

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// Listeners should be called in order
	if results[0] != 1 || results[1] != 2 || results[2] != 3 {
		t.Errorf("Expected [1, 2, 3], got %v", results)
	}
}

func TestDefaultEventEmitterConcurrent(t *testing.T) {
	Clear()

	var mu sync.Mutex
	callCount := 0

	_ = On("concurrent", func(...any) {
		mu.Lock()
		callCount++
		mu.Unlock()
	})

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			Emit("concurrent")
		})
	}

	wg.Wait()

	if callCount != 100 {
		t.Errorf("Expected 100 calls, got %d", callCount)
	}
}

func TestAddListener(t *testing.T) {
	Clear()

	err := AddListener("add_test", func(...any) {})
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if ListenerCount("add_test") != 1 {
		t.Errorf("Expected 1 listener, got %d", ListenerCount("add_test"))
	}
}

func TestOnAlias(t *testing.T) {
	Clear()

	called := false
	err := On("alias_test", func(...any) {
		called = true
	})

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	Emit("alias_test")

	if !called {
		t.Error("Expected On alias to work")
	}
}
