package utils

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// TestEncode tests the Encode method of the Yeast struct.
func TestEncode(t *testing.T) {
	y := NewYeast()

	tests := []struct {
		number   int64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{62, "-"},
		{63, "_"},
		{64, "10"},
		{123456, "U90"},
	}

	for _, test := range tests {
		result := y.Encode(test.number)
		if result != test.expected {
			t.Errorf("Encode(%d) = %s; expected %s", test.number, result, test.expected)
		}
	}
}

// TestDecode tests the Decode method of the Yeast struct.
func TestDecode(t *testing.T) {
	y := NewYeast()

	tests := []struct {
		str      string
		expected int64
	}{
		{"0", 0},
		{"1", 1},
		{"-", 62},
		{"_", 63},
		{"10", 64},
		{"W7E", 131534},
	}

	for _, test := range tests {
		result, err := y.Decode(test.str)
		if err != nil {
			t.Fatalf("Decode(%s) returned error: %v", test.str, err)
		}
		if result != test.expected {
			t.Errorf("Decode(%s) = %d; expected %d", test.str, result, test.expected)
		}
	}
}

// TestYeast tests the Yeast method of the Yeast struct.
func TestYeast(t *testing.T) {
	y := NewYeast()

	// Generate multiple YEAST IDs to ensure uniqueness and correctness
	id1 := y.Yeast()
	id2 := y.Yeast()

	if id1 == id2 {
		t.Errorf("Yeast() generated two identical IDs: %s and %s", id1, id2)
	}

	// Add a short delay to ensure different millisecond timestamp
	time.Sleep(1 * time.Millisecond)

	id3 := y.Yeast()
	if id1 == id3 || id2 == id3 {
		t.Errorf("Yeast() generated a duplicate ID: %s", id3)
	}
}

func TestYeastConcurrentUniqueness(t *testing.T) {
	const workers, perWorker = 16, 4096
	y := NewYeast()
	ids := make([][]string, workers)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for worker := range workers {
		wg.Go(func() {
			batch := make([]string, perWorker)
			<-start
			for i := range batch {
				batch[i] = y.Yeast()
			}
			ids[worker] = batch
		})
	}
	close(start)
	wg.Wait()

	seen := make(map[string]struct{}, workers*perWorker)
	for _, batch := range ids {
		for _, id := range batch {
			if _, exists := seen[id]; exists {
				t.Fatalf("duplicate ID from concurrent calls: %q", id)
			}
			seen[id] = struct{}{}
		}
	}
}

func TestYeastSequence(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		y := NewYeast()
		prefix := y.Encode(time.Now().UnixMilli())
		for _, want := range []string{prefix, prefix + ".0", prefix + ".1"} {
			if got := y.Yeast(); got != want {
				t.Fatalf("Yeast() = %q, want %q", got, want)
			}
		}
		time.Sleep(time.Millisecond)
		if got, want := y.Yeast(), y.Encode(time.Now().UnixMilli()); got != want {
			t.Fatalf("Yeast() after millisecond change = %q, want %q", got, want)
		}
	})
}
