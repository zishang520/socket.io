package socket

import (
	"github.com/zishang520/engine.io/events"
)

// Strictly typed version of an `EventEmitter`. A `TypedEventEmitter` takes type
// parameters for mappings of event names to event data types, and strictly
// types method calls to the `EventEmitter` according to these event maps.
type StrictEventEmitter struct {
	events.EventEmitter
}

func NewStrictEventEmitter() *StrictEventEmitter {
	return &StrictEventEmitter{EventEmitter: events.New()}
}

// Adds the `listener` function as an event listener for `ev`.
func (s *StrictEventEmitter) On(ev string, listeners ...events.Listener) error {
	return s.EventEmitter.On(events.EventName(ev), listeners...)
}

// Adds a one-time `listener` function as an event listener for `ev`.

func (s *StrictEventEmitter) Once(ev string, listeners ...events.Listener) error {
	return s.EventEmitter.Once(events.EventName(ev), listeners...)
}

// Emits an event.
func (s *StrictEventEmitter) Emit(ev string, args ...any) {
	s.EventEmitter.Emit(events.EventName(ev), args...)
}

// Emits a reserved event.
//
// This method is `protected`, so that only a class extending
// `StrictEventEmitter` can emit its own reserved events.
func (s *StrictEventEmitter) EmitReserved(ev string, args ...any) {
	s.EventEmitter.Emit(events.EventName(ev), args...)
}

// Emits an event.
//
// This method is `protected`, so that only a class extending
// `StrictEventEmitter` can get around the strict typing. This is useful for
// calling `emit.apply`, which can be called as `emitUntyped.apply`.
func (s *StrictEventEmitter) EmitUntyped(ev string, args ...any) {
	s.EventEmitter.Emit(events.EventName(ev), args...)
}

// Returns the listeners listening to an event.
func (s *StrictEventEmitter) Listeners(ev string) []events.Listener {
	return s.EventEmitter.Listeners(events.EventName(ev))
}
