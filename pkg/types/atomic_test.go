package types

import (
	"sync"
	"testing"
)

func TestAtomicInt(t *testing.T) {
	var a Atomic[int]

	// Test zero value
	if v := a.Load(); v != 0 {
		t.Errorf("Expected zero value 0, got %d", v)
	}

	// Test Store and Load
	a.Store(42)
	if v := a.Load(); v != 42 {
		t.Errorf("Expected 42, got %d", v)
	}

	// Test Store different value
	a.Store(100)
	if v := a.Load(); v != 100 {
		t.Errorf("Expected 100, got %d", v)
	}
}

func TestAtomicString(t *testing.T) {
	var a Atomic[string]

	// Test zero value
	if v := a.Load(); v != "" {
		t.Errorf("Expected empty string, got %q", v)
	}

	// Test Store and Load
	a.Store("hello")
	if v := a.Load(); v != "hello" {
		t.Errorf("Expected 'hello', got %q", v)
	}
}

func TestAtomicBool(t *testing.T) {
	var a Atomic[bool]

	// Test zero value
	if v := a.Load(); v != false {
		t.Errorf("Expected false, got %v", v)
	}

	// Test Store and Load
	a.Store(true)
	if v := a.Load(); v != true {
		t.Errorf("Expected true, got %v", v)
	}
}

func TestAtomicSwap(t *testing.T) {
	var a Atomic[int]
	a.Store(10)

	// Test Swap
	old := a.Swap(20)
	if old != 10 {
		t.Errorf("Expected old value 10, got %d", old)
	}
	if v := a.Load(); v != 20 {
		t.Errorf("Expected current value 20, got %d", v)
	}
}

func TestAtomicCompareAndSwap(t *testing.T) {
	var a Atomic[int]
	a.Store(10)

	// Test successful CAS
	if success := a.CompareAndSwap(10, 20); !success {
		t.Error("Expected CompareAndSwap to succeed")
	}
	if v := a.Load(); v != 20 {
		t.Errorf("Expected value 20 after CAS, got %d", v)
	}

	// Test failed CAS (wrong old value)
	if success := a.CompareAndSwap(10, 30); success {
		t.Error("Expected CompareAndSwap to fail with wrong old value")
	}
	if v := a.Load(); v != 20 {
		t.Errorf("Expected value to remain 20 after failed CAS, got %d", v)
	}
}

func TestAtomicConcurrent(t *testing.T) {
	var a Atomic[int]
	a.Store(0)

	var wg sync.WaitGroup
	goroutines := 100
	iterations := 1000

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			for j := range iterations {
				a.Store(id*iterations + j)
				_ = a.Load()
			}
		}(i)
	}

	wg.Wait()

	// Just verify no race condition occurred (run with -race flag)
	_ = a.Load()
}

func TestAtomicStruct(t *testing.T) {
	type Point struct {
		X, Y int
	}

	var a Atomic[Point]

	// Test zero value
	if v := a.Load(); v != (Point{}) {
		t.Errorf("Expected zero Point, got %+v", v)
	}

	// Test Store and Load
	p := Point{X: 10, Y: 20}
	a.Store(p)
	if v := a.Load(); v != p {
		t.Errorf("Expected %+v, got %+v", p, v)
	}
}

func TestAtomicCompareAndSwapStruct(t *testing.T) {
	type Point struct {
		X, Y int
	}

	var a Atomic[Point]
	initial := Point{X: 1, Y: 2}
	a.Store(initial)

	// Test successful CAS
	new := Point{X: 3, Y: 4}
	if success := a.CompareAndSwap(initial, new); !success {
		t.Error("Expected CompareAndSwap to succeed")
	}
	if v := a.Load(); v != new {
		t.Errorf("Expected %+v after CAS, got %+v", new, v)
	}

	// Test failed CAS
	if success := a.CompareAndSwap(initial, Point{X: 5, Y: 6}); success {
		t.Error("Expected CompareAndSwap to fail with wrong old value")
	}
}
