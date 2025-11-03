package utils

import (
	"testing"
	"time"
)

func TestNewBackoff(t *testing.T) {
	tests := []struct {
		name     string
		opts     []BackoffOption
		wantMin  float64
		wantMax  float64
		wantFact float64
	}{
		{
			name:     "default values",
			opts:     []BackoffOption{},
			wantMin:  100,
			wantMax:  10000,
			wantFact: 2,
		},
		{
			name:     "custom values",
			opts:     []BackoffOption{WithMin(200), WithMax(5000), WithFactor(1.5)},
			wantMin:  200,
			wantMax:  5000,
			wantFact: 1.5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewBackoff(tt.opts...)
			if got := b.Duration(); got < int64(tt.wantMin) || got > int64(tt.wantMax) {
				t.Errorf("Initial duration outside range: got %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestBackoff_Duration(t *testing.T) {
	b := NewBackoff()
	var prev int64

	// Test multiple attempts
	for range 5 {
		curr := b.Duration()
		if curr < prev {
			t.Errorf("Duration decreased: prev=%v, curr=%v", prev, curr)
		}
		prev = curr
		time.Sleep(time.Millisecond) // Ensure different random values
	}
}

func TestBackoff_Reset(t *testing.T) {
	b := NewBackoff()

	// Get some durations to advance attempts
	initial := b.Duration()
	_ = b.Duration()
	_ = b.Duration()

	b.Reset()
	after := b.Duration()

	if initial != after {
		t.Errorf("Reset failed: initial=%v, after reset=%v", initial, after)
	}
}

func TestBackoff_SetMethods(t *testing.T) {
	b := NewBackoff()

	newMin := 100.0
	b.SetMin(newMin)
	if got := b.Duration(); got < int64(newMin) {
		t.Errorf("SetMin failed: got %v, want >= %v", got, newMin)
	}

	newMax := 10000.0
	b.SetMax(newMax)
	for i := 0; i < 10; i++ {
		if got := b.Duration(); got > int64(newMax) {
			t.Errorf("SetMax failed: got %v, want <= %v", got, newMax)
		}
	}

	b.Reset()
	b.SetJitter(0.5)
	prev := b.Duration()
	found := false
	for range 5 {
		curr := b.Duration()
		if curr != prev {
			found = true
			break
		}
		prev = curr
	}
	if !found {
		t.Error("SetJitter seems to have no effect")
	}
}
