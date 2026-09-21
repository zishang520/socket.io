package engine

import (
	"sync"
	"testing"

	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type pausedSocketCloseRegistration struct {
	transports.Transport
	entered chan struct{}
	resume  <-chan struct{}
}

func (t *pausedSocketCloseRegistration) Once(event types.EventName, listeners ...types.EventListener) error {
	err := t.Transport.Once(event, listeners...)
	if event == "close" {
		close(t.entered)
		<-t.resume
	}
	return err
}

func TestSocketCloseDuringTransportListenerRegistration(t *testing.T) {
	entered, resume, rawClosed := make(chan struct{}), make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(resume) })
	server := &initializationTimeoutServer{Server: MakeServer()}
	server.Prototype(server)
	server.Construct(initializationTimeoutOptions())
	var observed transports.Transport
	server.create = func(name string, ctx *types.HttpContext) (transports.Transport, error) {
		transport, err := server.Server.CreateTransport(name, ctx)
		if err != nil {
			return nil, err
		}
		observed = transport
		_ = ctx.Websocket.Once("close", func(...any) { close(rawClosed) })
		return &pausedSocketCloseRegistration{Transport: transport, entered: entered, resume: resume}, nil
	}
	peer, handled := initializationTimeoutClient(t, server, release)
	readinessWait(t, entered, "Socket close listener registration")
	// The real startup deadline closes the connection while construction is
	// paused after subscribing to close, before publishing its cleanup.
	initializationTimeoutDisconnected(t, peer)
	readinessWait(t, rawClosed, "startup deadline closing the raw connection")
	release()
	readinessWait(t, handled, "late Socket construction return")
	initializationTimeoutEmpty(t, server)
	for _, event := range []types.EventName{"ready", "packet", "drain"} {
		if count := observed.ListenerCount(event); count != 0 {
			t.Errorf("closed transport retained %d Socket %q listeners", count, event)
		}
	}
}
