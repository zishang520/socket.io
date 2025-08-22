package types

type Optional[T any] interface {
	Get() T
}

type Some[T any] struct {
	value T
}

func NewSome[T any](value T) Optional[T] {
	return &Some[T]{value: value}
}

func (s *Some[T]) Get() T {
	return s.value
}
