package types

import (
	"reflect"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/zishang520/socket.io/v3/pkg/log"
)

const (
	// Version current version number
	EventVersion = "0.0.3"
)

var eventsLog = log.NewLog("engine:events")

type (
	// EventName is just a type of string, it's the event name
	EventName string
	// Listener is the type of a Listener, it's a func which receives any,optional, arguments from the caller/emmiter
	EventListener func(...any)
	// Events the type for registered listeners, it's just a map[string][]func(...any)
	Events map[EventName][]EventListener
	// EventEmitter is the message/or/event manager
	EventEmitter interface {
		// AddListener is an alias for .On(eventName, listener).
		AddListener(EventName, ...EventListener) error
		// Emit fires a particular event,
		// Synchronously calls each of the listeners registered for the event named
		// eventName, in the order they were registered,
		// passing the supplied arguments to each.
		Emit(EventName, ...any)
		// EventNames returns an array listing the events for which the emitter has registered listeners.
		// The values in the array will be strings.
		EventNames() []EventName
		// ListenerCount returns the length of all registered listeners to a particular event
		ListenerCount(EventName) int
		// Listeners returns a copy of the array of listeners for the event named eventName.
		Listeners(EventName) []EventListener
		// On registers a particular listener for an event, func receiver parameter(s) is/are optional
		On(EventName, ...EventListener) error
		// Once adds a one time listener function for the event named eventName.
		// The next time eventName is triggered, this listener is removed and then invoked.
		Once(EventName, ...EventListener) error
		// RemoveAllListeners removes all listeners, or those of the specified eventName.
		// Note that it will remove the event itself.
		// Returns an indicator if event and listeners were found before the remove.
		RemoveAllListeners(EventName) bool
		// RemoveListener removes given listener from the event named eventName.
		// Returns an indicator whether listener was removed
		// Use Subscribe when cleanup must identify one exact registration.
		RemoveListener(EventName, EventListener) bool
		// Clear removes all events and all listeners, restores Events to an empty value
		Clear()
		// Len returns the length of all registered events
		Len() int
	}

	eventEntry struct {
		fn           EventListener
		ptr          uintptr
		registration *listenerRegistration
	}

	eventEmitter struct {
		mu sync.RWMutex
		// Published entries never change: registration appends, removal copies.
		// Emit can therefore use its slice outside the lock.
		listeners map[EventName][]eventEntry
	}
)

// CopyTo copies the event listeners to an EventEmitter
func (e Events) CopyTo(emitter EventEmitter) {
	if len(e) > 0 {
		// register the events to/with their listeners
		for evt, listeners := range e {
			if len(listeners) > 0 {
				_ = emitter.AddListener(evt, listeners...)
			}
		}
	}
}

// New returns a new, empty, EventEmitter
func NewEventEmitter() EventEmitter {
	return &eventEmitter{}
}

// Subscribe registers listener and returns a function that removes it.
// For an emitter returned by NewEventEmitter, cleanup identifies this exact
// registration and is safe to call repeatedly, even when callbacks share a
// function body or receiver method. Other EventEmitter implementations use
// their own On and RemoveListener methods.
func Subscribe(emitter EventEmitter, evt EventName, listener EventListener) Callable {
	if e, ok := emitter.(*eventEmitter); ok {
		if listener == nil {
			return func() {}
		}
		registration := &listenerRegistration{evt: evt, emitter: e}
		e.mu.Lock()
		if e.listeners == nil {
			e.listeners = make(map[EventName][]eventEntry)
		}
		e.listeners[evt] = append(e.listeners[evt], eventEntry{
			fn: listener, ptr: reflect.ValueOf(listener).Pointer(), registration: registration,
		})
		e.mu.Unlock()
		return registration.remove
	}
	_ = emitter.On(evt, listener)
	return func() { emitter.RemoveListener(evt, listener) }
}

