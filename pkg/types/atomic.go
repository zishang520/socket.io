package types

import (
	"sync/atomic"
)

// Atomic is a type-safe atomic value container.
// The zero value is the zero value of TValue.
// An Atomic must not be copied after first use.
type Atomic[TValue any] struct {
	_    noCopy
	zero TValue
	v    atomic.Value
}

// Load atomically loads and returns the value stored in x.
// If the stored value is not of type TValue, returns the zero value of TValue.
func (s *Atomic[TValue]) Load() TValue {
	if val, ok := s.v.Load().(TValue); ok {
		return val
	}
	return s.zero
}

// Store atomically stores val into x.
func (s *Atomic[TValue]) Store(val TValue) {
	s.v.Store(val)
}

// Swap atomically stores new into x and returns the previous value.
// If the previous value is not of type TValue, returns the zero value of TValue.
func (s *Atomic[TValue]) Swap(new TValue) (old TValue) {
	if old, ok := s.v.Swap(new).(TValue); ok {
		return old
	}
	return s.zero
}

// CompareAndSwap executes the compare-and-swap operation for x.
// It returns true if the swap was successful (old matched the current value),
// false otherwise.
func (s *Atomic[TValue]) CompareAndSwap(old, new TValue) (swapped bool) {
	return s.v.CompareAndSwap(old, new)
}
