package engine

import (
	"fmt"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestSocketForceCloseAfterTransportClosed(t *testing.T) {
	for _, protocol := range []int{3, 4} {
		for _, state := range []string{"open", "closing"} {
			t.Run(fmt.Sprintf("EIO%d_%s", protocol, state), func(t *testing.T) {
				ctx := types.NewHttpContext(httptest.NewRecorder(), httptest.NewRequest("GET", fmt.Sprintf("/?EIO=%d", protocol), nil))
				t.Cleanup(ctx.Flush)
				ctx.SetTransportReadPermission(make(chan bool))
				server := NewServer(nil)
				t.Cleanup(func() { server.Close() })

				// Initialization can resume after its deadline closed the transport,
				// before Socket has had a chance to install a close listener.
				transport := transports.NewTransport(ctx)
				transport.OnClose()
				conn := NewSocket("late-initialization", server, transport, ctx, protocol)
				ready := ctx.TakeTransportReady()
				if ready == nil {
					t.Fatal("stream initialization did not register heartbeat startup")
				}
				conn.SetReadyState(state)
				t.Cleanup(func() { conn.Close(true) })

				var closes atomic.Int32
				_ = conn.On("close", func(...any) {
					closes.Add(1)
					// A user close listener may close the same Socket again.
					conn.Close(true)
				})
				conn.Close(true)
				conn.Close(true)

				if got := conn.ReadyState(); got != "closed" {
					t.Fatalf("forced close left the Socket %q", got)
				}
				if got := closes.Load(); got != 1 {
					t.Fatalf("forced close emitted %d close events, want 1", got)
				}
				if !transport.Discarded() {
					t.Fatal("forced close did not discard the transport")
				}
				for _, event := range []types.EventName{"ready", "packet", "drain", "close"} {
					if got := transport.ListenerCount(event); got != 0 {
						t.Errorf("closed Socket retained %d transport %q listeners", got, event)
					}
				}

				// A late completion must not reactivate a canceled Socket. The
				// concrete value is inspected only to detect retained timer state;
				// all lifecycle actions above use the public Socket interface.
				ready()
				implementation := conn.(*socket)
				if implementation.pingIntervalTimer.Load() != nil || implementation.pingTimeoutTimer.Load() != nil {
					t.Fatal("canceled initialization installed a heartbeat timer")
				}
				if got := implementation.writeBuffer.Len(); got != 0 {
					t.Fatalf("closed Socket retained %d buffered packets", got)
				}
			})
		}
	}
}
