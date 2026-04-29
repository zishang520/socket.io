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

// None represents an absent optional value.
type None[T any] struct{}

// NewNone returns an Optional that holds no value.
func NewNone[T any]() Optional[T] {
	return &None[T]{}
}

func (n *None[T]) IsPresent() bool { return false }
func (n *None[T]) IsEmpty() bool   { return true }
func (n *None[T]) Get() T          { var zero T; return zero }
