package types

type Optional[T any] interface {
	IsPresent() bool
	IsEmpty() bool
	Get() T
}

type Some[T any] struct {
	value T
}

func NewSome[T any](value T) Optional[T] {
	return &Some[T]{value: value}
}

// IsPresent returns true if this Some is not nil
func (s *Some[T]) IsPresent() bool { return s != nil }

// IsEmpty returns true if this Some is nil
func (s *Some[T]) IsEmpty() bool { return s == nil }

func (s *Some[T]) Get() T {
	if s == nil {
		var zero T
		return zero
	}
	return s.value
}
