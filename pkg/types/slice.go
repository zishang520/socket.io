package types

import (
	"errors"
	"sync"
)

// Define custom error types
var (
	ErrSliceEmpty        = errors.New("slice is empty")
	ErrIndexOutOfBounds  = errors.New("index out of bounds")
	ErrInvalidSliceRange = errors.New("invalid slice range")
)

// Slice is a generic type that holds elements of any type.
type Slice[T any] struct {
	mu       sync.RWMutex
	elements []T
}

// NewSlice creates and returns a new Slice.
func NewSlice[T any](elements ...T) *Slice[T] {
	return &Slice[T]{elements: elements}
}

// Push adds one or more elements to the end of the slice and returns the new length.
func (s *Slice[T]) Push(elements ...T) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.elements = append(s.elements, elements...)
	return len(s.elements)
}

// Unshift adds one or more elements to the beginning of the slice and returns the new length.
func (s *Slice[T]) Unshift(elements ...T) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.elements = append(elements, s.elements...)
	return len(s.elements)
}

// Pop removes the last element from the slice and returns it, or an error if the slice is empty.
func (s *Slice[T]) Pop() (element T, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.elements) == 0 {
		return element, ErrSliceEmpty
	}
	element = s.elements[len(s.elements)-1]
	s.elements = s.elements[:len(s.elements)-1]
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

	result := make([]T, end-start)
	copy(result, s.elements[start:end])
	return result, nil
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

func (s *Slice[T]) splice(start, deleteCount int, insert ...T) ([]T, error) {
	if start < 0 || start > len(s.elements) {
		return nil, ErrIndexOutOfBounds
	}

	deleteCount = min(deleteCount, len(s.elements)-start)
	removed := make([]T, deleteCount)
	copy(removed, s.elements[start:start+deleteCount])

	s.elements = append(s.elements[:start], append(insert, s.elements[start+deleteCount:]...)...)
	return removed, nil
}

// Remove removes the first element in the slice that satisfies the conditional function.
func (s *Slice[T]) Remove(condition func(T) bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i, el := range s.elements {
		if condition(el) {
			s.elements = append(s.elements[:i], s.elements[i+1:]...)
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
			element := s.elements[i]
			if condition, start, deleteCount, insert := f(element, i); condition {
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

	return s.all()
}

func (s *Slice[T]) all() []T {
	result := make([]T, len(s.elements))
	copy(result, s.elements)
	return result
}

// Clear removes all the elements in the slice.
func (s *Slice[T]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.clear()
}

func (s *Slice[T]) clear() {
	s.elements = s.elements[:0]
}

// AllAndClear returns all the elements in the slice and clears the slice.
func (s *Slice[T]) AllAndClear() []T {
	s.mu.Lock()
	defer s.mu.Unlock()

	defer s.clear()
	return s.all()
}

// Len returns the number of elements in the slice.
func (s *Slice[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.elements)
}
