package socket

import (
	"sync"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

type broadcastFrame struct {
	types.BufferInterface

	mu sync.Mutex
	ws any
	wt any
}

func newBroadcastFrame(data types.BufferInterface) types.BufferInterface {
	if data == nil {
		return nil
	}
	return &broadcastFrame{BufferInterface: data}
}

func (f *broadcastFrame) PreparedWebSocketFrame(build func(types.BufferInterface) (any, error)) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.ws != nil {
		return f.ws, nil
	}
	prepared, err := build(f.BufferInterface)
	if err != nil {
		return nil, err
	}
	f.ws = prepared
	return prepared, nil
}

func (f *broadcastFrame) PreparedWebTransportFrame(build func(types.BufferInterface) (any, error)) (any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.wt != nil {
		return f.wt, nil
	}
	prepared, err := build(f.BufferInterface)
	if err != nil {
		return nil, err
	}
	f.wt = prepared
	return prepared, nil
}
