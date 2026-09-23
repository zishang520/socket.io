package socket

import (
	"errors"
	"testing"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestNamespaceDestroyPreservesOtherSubscriptions(t *testing.T) {
	m := newTestManager()
	t.Cleanup(m.taskQueue.Close)
	first := m.Socket("/first", nil)
	second := m.Socket("/second", nil)
	first.subEvents()
	second.subEvents()
	t.Cleanup(first.destroy)
	t.Cleanup(second.destroy)
	firstErrors, secondErrors := 0, 0
	_ = first.On("connect_error", func(...any) { firstErrors++ })
	_ = second.On("connect_error", func(...any) { secondErrors++ })

	second.destroy()
	second.destroy()
	for _, event := range []types.EventName{"open", "packet", "error", "close"} {
		if got := m.ListenerCount(event); got != 1 {
			t.Fatalf("%s listener count = %d, want 1", event, got)
		}
	}
	m.Emit("error", errors.New("connection failed"))
	if firstErrors != 1 || secondErrors != 0 {
		t.Fatalf("errors reached first=%d second=%d, want first=1 second=0", firstErrors, secondErrors)
	}
	m.Emit("packet", &parser.Packet{Type: parser.EVENT, Nsp: "/first", Data: []any{"message", "payload"}})
	if first.ReceiveBuffer().Len() != 1 || second.ReceiveBuffer().Len() != 0 {
		t.Fatal("namespace destruction removed another namespace's packet subscription")
	}
}
