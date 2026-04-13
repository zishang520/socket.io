package utils

import (
	"math"
	"testing"
)

func TestBackoff_Attempts(t *testing.T) {
	b := NewBackoff()
	if got := b.Attempts(); got != 0 {
		t.Errorf("Initial Attempts() = %d, want 0", got)
	}
	b.Duration()
	if got := b.Attempts(); got != 1 {
		t.Errorf("After one Duration(), Attempts() = %d, want 1", got)
	}
	b.Duration()
	b.Duration()
	if got := b.Attempts(); got != 3 {
		t.Errorf("After three Duration(), Attempts() = %d, want 3", got)
	}
}

func TestBackoff_GetMethods(t *testing.T) {
	b := NewBackoff(WithMin(200), WithMax(5000), WithFactor(3), WithJitter(0.5))
	if got := b.GetMin(); got != 200 {
		t.Errorf("GetMin() = %v, want 200", got)
	}
	if got := b.GetMax(); got != 5000 {
		t.Errorf("GetMax() = %v, want 5000", got)
	}
	if got := b.GetFactor(); got != 3 {
		t.Errorf("GetFactor() = %v, want 3", got)
	}
	if got := b.GetJitter(); got != 0.5 {
		t.Errorf("GetJitter() = %v, want 0.5", got)
	}
}

func TestBackoff_SetFactor(t *testing.T) {
	b := NewBackoff()
	b.SetFactor(3)
	if got := b.GetFactor(); got != 3 {
		t.Errorf("After SetFactor(3), GetFactor() = %v, want 3", got)
	}

	// Invalid values should be ignored
	b.SetFactor(1) // Must be > 1
	if got := b.GetFactor(); got != 3 {
		t.Errorf("SetFactor(1) should be ignored, GetFactor() = %v, want 3", got)
	}
	b.SetFactor(-1)
	if got := b.GetFactor(); got != 3 {
		t.Errorf("SetFactor(-1) should be ignored, GetFactor() = %v, want 3", got)
	}
	b.SetFactor(math.NaN())
	if got := b.GetFactor(); got != 3 {
		t.Errorf("SetFactor(NaN) should be ignored, GetFactor() = %v, want 3", got)
	}
}

func TestBackoff_InvalidOptions(t *testing.T) {
	// NaN, Inf, negative values should be ignored, falling back to defaults
	b := NewBackoff(
		WithMin(math.NaN()),
		WithMax(math.Inf(1)),
		WithFactor(math.Inf(-1)),
		WithJitter(-0.5),
	)
	if got := b.GetMin(); got != defaultMin {
		t.Errorf("WithMin(NaN) should use default, got %v", got)
	}
	if got := b.GetMax(); got != defaultMax {
		t.Errorf("WithMax(+Inf) should use default, got %v", got)
	}
	if got := b.GetFactor(); got != defaultFactor {
		t.Errorf("WithFactor(-Inf) should use default, got %v", got)
	}
	if got := b.GetJitter(); got != 0 {
		t.Errorf("WithJitter(-0.5) should use default 0, got %v", got)
	}
}

func TestBackoff_MinGreaterThanMax(t *testing.T) {
	b := NewBackoff(WithMin(10000), WithMax(100))
	// min should be clamped to max
	if b.GetMin() > b.GetMax() {
		t.Errorf("min (%v) should not be greater than max (%v)", b.GetMin(), b.GetMax())
	}
}

func TestBackoff_SetMinClampsToMax(t *testing.T) {
	b := NewBackoff(WithMax(500))
	b.SetMin(1000) // larger than max
	if b.GetMin() > b.GetMax() {
		t.Errorf("SetMin should clamp to max: min=%v, max=%v", b.GetMin(), b.GetMax())
	}
}

func TestBackoff_SetMaxClampsToMin(t *testing.T) {
	b := NewBackoff(WithMin(500))
	b.SetMax(100) // smaller than min
	if b.GetMax() < b.GetMin() {
		t.Errorf("SetMax should clamp to min: min=%v, max=%v", b.GetMin(), b.GetMax())
	}
}

func TestBackoff_SetInvalidValues(t *testing.T) {
	b := NewBackoff()
	origMin := b.GetMin()
	origMax := b.GetMax()

	b.SetMin(math.NaN())
	if b.GetMin() != origMin {
		t.Error("SetMin(NaN) should be ignored")
	}
	b.SetMin(-100)
	if b.GetMin() != origMin {
		t.Error("SetMin(-100) should be ignored")
	}
	b.SetMax(math.Inf(1))
	if b.GetMax() != origMax {
		t.Error("SetMax(Inf) should be ignored")
	}
	b.SetMax(0)
	if b.GetMax() != origMax {
		t.Error("SetMax(0) should be ignored")
	}
}

func TestBackoff_SetJitter(t *testing.T) {
	b := NewBackoff()
	b.SetJitter(0.5)
	if got := b.GetJitter(); got != 0.5 {
		t.Errorf("SetJitter(0.5) = %v, want 0.5", got)
	}
	b.SetJitter(1.5) // out of range, should be ignored
	if got := b.GetJitter(); got != 0.5 {
		t.Errorf("SetJitter(1.5) should be ignored, got %v", got)
	}
	b.SetJitter(0)
	if got := b.GetJitter(); got != 0 {
		t.Errorf("SetJitter(0) = %v, want 0", got)
	}
	b.SetJitter(1)
	if got := b.GetJitter(); got != 1 {
		t.Errorf("SetJitter(1) = %v, want 1", got)
	}
}

func TestBackoff_DurationWithMaxAttempts(t *testing.T) {
	b := NewBackoff()
	// Push attempts well beyond maxAttempts to ensure no overflow
	for range 100 {
		d := b.Duration()
		if d < int64(b.GetMin()) || d > int64(b.GetMax()) {
			t.Errorf("Duration() = %d, outside [%v, %v]", d, b.GetMin(), b.GetMax())
		}
	}
}

func TestBackoff_WithJitterOption(t *testing.T) {
	b := NewBackoff(WithJitter(1.0))
	if got := b.GetJitter(); got != 1.0 {
		t.Errorf("WithJitter(1.0) = %v, want 1.0", got)
	}
}
