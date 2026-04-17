// Package slices provides utilities for safe slice operations with generics
package slices

import "slices"

// Get safely retrieves an element from a slice with bounds checking
// Returns the element and true if successful, zero value and false otherwise
func Get[S ~[]E, E any](s S, idx int) (E, bool) {
	if idx < 0 || idx >= len(s) {
		var zero E
		return zero, false
	}
	return s[idx], true
}

// GetAny safely retrieves and type-asserts an element from []any slice
// Returns the converted element and true if successful, zero value and false otherwise
func GetAny[O any](vals []any, idx int) (O, bool) {
	var zero O
	if idx < 0 || idx >= len(vals) {
		return zero, false
	}
	v, ok := vals[idx].(O)
	return v, ok
}

// TryGet retrieves an element or returns zero value if index is out of bounds
func TryGet[S ~[]E, E any](s S, idx int) E {
	if idx < 0 || idx >= len(s) {
		var zero E
		return zero
	}
	return s[idx]
}

// TryGetAny retrieves and type-asserts an element from []any or returns zero value
func TryGetAny[O any](vals []any, idx int) O {
	var zero O
	if idx < 0 || idx >= len(vals) {
		return zero
	}
	if v, ok := vals[idx].(O); ok {
		return v
	}
	return zero
}

// GetWithDefault retrieves an element or returns a default value
func GetWithDefault[S ~[]E, E any](s S, idx int, defaultVal E) E {
	if idx < 0 || idx >= len(s) {
		return defaultVal
	}
	return s[idx]
}

// GetPtr returns a pointer to the element if it exists, nil otherwise
// Useful when you need to distinguish between zero value and missing element
func GetPtr[S ~[]E, E any](s S, idx int) *E {
	if idx < 0 || idx >= len(s) {
		return nil
	}
	return &s[idx]
}

// Slice returns a sub-slice with bounds checking
// Automatically adjusts start and end to valid ranges
func Slice[S ~[]E, E any](s S, start int) S {
	n := len(s)

	// fast path: normal positive index within bounds
	if start >= 0 {
		if start <= n {
			return s[start:n]
		}
		// start > n => empty slice
		return s[n:n]
	}

	// negative start
	start += n
	if start <= 0 {
		return s[0:n]
	}
	// now 0 < start < n
	return s[start:n]
}

// First returns the first element if slice is not empty
func First[S ~[]E, E any](s S) (E, bool) {
	if len(s) == 0 {
		var zero E
		return zero, false
	}
	return s[0], true
}

// Last returns the last element if slice is not empty
func Last[S ~[]E, E any](s S) (E, bool) {
	if len(s) == 0 {
		var zero E
		return zero, false
	}
	return s[len(s)-1], true
}

// Filter creates a new slice with elements that pass the test
func Filter[S ~[]E, E any](s S, predicate func(E) bool) S {
	result := make(S, 0, len(s)) // Pre-allocate with capacity
	for _, v := range s {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

// Map transforms each element and returns a new slice
func Map[T, R any](vals []T, transform func(T) R) []R {
	result := make([]R, len(vals)) // Pre-allocate with exact size
	for i, v := range vals {
		result[i] = transform(v)
	}
	return result
}

// Reduce applies a function against elements to reduce to a single value
func Reduce[T, R any](vals []T, initial R, reducer func(R, T) R) R {
	result := initial
	for _, v := range vals {
		result = reducer(result, v)
	}
	return result
}

// Contains reports whether the slice contains the given value.
func Contains[S ~[]E, E comparable](s S, val E) bool {
	return slices.Contains(s, val)
}

// FindIndex returns the index of the first element satisfying the predicate,
// or -1 if no such element is found.
func FindIndex[S ~[]E, E any](s S, predicate func(E) bool) int {
	for i, v := range s {
		if predicate(v) {
			return i
		}
	}
	return -1
}

// Flatten concatenates a slice of slices into a single slice.
func Flatten[S ~[]E, E any](ss []S) S {
	var total int
	for _, s := range ss {
		total += len(s)
	}
	result := make(S, 0, total)
	for _, s := range ss {
		result = append(result, s...)
	}
	return result
}

// Unique returns a new slice with duplicate elements removed, preserving order.
func Unique[S ~[]E, E comparable](s S) S {
	seen := make(map[E]struct{}, len(s))
	result := make(S, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; !ok {
			seen[v] = struct{}{}
			result = append(result, v)
		}
	}
	return result
}

// IsEmpty checks if slice is nil or empty
func IsEmpty[S ~[]E, E any](s S) bool {
	return len(s) == 0
}

// IsValidIndex checks if index is valid for the slice.
//
//go:inline
func IsValidIndex[S ~[]E, E any](s S, idx int) bool {
	return uint(idx) < uint(len(s))
}
