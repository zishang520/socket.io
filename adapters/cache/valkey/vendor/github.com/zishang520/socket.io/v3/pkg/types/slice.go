package types

import (
	"errors"
	"slices"
	"sync"
)

// Define custom error types
var (
	ErrSliceEmpty        = errors.New("slice is empty")
	ErrIndexOutOfBounds  = errors.New("index out of bounds")
	ErrInvalidSliceRange = errors.New("invalid slice range")
)

// Slice is a thread-safe generic slice type.
type Slice[T any] struct {
	mu       sync.RWMutex
	elements []T
}

// NewSlice creates and returns a new Slice.
// The input elements are copied to avoid aliasing with the caller's backing array.
func NewSlice[T any](elements ...T) *Slice[T] {
	return &Slice[T]{elements: slices.Clone(elements)}
}

// Push adds one or more elements to the end of the slice and returns the new length.
func (s *Slice[T]) Push(elements ...T) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.elements = append(s.elements, elements...)
	return len(s.elements)
}

// Unshift adds one or more elements to the beginning of the slice and returns the new length.
// Uses a single allocation instead of the double-append pattern.
func (s *Slice[T]) Unshift(elements ...T) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(elements) == 0 {
		return len(s.elements)
	}
	newLen := len(s.elements) + len(elements)
	merged := make([]T, newLen)
	copy(merged, elements)
	copy(merged[len(elements):], s.elements)
	s.elements = merged
	return newLen
}

// Pop removes the last element from the slice and returns it, or an error if the slice is empty.
func (s *Slice[T]) Pop() (element T, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := len(s.elements)
	if n == 0 {
		return element, ErrSliceEmpty
	}
	element = s.elements[n-1]
	clear(s.elements[n-1 : n]) // zero out removed slot to help GC for pointer types
	s.elements = s.elements[:n-1]
	return element, nil
}

// Shift removes the first element from the slice and returns it, or an error if the slice is empty.
func (s *Slice[T]) Shift() (element T, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.elements) == 0 {
		return element, ErrSliceEmpty
	}
	element = s.elements[0]
	clear(s.elements[:1]) // zero out removed slot to help GC for pointer types
	s.elements = s.elements[1:]
	return element, nil
}

// Get returns the element at the specified index, or an error if the index is out of bounds.
func (s *Slice[T]) Get(index int) (element T, err error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if index < 0 || index >= len(s.elements) {
		return element, ErrIndexOutOfBounds
	}
	return s.elements[index], nil
}

// Set sets the element at the specified index, and returns an error if the index is out of bounds.
func (s *Slice[T]) Set(index int, element T) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if index < 0 || index >= len(s.elements) {
		return ErrIndexOutOfBounds
	}
	s.elements[index] = element
	return nil
}

// Slice returns a new slice containing the elements between start and end.
func (s *Slice[T]) Slice(start, end int) ([]T, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if start < 0 || end > len(s.elements) || start > end {
		return nil, ErrInvalidSliceRange
	}
	return slices.Clone(s.elements[start:end]), nil
}

// Filter returns a new slice containing all elements that satisfy the provided function.
func (s *Slice[T]) Filter(condition func(T) bool) (filtered []T) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, el := range s.elements {
		if condition(el) {
			filtered = append(filtered, el)
		}
	}
	return filtered
}

// Splice removes elements from the slice at the specified start index and inserts new elements at that position.
func (s *Slice[T]) Splice(start, deleteCount int, insert ...T) ([]T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.splice(start, deleteCount, insert...)
}

