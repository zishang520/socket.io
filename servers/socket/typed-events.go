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

// Adds the `listener` function as an event listener for `ev`.
//
// Param: ev Name of the event
//
// Param: listener Callback function
func (s *StrictEventEmitter) On(ev string, listeners ...types.EventListener) error {
	return s.EventEmitter.On(types.EventName(ev), listeners...)
}

// Adds a one-time `listener` function as an event listener for `ev`.
//
// Param: ev Name of the event
//
// Param: listener Callback function
func (s *StrictEventEmitter) Once(ev string, listeners ...types.EventListener) error {
	return s.EventEmitter.Once(types.EventName(ev), listeners...)
}

// Emits an event.
//
// Param: ev Name of the event
//
// Param: args Values to send to listeners of this event
func (s *StrictEventEmitter) Emit(ev string, args ...any) {
	s.EventEmitter.Emit(types.EventName(ev), args...)
}

// Emits a reserved event.
//
// This method is `protected`, so that only a class extending
// `StrictEventEmitter` can emit its own reserved events.
//
// Param: ev Reserved event name
//
// Param: args Arguments to emit along with the event
func (s *StrictEventEmitter) EmitReserved(ev string, args ...any) {
	s.EventEmitter.Emit(types.EventName(ev), args...)
}

// Emits an event.
//
// This method is `protected`, so that only a class extending
// `StrictEventEmitter` can get around the strict typing. This is useful for
// calling `emit.apply`, which can be called as `emitUntyped.apply`.
//
// Param: ev Event name
//
// Param: args Arguments to emit along with the event
func (s *StrictEventEmitter) EmitUntyped(ev string, args ...any) {
	s.EventEmitter.Emit(types.EventName(ev), args...)
}

// Returns the listeners listening to an event.
//
// Param: event Event name
//
// Returns: Slice of listeners subscribed to `event`
func (s *StrictEventEmitter) Listeners(ev string) []types.EventListener {
	return s.EventEmitter.Listeners(types.EventName(ev))
}
