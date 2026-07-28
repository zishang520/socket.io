package adapter

import (
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type fixedServerCountAdapter struct {
	socket.Adapter
	serverCount int64
}

func TestServerSideEmitResponseStoresScalarPacket(t *testing.T) {
	cluster := MakeClusterAdapter().(*clusterAdapter)
	request := &ClusterRequest{
		Expected:  2,
		Current:   new(atomic.Int64),
		Responses: types.NewSlice[any](),
	}
	cluster.requests.Store("request", request)

	cluster.OnResponse(&ClusterResponse{
		Type: SERVER_SIDE_EMIT_RESPONSE,
		Data: &ServerSideEmitResponse{
			RequestId: "request",
			Packet:    []any{"response"},
		},
	})

	responses := request.Responses.All()
	if len(responses) != 1 || responses[0] != "response" {
		t.Fatalf("expected scalar response, got %#v", responses)
	}
}

func (a *fixedServerCountAdapter) ServerCount() int64 {
	return a.serverCount
}

type testClusterAdapter struct {
	ClusterAdapter
	published atomic.Int64
}

func (a *testClusterAdapter) DoPublish(*ClusterMessage) (Offset, error) {
	a.published.Add(1)
	return "", nil
}

func (*testClusterAdapter) DoPublishResponse(ServerId, *ClusterResponse) error {
	return nil
}

func TestFetchSocketsImmediateTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := MakeClusterAdapter().(*clusterAdapter)
		cluster.Adapter = &fixedServerCountAdapter{
			Adapter:     socket.NewAdapter(nsp),
			serverCount: 2,
		}
		cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
		cluster.Construct(nsp)

		var timeoutErr error
		cluster.FetchSockets(&socket.BroadcastOptions{
			Flags: &socket.BroadcastFlags{Timeout: new(time.Duration(0))},
		})(func(_ []socket.SocketDetails, err error) {
			timeoutErr = err
		})
		time.Sleep(time.Millisecond)
		synctest.Wait()

		if timeoutErr == nil {
			t.Fatal("FetchSockets() returned nil error after timeout")
		}
		if cluster.requests.Len() != 0 {
			t.Fatal("FetchSockets() did not remove timed-out request")
		}
	})
}

func TestHeartbeatConcurrentPublishAndClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const heartbeatInterval = 10 * time.Millisecond

		opts := DefaultClusterAdapterOptions()
		opts.SetHeartbeatInterval(heartbeatInterval)

		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := NewClusterAdapterWithHeartbeat(nsp, opts).(*clusterAdapterWithHeartbeat)
		transport := &testClusterAdapter{ClusterAdapter: cluster}
		cluster.Prototype(transport)

		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() {
				cluster.Publish(&ClusterMessage{Type: BROADCAST})
			})
		}
		wg.Go(cluster.Close)
		wg.Wait()

		if got := transport.published.Load(); got != 101 {
			t.Fatalf("published messages = %d, want 101", got)
		}
		heartbeatTimer := cluster.heartbeatTimer.Load()
		if heartbeatTimer == nil {
			t.Fatal("heartbeat timer was not retained after Close()")
		}
		cluster.Publish(&ClusterMessage{Type: BROADCAST})
		if cluster.heartbeatTimer.Load() != heartbeatTimer {
			t.Fatal("heartbeat timer was replaced after Close()")
		}

		afterPublish := transport.published.Load()
		time.Sleep(2 * heartbeatInterval)
		synctest.Wait()
		if got := transport.published.Load(); got != afterPublish {
			t.Fatalf("heartbeat fired after Close(): got %d, want %d", got, afterPublish)
		}
	})
}

func TestHeartbeatCloseBeforePublish(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const heartbeatInterval = 10 * time.Millisecond

		opts := DefaultClusterAdapterOptions()
		opts.SetHeartbeatInterval(heartbeatInterval)

		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := NewClusterAdapterWithHeartbeat(nsp, opts).(*clusterAdapterWithHeartbeat)
		transport := &testClusterAdapter{ClusterAdapter: cluster}
		cluster.Prototype(transport)

		cluster.Close()
		heartbeatTimer := cluster.heartbeatTimer.Load()
		if heartbeatTimer == nil {
			t.Fatal("heartbeat timer was not retained after Close()")
		}

		cluster.Publish(&ClusterMessage{Type: BROADCAST})
		afterPublish := transport.published.Load()
		if cluster.heartbeatTimer.Load() != heartbeatTimer {
			t.Fatal("heartbeat timer was replaced after Close()")
		}

		time.Sleep(2 * heartbeatInterval)
		synctest.Wait()
		if got := transport.published.Load(); got != afterPublish {
			t.Fatalf("heartbeat fired after Close(): got %d, want %d", got, afterPublish)
		}
	})
}

func TestHeartbeatTimer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const heartbeatInterval = 10 * time.Millisecond

		opts := DefaultClusterAdapterOptions()
		opts.SetHeartbeatInterval(heartbeatInterval)

		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := NewClusterAdapterWithHeartbeat(nsp, opts).(*clusterAdapterWithHeartbeat)
		transport := &testClusterAdapter{ClusterAdapter: cluster}
		cluster.Prototype(transport)
		message := &ClusterMessage{Type: BROADCAST}

		if cluster.heartbeatTimer.Load() != nil {
			t.Fatal("heartbeat timer was created before Publish()")
		}
		cluster.Publish(message)

		time.Sleep(heartbeatInterval / 2)
		cluster.Publish(message)
		time.Sleep(heartbeatInterval - time.Nanosecond)
		synctest.Wait()
		beforeHeartbeat := transport.published.Load()

		time.Sleep(time.Nanosecond)
		synctest.Wait()
		afterHeartbeat := transport.published.Load()

		heartbeatTimer := cluster.heartbeatTimer.Load()
		cluster.Close()
		synctest.Wait()
		afterClose := transport.published.Load()
		cluster.Publish(message)
		afterClosedPublish := transport.published.Load()

		time.Sleep(2 * heartbeatInterval)
		synctest.Wait()
		afterClosedInterval := transport.published.Load()

		if beforeHeartbeat != 2 {
			t.Fatalf("published messages before interval = %d, want 2", beforeHeartbeat)
		}
		if afterHeartbeat != 3 {
			t.Fatalf("published messages after heartbeat = %d, want 3", afterHeartbeat)
		}
		if afterClose != 4 {
			t.Fatalf("published messages after Close() = %d, want 4", afterClose)
		}
		if afterClosedPublish != 5 {
			t.Fatalf("published messages after closed Publish() = %d, want 5", afterClosedPublish)
		}
		if afterClosedInterval != afterClosedPublish {
			t.Fatalf("published messages after closed heartbeat = %d, want %d", afterClosedInterval, afterClosedPublish)
		}
		if cluster.heartbeatTimer.Load() != heartbeatTimer {
			t.Fatal("heartbeat timer was replaced after Close()")
		}
	})
}
