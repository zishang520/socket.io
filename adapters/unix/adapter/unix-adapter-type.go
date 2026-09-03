// Package adapter defines the Unix Domain Socket adapter and its builder.
package adapter

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/unix/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const (
	initialListenRetryDelay = 100 * time.Millisecond
	maxListenRetryDelay     = 5 * time.Second
)

type UnixAdapter interface {
	adapter.ClusterAdapterWithHeartbeat

	SetUnix(*unix.UnixClient)
	Cleanup(func())
}

// UnixAdapterBuilder shares one Unix listener across all namespace adapters.
type UnixAdapterBuilder struct {
	Unix *unix.UnixClient
	Opts UnixAdapterOptionsInterface

	mu                  sync.Mutex
	namespaceToAdapters types.Map[string, UnixAdapter]
	listening           bool
	retrying            bool
	retryPath           string
}

// New creates an adapter for a namespace and starts the shared listener once.
func (ub *UnixAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	name := nsp.Name()
	adapterInstance := NewUnixAdapter(nsp, ub.Unix, ub.Opts)
	stopContextClose := context.AfterFunc(ub.Unix.Context(), adapterInstance.Close)

	ub.mu.Lock()
	previous, replaced := ub.namespaceToAdapters.Swap(name, adapterInstance)
	startListening := false
	startRetry := false
	var listenErr error
	if !ub.listening && ub.Unix.Context().Err() == nil {
		listenerPath := ub.Unix.SocketPath() + "." + string(adapterInstance.Uid())
		listenErr = ub.listen(listenerPath)
		startListening = listenErr == nil
		if listenErr != nil && !ub.retrying {
			ub.retrying = true
			startRetry = true
		}
	}
	ub.mu.Unlock()

	if listenErr != nil {
		ub.Unix.Emit("error", listenErr)
	}
	if startListening {
		go ub.startListening()
	} else if startRetry {
		go ub.retryListen()
	}

	adapterInstance.Cleanup(func() {
		stopContextClose()
		ub.mu.Lock()
		ub.namespaceToAdapters.CompareAndDelete(name, adapterInstance)
		ub.mu.Unlock()
	})
	if replaced {
		previous.Close()
	}
	return adapterInstance
}

// listen transitions the builder from no listener to one active listener.
// ub.mu must be held by the caller.
func (ub *UnixAdapterBuilder) listen(listenerPath string) error {
	ub.retryPath = listenerPath
	if err := ub.Unix.Listen(listenerPath); err != nil {
		return err
	}
	ub.listening = true
	return nil
}

// retryListen is the sole background retry loop for a builder. Namespace
// creation may still make an immediate serialized attempt through New.
func (ub *UnixAdapterBuilder) retryListen() {
	delay := initialListenRetryDelay
	ctx := ub.Unix.Context()
	for {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			ub.mu.Lock()
			ub.retrying = false
			ub.mu.Unlock()
			return
		}

		ub.mu.Lock()
		if ub.listening || ctx.Err() != nil || ub.namespaceToAdapters.Len() == 0 {
			ub.retrying = false
			ub.mu.Unlock()
			return
		}
		err := ub.listen(ub.retryPath)
		if err == nil {
			ub.retrying = false
		}
		ub.mu.Unlock()

		if err != nil {
			if ctx.Err() == nil {
				ub.Unix.Emit("error", err)
			}
			delay = min(2*delay, maxListenRetryDelay)
			continue
		}
		ub.startListening()
		return
	}
}

// startListening reads complete framed messages until the shared client closes
// or the read loop encounters an error. A failed Listen never starts this loop.
func (ub *UnixAdapterBuilder) startListening() {
	for {
		payload, err := ub.Unix.ReadMessage()
		if err != nil {
			if ub.Unix.Context().Err() == nil {
				ub.Unix.Emit("error", err)
			}
			return
		}
		ub.dispatchMessage(payload)
	}
}

// dispatchMessage decodes once and routes the message to the exact namespace adapter.
func (ub *UnixAdapterBuilder) dispatchMessage(payload []byte) {
	message, err := adapter.DecodeClusterMessage(payload)
	if err != nil {
		ub.Unix.Emit("error", fmt.Errorf("failed to decode cluster message: %w", err))
		return
	}
	adapterInstance, ok := ub.namespaceToAdapters.Load(message.Nsp)
	if !ok {
		return
	}
	adapterInstance.OnMessage(message, "")
}
