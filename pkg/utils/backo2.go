package utils

import (
	"math"
	"math/rand/v2"
	"sync/atomic"
)

// Backoff represents a backoff timer with configurable parameters.
type Backoff struct {
	min      atomic.Value
	max      atomic.Value
	factor   atomic.Value
	jitter   atomic.Value
	attempts atomic.Uint64
}

// BackoffConfig holds configuration parameters for the Backoff.
type backoffConfig struct {
	min    float64
	max    float64
	factor float64
	jitter float64
}

type backoffOption = func(*backoffConfig)

func WithMin(min float64) backoffOption {
	return func(c *backoffConfig) {
		c.min = min
	}
}

func WithMax(max float64) backoffOption {
	return func(c *backoffConfig) {
		c.max = max
	}
}

func WithFactor(factor float64) backoffOption {
	return func(c *backoffConfig) {
		c.factor = factor
	}
}

func WithJitter(jitter float64) backoffOption {
	return func(c *backoffConfig) {
		if jitter > 0 && jitter <= 1 {
			c.jitter = jitter
		} else {
			c.jitter = 0
		}
	}
}

// NewBackoff creates a new Backoff instance with the given configuration.
func NewBackoff(opts ...backoffOption) *Backoff {
	config := &backoffConfig{
		min:    100,
		max:    10_000,
		factor: 2,
		jitter: 0,
	}
	for _, f := range opts {
		f(config)
	}

	b := &Backoff{}
	b.min.Store(config.min)
	b.max.Store(config.max)
	b.factor.Store(config.factor)
	b.jitter.Store(config.jitter)
	b.attempts.Store(0)

	return b
}

func (b *Backoff) Attempts() uint64 {
	return b.attempts.Load()
}

// Duration returns the next backoff duration.
func (b *Backoff) Duration() int64 {
	ms := b.min.Load().(float64) * math.Pow(b.factor.Load().(float64), float64(b.attempts.Add(1)-1))
	if jitter := b.jitter.Load().(float64); jitter > 0 {
		ms += jitter * ms * (rand.Float64()*2 - 1)
	}
	return int64(math.Max(b.min.Load().(float64), math.Min(ms, b.max.Load().(float64))))
}

// Reset resets the number of attempts to 0.
func (b *Backoff) Reset() {
	b.attempts.Store(0)
}

// SetMin sets the minimum duration (in milliseconds).
func (b *Backoff) SetMin(min float64) {
	if min > b.max.Load().(float64) {
		min = b.max.Load().(float64)
	}
	b.min.Store(min)
}

// SetMax sets the maximum duration (in milliseconds).
func (b *Backoff) SetMax(max float64) {
	if max < b.min.Load().(float64) {
		max = b.min.Load().(float64)
	}
	b.max.Store(max)
}

// SetJitter sets the jitter factor.
func (b *Backoff) SetJitter(jitter float64) {
	if jitter < 0 || jitter > 1 {
		jitter = 0
	}
	b.jitter.Store(jitter)
}
