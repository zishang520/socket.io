package types

import (
	"reflect"
	"sync"
)

const (
	// Version current version number
	EventVersion = "0.0.3"
	// DefaultMaxListeners is the number of max listeners per event
	// default EventEmitters will print a warning if more than x listeners are
	// added to it. This is a useful default which helps finding memory leaks.
	// Defaults to 0, which means unlimited
	//
	// Deprecated: No longer limit the number of event listeners.
	EventDefaultMaxListeners = 0
)

type (
	// EventName is just a type of string, it's the event name
	EventName string
	// Listener is the type of a Listener, it's a func which receives any,optional, arguments from the caller/emmiter
	Listener func(...any)
	// Events the type for registered listeners, it's just a map[string][]func(...any)
	Events map[EventName][]Listener
	// EventEmitter is the message/or/event manager
	EventEmitter interface {
		// AddListener is an alias for .On(eventName, listener).
		AddListener(EventName, ...Listener) error
		// Emit fires a particular event,
		// Synchronously calls each of the listeners registered for the event named
		// eventName, in the order they were registered,
		// passing the supplied arguments to each.
		Emit(EventName, ...any)
		// EventNames returns an array listing the events for which the emitter has registered listeners.
		// The values in the array will be strings.
		EventNames() []EventName
		// GetMaxListeners returns the max listeners for this emmiter
		// see SetMaxListeners
		//
		// Deprecated: No longer limit the number of event listeners.
		GetMaxListeners() uint
		// ListenerCount returns the length of all registered listeners to a particular event
		ListenerCount(EventName) int
		// Listeners returns a copy of the array of listeners for the event named eventName.
		Listeners(EventName) []Listener
		// On registers a particular listener for an event, func receiver parameter(s) is/are optional
		On(EventName, ...Listener) error
		// Once adds a one time listener function for the event named eventName.
		// The next time eventName is triggered, this listener is removed and then invoked.
		Once(EventName, ...Listener) error
		// RemoveAllListeners removes all listeners, or those of the specified eventName.
		// Note that it will remove the event itself.
		// Returns an indicator if event and listeners were found before the remove.
		RemoveAllListeners(EventName) bool
		// RemoveListener removes given listener from the event named eventName.
		// Returns an indicator whether listener was removed
		RemoveListener(EventName, Listener) bool
		// Clear removes all events and all listeners, restores Events to an empty value
		Clear()
		// SetMaxListeners obviously this function allows the MaxListeners
		// to be decrease or increase. Set to zero for unlimited
		//
		// Deprecated: No longer limit the number of event listeners.
		SetMaxListeners(uint)
		// Len returns the length of all registered events
		Len() int
	}

	eventEntry struct {
		fn  Listener
		ptr uintptr
	}

	emmiter struct {
		evtListeners Map[EventName, *Slice[*eventEntry]]
	}
)

// CopyTo copies the event listeners to an EventEmitter
func (e Events) CopyTo(emitter EventEmitter) {
	if len(e) > 0 {
		// register the events to/with their listeners
		for evt, listeners := range e {
			if len(listeners) > 0 {
				emitter.AddListener(evt, listeners...)
			}
		}
	}
}

// New returns a new, empty, EventEmitter
func NewEventEmitter() EventEmitter {
	emmiter := &emmiter{
		evtListeners: Map[EventName, *Slice[*eventEntry]]{},
	}

	return emmiter
}

// Deprecated: No longer limit the number of event listeners.
func (e *emmiter) SetMaxListeners(n uint) {
}

// Deprecated: No longer limit the number of event listeners.
func (e *emmiter) GetMaxListeners() uint {
	return EventDefaultMaxListeners
}

func (e *emmiter) addListeners(evt EventName, listeners []*eventEntry) error {
	if len(listeners) == 0 {
		return nil
	}

	evtEntry, _ := e.evtListeners.LoadOrStore(evt, NewSlice[*eventEntry]())
	evtEntry.Push(listeners...)
	return nil
}

func (e *emmiter) AddListener(evt EventName, listeners ...Listener) error {
	if len(listeners) == 0 {
		return nil
	}

	events := make([]*eventEntry, len(listeners))
	for i, event := range listeners {
		if event == nil {
			continue
		}
		events[i] = &eventEntry{fn: event, ptr: reflect.ValueOf(event).Pointer()}
	}

	return e.addListeners(evt, events)
}

// Alias: [AddListener]
func (e *emmiter) On(evt EventName, listeners ...Listener) error {
	return e.AddListener(evt, listeners...)
}

func (e *emmiter) Emit(evt EventName, data ...any) {
	evtEntry, ok := e.evtListeners.Load(evt)
	if !ok {
		return
	}

	if evtEntry.Len() == 0 {
		return
	}

	for _, event := range evtEntry.All() {
		if event != nil {
			event.fn(data...)
		}
	}
}

func (e *emmiter) EventNames() []EventName {
	return e.evtListeners.Keys()
}

func (e *emmiter) ListenerCount(evt EventName) int {
	evtEntry, ok := e.evtListeners.Load(evt)
	if !ok {
		return 0
	}

	return evtEntry.Len()
}

func (e *emmiter) Listeners(evt EventName) []Listener {
	evtEntry, ok := e.evtListeners.Load(evt)
	if !ok {
		return nil
	}

	datas := evtEntry.All()
	listeners := make([]Listener, len(datas))
	for i, l := range datas {
		listeners[i] = l.fn
	}

	return listeners
}

type oneTimeListener struct {
	fired *sync.Once

	evt     EventName
	emitter *emmiter
	fn      Listener
}

func (l *oneTimeListener) execute(vals ...any) {
	l.fired.Do(func() {
		defer l.emitter.RemoveListener(l.evt, l.fn)
		l.fn(vals...)
	})
}

func (e *emmiter) Once(evt EventName, listeners ...Listener) error {
	if len(listeners) == 0 {
		return nil
	}

	events := make([]*eventEntry, len(listeners))
	for i, event := range listeners {
		if event == nil {
			continue
		}
		oneTime := &oneTimeListener{fired: &sync.Once{}, evt: evt, emitter: e, fn: event}
		events[i] = &eventEntry{fn: oneTime.execute, ptr: reflect.ValueOf(event).Pointer()}
	}
	return e.addListeners(evt, events)
}

// RemoveListener removes the specified listener from the listener array for the event named eventName.
func (e *emmiter) RemoveListener(evt EventName, listener Listener) bool {
	if listener == nil {
		return false
	}

	evtEntry, ok := e.evtListeners.Load(evt)

	if !ok {
		return false
	}

	if evtEntry.Len() == 0 {
		return false
	}

	targetPtr := reflect.ValueOf(listener).Pointer()

	remove, _ := evtEntry.RangeAndSplice(func(listener *eventEntry, i int) (bool, int, int, []*eventEntry) {
		return listener.ptr == targetPtr, i, 1, nil
	})
	return len(remove) > 0
}

func (e *emmiter) RemoveAllListeners(evt EventName) bool {
	_, loaded := e.evtListeners.LoadAndDelete(evt)
	return loaded
}

func (e *emmiter) Clear() {
	e.evtListeners.Clear()
}

func (e *emmiter) Len() int {
	return e.evtListeners.Len()
}
