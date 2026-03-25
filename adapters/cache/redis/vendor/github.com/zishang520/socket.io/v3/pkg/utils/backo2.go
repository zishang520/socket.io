package utils

import (
	"math"
	"math/rand/v2"
	"sync/atomic"
)

// Backoff represents a thread-safe exponential backoff timer.
type Backoff struct {
	min      atomic.Uint64
	max      atomic.Uint64
	factor   atomic.Uint64
	jitter   atomic.Uint64
	attempts atomic.Uint64
}

// BackoffOption defines a configuration option function type.
type BackoffOption func(*Backoff)

// Default configuration constants
const (
	defaultMin    = 100.0
	defaultMax    = 10_000.0
	defaultFactor = 2.0
	maxAttempts   = 63 // Prevent math.Pow overflow
)

// WithMin sets the minimum backoff duration in milliseconds.
func WithMin(min float64) BackoffOption {
	return func(b *Backoff) {
		if isValid(min) && min > 0 {
			storeFloat(&b.min, min)
		}
	}
}

// WithMax sets the maximum backoff duration in milliseconds.
func WithMax(max float64) BackoffOption {
	return func(b *Backoff) {
		if isValid(max) && max > 0 {
			storeFloat(&b.max, max)
		}
	}
}

// WithFactor sets the exponential growth factor.
func WithFactor(factor float64) BackoffOption {
	return func(b *Backoff) {
		if isValid(factor) && factor > 1 {
			storeFloat(&b.factor, factor)
		}
	}
}

// WithJitter sets the jitter factor (between 0 and 1).
func WithJitter(jitter float64) BackoffOption {
	return func(b *Backoff) {
		if isValid(jitter) && jitter >= 0 && jitter <= 1 {
			storeFloat(&b.jitter, jitter)
		}
	}
}

// NewBackoff creates a new Backoff instance with the given options.
func NewBackoff(opts ...BackoffOption) *Backoff {
	b := &Backoff{}
	storeFloat(&b.min, defaultMin)
	storeFloat(&b.max, defaultMax)
	storeFloat(&b.factor, defaultFactor)

	for _, opt := range opts {
		opt(b)
	}

	// Ensure min <= max
	if b.GetMin() > b.GetMax() {
		storeFloat(&b.min, b.GetMax())
	}

	return b
}

// Attempts returns the current number of attempts.
func (b *Backoff) Attempts() uint64 {
	return b.attempts.Load()
}

// Duration calculates and returns the next backoff duration in milliseconds.
func (b *Backoff) Duration() int64 {
	attempt := min(b.attempts.Add(1)-1, maxAttempts)

	minVal := loadFloat(&b.min)
	maxVal := loadFloat(&b.max)
	factor := loadFloat(&b.factor)
	jitter := loadFloat(&b.jitter)

	// Calculate exponential backoff
	duration := minVal * math.Pow(factor, float64(attempt))
	duration = clamp(duration, minVal, maxVal)

	// Apply jitter
	if jitter > 0 {
		offset := jitter * duration * (rand.Float64()*2 - 1)
		duration = clamp(duration+offset, minVal, maxVal)
	}

	return int64(duration)
}

// Reset resets the attempt counter to zero.
func (b *Backoff) Reset() {
	b.attempts.Store(0)
}

// SetMin sets the minimum backoff duration.
func (b *Backoff) SetMin(val float64) {
	if !isValid(val) || val <= 0 {
		return
	}
	storeFloat(&b.min, min(val, b.GetMax()))
}

// SetMax sets the maximum backoff duration.
func (b *Backoff) SetMax(val float64) {
	if !isValid(val) || val <= 0 {
		return
	}
	storeFloat(&b.max, max(val, b.GetMin()))
}

// SetFactor sets the growth factor.
func (b *Backoff) SetFactor(val float64) {
	if isValid(val) && val > 1 {
		storeFloat(&b.factor, val)
	}
}

// SetJitter sets the jitter factor.
func (b *Backoff) SetJitter(val float64) {
	if isValid(val) && val >= 0 && val <= 1 {
		storeFloat(&b.jitter, val)
	}
}

// GetMin returns the current minimum backoff duration.
func (b *Backoff) GetMin() float64 {
	return loadFloat(&b.min)
}

// GetMax returns the current maximum backoff duration.
func (b *Backoff) GetMax() float64 {
	return loadFloat(&b.max)
}

// GetFactor returns the current growth factor.
func (b *Backoff) GetFactor() float64 {
	return loadFloat(&b.factor)
}

// GetJitter returns the current jitter factor.
func (b *Backoff) GetJitter() float64 {
	return loadFloat(&b.jitter)
}

// Helper functions

// storeFloat atomically stores a float64 value.
func storeFloat(target *atomic.Uint64, val float64) {
	target.Store(math.Float64bits(val))
}

// loadFloat atomically loads a float64 value.
func loadFloat(source *atomic.Uint64) float64 {
	return math.Float64frombits(source.Load())
}

// isValid checks if a float64 value is valid (not NaN or Inf).
func isValid(val float64) bool {
	return !math.IsNaN(val) && !math.IsInf(val, 0)
}

// clamp restricts a value to the range [minVal, maxVal].
func clamp(val, minVal, maxVal float64) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return maxVal
	}
	return max(minVal, min(val, maxVal))
}
