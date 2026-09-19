package engine

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
)

type closeTestTransport struct {
	Transport
	close func()
}

func (p *closeTestTransport) Close() Transport {
	p.close()
	return p
}

func TestSocketCloseClaimsStateBeforeTransportCleanup(t *testing.T) {
	for _, state := range []SocketState{SocketStateOpening, SocketStateOpen, SocketStateClosing} {
		t.Run(string(state), func(t *testing.T) {
			s := MakeSocketWithoutUpgrade().(*socketWithoutUpgrade)
			t.Cleanup(s.taskQueue.Close)
			s.opts = DefaultSocketOptions()
			s.readyState.Store(state)
			entered, release := make(chan struct{}), make(chan struct{})
			var cleanups, notifications atomic.Int32
			p := &closeTestTransport{Transport: MakeTransport(), close: func() {
				if cleanups.Add(1) == 1 {
					close(entered)
					<-release
					s.Close()
					s._onClose("reentrant close", nil)
				}
			}}
			p.Prototype(p)
			p.SetReadyState(TransportStateOpen)
			s.SetTransport(p)
			s.writeBuffer.Push(&packet.Packet{Type: packet.MESSAGE})
			_ = s.On("close", func(...any) {
				notifications.Add(1)
				if s.ReadyState() != SocketStateClosed || s.writeBuffer.Len() != 1 {
					t.Error("close listener must observe closed state and pending buffers")
				}
			})
			firstDone := make(chan struct{})
			go func() { s._onClose("transport close", nil); close(firstDone) }()
			<-entered
			s._onClose("ping timeout", nil)
			close(release)
			select {
			case <-firstDone:
			case <-time.After(time.Second):
				t.Fatal("reentrant close deadlocked")
			}
			if cleanups.Load() != 1 || notifications.Load() != 1 || s.writeBuffer.Len() != 0 {
				t.Fatalf("cleanups=%d close events=%d buffered=%d", cleanups.Load(), notifications.Load(), s.writeBuffer.Len())
			}
		})
	}
}