// splice performs the splice operation without locking.
// Avoids the double-append pattern (append(a, append(b, c...)...)) which creates
// a temporary intermediate slice, and instead operates in-place with a single allocation.
func (s *Slice[T]) splice(start, deleteCount int, insert ...T) ([]T, error) {
	n := len(s.elements)
	if start < 0 || start > n {
		return nil, ErrIndexOutOfBounds
	}

	deleteCount = min(deleteCount, n-start)
	removed := make([]T, deleteCount)
	copy(removed, s.elements[start:start+deleteCount])

	diff := len(insert) - deleteCount
	switch {
	case diff == 0:
		// Exact replacement: copy in place, no resize needed
		copy(s.elements[start:], insert)
	case diff > 0:
		// Growing: extend slice, shift tail right, then insert
		s.elements = slices.Grow(s.elements, diff)[:n+diff]
		copy(s.elements[start+len(insert):], s.elements[start+deleteCount:n])
		copy(s.elements[start:], insert)
	default:
		// Shrinking: insert, shift tail left, zero out freed slots for GC
		copy(s.elements[start:], insert)
		copy(s.elements[start+len(insert):], s.elements[start+deleteCount:])
		newLen := n + diff
		clear(s.elements[newLen:n]) // zero out freed tail for GC
		s.elements = s.elements[:newLen]
	}

	return removed, nil
}

// Remove removes the first element in the slice that satisfies the conditional function.
// Uses copy instead of append for single-element removal and zeroes the freed slot.
func (s *Slice[T]) Remove(condition func(T) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, el := range s.elements {
		if condition(el) {
			copy(s.elements[i:], s.elements[i+1:])
			clear(s.elements[len(s.elements)-1:]) // zero out freed slot for GC
			s.elements = s.elements[:len(s.elements)-1]
			break
		}
	}
}

// RemoveAll removes elements from the slice that satisfy the provided condition function.
func (s *Slice[T]) RemoveAll(condition func(T) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	n := 0
	for _, el := range s.elements {
		if !condition(el) {
			s.elements[n] = el
			n++
		}
	}
	clear(s.elements[n:]) // zero out freed tail for GC
	s.elements = s.elements[:n]
}

// Range executes the provided function once for each slice element.
func (s *Slice[T]) Range(f func(T, int) bool, reverse ...bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(reverse) > 0 && reverse[0] {
		for i := len(s.elements) - 1; i >= 0; i-- {
			if !f(s.elements[i], i) {
				break
			}
		}
	} else {
		for index, element := range s.elements {
			if !f(element, index) {
				break
			}
		}
	}
}

// RangeAndSplice executes a function on each slice element and performs splice operations based on the function's return values.
// reverse is an optional parameter; if provided and true, the iteration will be in reverse order.
func (s *Slice[T]) RangeAndSplice(f func(T, int) (bool, int, int, []T), reverse ...bool) ([]T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(reverse) > 0 && reverse[0] {
		for i := len(s.elements) - 1; i >= 0; i-- {
			if condition, start, deleteCount, insert := f(s.elements[i], i); condition {
				return s.splice(start, deleteCount, insert...)
			}
		}
	} else {
		for i, element := range s.elements {
			if condition, start, deleteCount, insert := f(element, i); condition {
				return s.splice(start, deleteCount, insert...)
			}
		}
	}

	return nil, nil
}

// FindIndex returns the index of the first element that satisfies the provided function.
// If no element satisfies the function, it returns -1.
func (s *Slice[T]) FindIndex(condition func(T) bool) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for i, el := range s.elements {
		if condition(el) {
			return i
		}
	}
	return -1
}

// DoRead allows a custom read-only operation on the slice with a read lock.
func (s *Slice[T]) DoRead(op func([]T)) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	op(s.elements)
}

// DoWrite allows a custom write operation on the slice with a write lock.
func (s *Slice[T]) DoWrite(op func([]T) []T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.elements = op(s.elements)
}

// Replace replaces the slice elements with the given elements.
func (s *Slice[T]) Replace(elements []T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.elements = elements
}

// All returns a copy of all the elements in the slice.
func (s *Slice[T]) All() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return slices.Clone(s.elements)
}

// Clear removes all the elements in the slice.
func (s *Slice[T]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clear()
}

func (s *Slice[T]) clear() {
	clear(s.elements)           // zero out all elements for GC
	s.elements = s.elements[:0] // retain backing array for reuse
}

// AllAndClear returns all the elements in the slice and clears the slice.
func (s *Slice[T]) AllAndClear() []T {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := slices.Clone(s.elements)
	s.clear()
	return result
}

// Len returns the number of elements in the slice.
func (s *Slice[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.elements)
}
