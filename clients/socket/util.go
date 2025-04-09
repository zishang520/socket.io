package socket

import (
	"fmt"

	"github.com/zishang520/socket.io/servers/engine/v3/events"
	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

func on(evt events.EventEmitter, ev events.EventName, fn events.Listener) types.Callable {
	evt.On(ev, fn)
	return func() {
		evt.RemoveListener(ev, fn)
	}
}

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