func (e *eventEmitter) addListeners(evt EventName, once bool, listeners []EventListener) error {
	if len(listeners) == 0 {
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	entries := e.listeners[evt]
	for _, listener := range listeners {
		if listener == nil {
			continue
		}
		entry := eventEntry{fn: listener, ptr: reflect.ValueOf(listener).Pointer()}
		if once {
			entry.registration = &listenerRegistration{evt: evt, emitter: e, fn: listener}
			entry.fn = entry.registration.executeOnce
		}
		entries = append(entries, entry)
	}
	if len(entries) == 0 {
		return nil
	}

	if e.listeners == nil {
		e.listeners = make(map[EventName][]eventEntry)
	}
	e.listeners[evt] = entries
	return nil
}

func (e *eventEmitter) AddListener(evt EventName, listeners ...EventListener) error {
	return e.addListeners(evt, false, listeners)
}

// Alias: [AddListener]
func (e *eventEmitter) On(evt EventName, listeners ...EventListener) error {
	return e.AddListener(evt, listeners...)
}

func (e *eventEmitter) snapshot(evt EventName) []eventEntry {
	e.mu.RLock()
	listeners := e.listeners[evt]
	e.mu.RUnlock()
	return listeners
}

func (e *eventEmitter) Emit(evt EventName, data ...any) {
	for _, event := range e.snapshot(evt) {
		executeEvent(event.fn, data...)
	}
}

func executeEvent(listener EventListener, data ...any) {
	defer func() {
		if r := recover(); r != nil {
			// Prevent a panicking listener from crashing the emitter.
			eventsLog.Errorf("event listener panic recovered: %v\n%s", r, debug.Stack())
		}
	}()
	listener(data...)
}

func (e *eventEmitter) EventNames() []EventName {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if len(e.listeners) == 0 {
		return nil
	}
	names := make([]EventName, 0, len(e.listeners))
	for name := range e.listeners {
		names = append(names, name)
	}
	return names
}

func (e *eventEmitter) ListenerCount(evt EventName) int {
	return len(e.snapshot(evt))
}

func (e *eventEmitter) Listeners(evt EventName) []EventListener {
	entries := e.snapshot(evt)
	if entries == nil {
		return nil
	}

	listeners := make([]EventListener, len(entries))
	for i, l := range entries {
		listeners[i] = l.fn
	}

	return listeners
}

type listenerRegistration struct {
	fired atomic.Bool

	evt     EventName
	emitter *eventEmitter
	fn      EventListener
}

func (l *listenerRegistration) remove() {
	l.emitter.removeListener(l.evt, func(entry eventEntry) bool { return entry.registration == l })
}

func (l *listenerRegistration) executeOnce(vals ...any) {
	if !l.fired.CompareAndSwap(false, true) {
		return
	}
	// Remove this registration, even if the same function was registered twice.
	l.remove()
	l.fn(vals...)
}

func (e *eventEmitter) Once(evt EventName, listeners ...EventListener) error {
	return e.addListeners(evt, true, listeners)
}

// RemoveListener removes the specified listener from the listener array for the event named eventName.
// It compares function entry points, which cannot distinguish closures or method
// receivers sharing the same code. Subscribe provides exact registration cleanup.
func (e *eventEmitter) RemoveListener(evt EventName, listener EventListener) bool {
	if listener == nil {
		return false
	}

	targetPtr := reflect.ValueOf(listener).Pointer()
	return e.removeListener(evt, func(entry eventEntry) bool { return entry.ptr == targetPtr })
}

func (e *eventEmitter) removeListener(evt EventName, match func(eventEntry) bool) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	entries := e.listeners[evt]
	index := slices.IndexFunc(entries, match)
	if index < 0 {
		return false
	}
	// Keep empty events registered, and never mutate a published snapshot.
	remaining := make([]eventEntry, len(entries)-1)
	copy(remaining, entries[:index])
	copy(remaining[index:], entries[index+1:])
	e.listeners[evt] = remaining
	return true
}

func (e *eventEmitter) RemoveAllListeners(evt EventName) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, loaded := e.listeners[evt]
	delete(e.listeners, evt)
	return loaded
}

func (e *eventEmitter) Clear() {
	e.mu.Lock()
	e.listeners = nil
	e.mu.Unlock()
}

func (e *eventEmitter) Len() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.listeners)
}
