package socket

import (
	"fmt"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// on registers an event listener for the given event name on the EventEmitter.
// It returns a Callable to remove the listener.
func on(evt types.EventEmitter, ev types.EventName, fn types.EventListener) types.Callable {
	evt.On(ev, fn)
	return func() {
		evt.RemoveListener(ev, fn)
	}
}

// extractValue extracts a value of type T from the map by key.
// Returns an error if the key is missing or the type does not match.
func extractValue[T any](m map[string]any, key string) (v T, err error) {
	if m == nil {
		return v, fmt.Errorf("map input cannot be nil")
	}
	val, ok := m[key]
	if !ok {
		return v, fmt.Errorf("missing '%s' field", key)
	}
	if v, ok := val.(T); ok {
		return v, nil
	}
	return v, fmt.Errorf("invalid type for '%s' field: expected %T, got %T", key, v, val)
}

// processHandshake parses a handshake map into a Handshake struct.
// Returns an error if required fields are missing or invalid.
func processHandshake(d map[string]any) (*Handshake, error) {
	if d == nil {
		return nil, fmt.Errorf("map input cannot be nil")
	}
	sid, err := extractValue[string](d, "sid")
	if err != nil {
		return nil, err
	}

	pid, _ := extractValue[string](d, "pid")

	return &Handshake{
		Sid: sid,
		Pid: pid,
	}, nil
}

// processExtendedError parses an error map into an ExtendedError struct.
// Returns an error if required fields are missing or invalid.
func processExtendedError(d map[string]any) (*ExtendedError, error) {
	if d == nil {
		return nil, fmt.Errorf("map input cannot be nil")
	}
	message, err := extractValue[string](d, "message")
	if err != nil {
		return nil, err
	}

	data, err := extractValue[any](d, "data")
	if err != nil {
		return nil, err
	}

	return &ExtendedError{
		Message: message,
		Data:    data,
	}, nil
}
