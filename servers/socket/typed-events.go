package socket

import (
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Strictly typed version of an `EventEmitter`. A `TypedEventEmitter` takes type
// parameters for mappings of event names to event data types, and strictly
// types method calls to the `EventEmitter` according to these event maps.
type StrictEventEmitter struct {
	types.EventEmitter
}

func NewStrictEventEmitter() *StrictEventEmitter {
	return &StrictEventEmitter{EventEmitter: types.NewEventEmitter()}
}

// On adds the listener function as an event listener for the given event name.
func (s *StrictEventEmitter) On(ev string, listeners ...types.EventListener) error {
	return s.EventEmitter.On(types.EventName(ev), listeners...)
}

// Once adds a one-time listener function for the given event name.
func (s *StrictEventEmitter) Once(ev string, listeners ...types.EventListener) error {
	return s.EventEmitter.Once(types.EventName(ev), listeners...)
}

// Emit emits an event with the specified name and arguments to all listeners.
func (s *StrictEventEmitter) Emit(ev string, args ...any) {
	s.EventEmitter.Emit(types.EventName(ev), args...)
}

// EmitReserved emits a reserved event. Only subclasses should use this method.
func (s *StrictEventEmitter) EmitReserved(ev string, args ...any) {
	s.EventEmitter.Emit(types.EventName(ev), args...)
}

// EmitUntyped emits an event without strict type checking. Only subclasses should use this method.
func (s *StrictEventEmitter) EmitUntyped(ev string, args ...any) {
	s.EventEmitter.Emit(types.EventName(ev), args...)
}

// Listeners returns a slice of listeners subscribed to the given event name.
func (s *StrictEventEmitter) Listeners(ev string) []types.EventListener {
	return s.EventEmitter.Listeners(types.EventName(ev))
}
