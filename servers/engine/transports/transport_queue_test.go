package transports

import (
	"testing"
	"time"

	"github.com/zishang520/socket.io/v3/pkg/queue"
)

func TestPeerCloseStopsTransportWriteQueue(t *testing.T) {
	tests := []struct {
		name string
		new  func() (Transport, *queue.Queue)
	}{
		{
			name: "websocket",
			new: func() (Transport, *queue.Queue) {
				w := MakeWebSocket().(*websocket)
				w.writeQueue = queue.New()
				return w, w.writeQueue
			},
		},
		{
			name: "webtransport",
			new: func() (Transport, *queue.Queue) {
				w := MakeWebTransport().(*webTransport)
				w.writeQueue = queue.New()
				return w, w.writeQueue
			},
		},
		{
			name: "polling",
			new: func() (Transport, *queue.Queue) {
				p := MakePolling().(*polling)
				p.writeQueue = queue.New()
				return p, p.writeQueue
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transport, writeQueue := tc.new()
			t.Cleanup(writeQueue.TryClose)

			transport.OnClose()
			transport.Close()

			executed := make(chan struct{}, 1)
			writeQueue.Enqueue(func() { executed <- struct{}{} })

			select {
			case <-executed:
				t.Fatal("write queue accepted work after peer close")
			case <-time.After(25 * time.Millisecond):
			}
		})
	}
}
