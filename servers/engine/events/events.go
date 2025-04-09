// Source: https://github.com/kataras/go-events
// Package events provides simple EventEmitter support for Go Programming Language
package events

import (
	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

const (
	// Version current version number
	Version = types.EventVersion

	// DefaultMaxListeners is the number of max listeners per event
	// default EventEmitters will print a warning if more than x listeners are
	// added to it. This is a useful default which helps finding memory leaks.
	// Defaults to 0, which means unlimited
	//
	// Deprecated: No longer limit the number of event listeners.
	DefaultMaxListeners = types.EventDefaultMaxListeners
)

type (
	// EventName is just a type of string, it's the event name
	EventName = types.EventName
	// Listener is the type of a Listener, it's a func which receives any,optional, arguments from the caller/emmiter
	Listener = types.Listener
	// Events the type for registered listeners, it's just a map[string][]func(...any)
	Events = types.Events
	// EventEmitter is the message/or/event manager
	EventEmitter = types.EventEmitter
)

// New returns a new, empty, EventEmitter
func New() EventEmitter {
	return types.NewEventEmitter()
}

var defaultEventEmitter = New()

// SetMaxListeners sets the max listeners for the default event emitter
//
// Deprecated: No longer limit the number of event listeners.
func SetMaxListeners(n uint) {
	defaultEventEmitter.SetMaxListeners(n)
}

// GetMaxListeners returns the max listeners for the default event emitter
//
// Deprecated: No longer limit the number of event listeners.
func GetMaxListeners() uint {
	return defaultEventEmitter.GetMaxListeners()
}

// AddListener adds listeners to the default event emitter
func AddListener(evt EventName, listeners ...Listener) error {
	return defaultEventEmitter.AddListener(evt, listeners...)
}

// On is an alias for AddListener
func On(evt EventName, listeners ...Listener) error {
	return AddListener(evt, listeners...)
}

// Emit triggers an event on the default event emitter
func Emit(evt EventName, data ...any) {
	defaultEventEmitter.Emit(evt, data...)
}

// EventNames returns all the event names
func EventNames() []EventName {
	return defaultEventEmitter.EventNames()
}

// ListenerCount returns the number of listeners for an event
func ListenerCount(evt EventName) int {
	return defaultEventEmitter.ListenerCount(evt)
}

// Listeners returns all the listeners for an event
func Listeners(evt EventName) []Listener {
	return defaultEventEmitter.Listeners(evt)
}

// Once adds a one-time listener to the event emitter
func Once(evt EventName, listeners ...Listener) error {
	return defaultEventEmitter.Once(evt, listeners...)
}

// RemoveListener removes a listener from the event emitter
func RemoveListener(evt EventName, listener Listener) bool {
	return defaultEventEmitter.RemoveListener(evt, listener)
}

// RemoveAllListeners removes all listeners for an event
func RemoveAllListeners(evt EventName) bool {
	return defaultEventEmitter.RemoveAllListeners(evt)
}

// Clear removes all listeners and events
func Clear() {
	defaultEventEmitter.Clear()
}

// Len returns the total number of events currently registered
func Len() int {
	return defaultEventEmitter.Len()
}
