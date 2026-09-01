package adapter

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestClusterAdapterOptionsAssignIgnoresTypedNil(t *testing.T) {
	target := DefaultClusterAdapterOptions()
	var source *ClusterAdapterOptions

	if result := target.Assign(source); result != target {
		t.Fatal("Assign() did not return the target options")
	}
}

func newHeartbeatPublishTestAdapter(t *testing.T, publishErr error) *clusterAdapterWithHeartbeat {
	t.Helper()
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := MakeClusterAdapterWithHeartbeat().(*clusterAdapterWithHeartbeat)
	cluster.ClusterAdapter.(*clusterAdapter).Adapter = socket.NewAdapter(nsp)
	transport := &testClusterAdapter{ClusterAdapter: cluster, publishErr: publishErr}
	cluster.Prototype(transport)
	cluster.Construct(nsp)
	cluster.nodesMap.Store("remote", time.Now().UnixMilli())
	return cluster
}

func TestHeartbeatServerSideEmitResponseStoresScalarPacket(t *testing.T) {
	cluster := MakeClusterAdapterWithHeartbeat().(*clusterAdapterWithHeartbeat)
	request := &CustomClusterRequest{
		MissingUids: types.NewSet[ServerId]("node", "other"),
		Responses:   types.NewSlice[any](),
	}
	cluster.customRequests.Store("request", request)

	cluster.OnResponse(&ClusterResponse{
		Uid:  "node",
		Type: SERVER_SIDE_EMIT_RESPONSE,
		Data: &ServerSideEmitResponse{
			RequestId: "request",
			Packet:    "response",
		},
	})

	responses := request.Responses.All()
	if len(responses) != 1 || responses[0] != "response" {
		t.Fatalf("expected scalar response, got %#v", responses)
	}
}

func TestHeartbeatServerSideEmitWithoutRemoteNodesReturnsEmptyResponses(t *testing.T) {
	cluster := MakeClusterAdapterWithHeartbeat().(*clusterAdapterWithHeartbeat)
	var responses []any

	err := cluster.ServerSideEmit([]any{"event", func(args []any, err error) {
		if err != nil {
			t.Errorf("unexpected acknowledgement error: %v", err)
		}
		responses = args
	}})
	if err != nil {
		t.Fatalf("ServerSideEmit() error = %v", err)
	}
	if responses == nil || len(responses) != 0 {
		t.Fatalf("acknowledgement responses = %#v, want non-nil empty slice", responses)
	}
}

func TestHeartbeatRegisterRequestReconcilesRemovedNode(t *testing.T) {
	cluster := MakeClusterAdapterWithHeartbeat().(*clusterAdapterWithHeartbeat)
	const uid ServerId = "node"
	cluster.nodesMap.Store(uid, 1)
	missingUids := types.NewSet(cluster.nodesMap.Keys()...)
	cluster.removeNode(uid)

	var calls atomic.Int64
	request := &CustomClusterRequest{
		Resolve:     func(*types.Slice[any]) { calls.Add(1) },
		Timeout:     new(atomic.Pointer[utils.Timer]),
		MissingUids: missingUids,
		Responses:   types.NewSlice[any](),
	}
	if cluster.registerRequest("request", request, time.Hour, func() {}) {
		t.Fatal("request remained pending for a removed node")
	}
	if calls.Load() != 1 || cluster.customRequests.Len() != 0 || request.MissingUids.Len() != 0 {
		t.Fatalf("Resolve()/pending/missing = %d/%d/%d, want 1/0/0", calls.Load(), cluster.customRequests.Len(), request.MissingUids.Len())
	}
}

func TestHeartbeatServerSideEmitReturnsPublishError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		publishErr := errors.New("publish failed")
		cluster := newHeartbeatPublishTestAdapter(t, publishErr)
		defer cluster.Close()

		if err := cluster.ServerSideEmit([]any{"event"}); !errors.Is(err, publishErr) {
			t.Fatalf("ServerSideEmit() error = %v, want %v", err, publishErr)
		}
		if cluster.heartbeatTimer.Load() == nil {
			t.Fatal("heartbeat was not scheduled for the failed publish")
		}

		var ackCalls atomic.Int64
		if err := cluster.ServerSideEmit([]any{"event", func([]any, error) {
			ackCalls.Add(1)
		}}); !errors.Is(err, publishErr) {
			t.Fatalf("ServerSideEmit() with ack error = %v, want %v", err, publishErr)
		}
		if cluster.customRequests.Len() != 0 {
			t.Fatal("request was retained after publish failed")
		}

		time.Sleep(DEFAULT_TIMEOUT)
		synctest.Wait()
		if ackCalls.Load() != 0 {
			t.Fatalf("acknowledgement calls = %d, want 0", ackCalls.Load())
		}
	})
}

func TestHeartbeatFetchSocketsReturnsPublishError(t *testing.T) {
	publishErr := errors.New("publish failed")
	cluster := newHeartbeatPublishTestAdapter(t, publishErr)
	defer cluster.Close()

	var gotErr error
	cluster.FetchSockets(nil)(func(_ []socket.SocketDetails, err error) {
		gotErr = err
	})
	if !errors.Is(gotErr, publishErr) {
		t.Fatalf("FetchSockets() error = %v, want %v", gotErr, publishErr)
	}
	if cluster.customRequests.Len() != 0 {
		t.Fatal("request was retained after publish failed")
	}
}

func TestHeartbeatDoesNotTrackEmptyUid(t *testing.T) {
	cluster := MakeClusterAdapterWithHeartbeat().(*clusterAdapterWithHeartbeat)
	cluster.ClusterAdapter.(*clusterAdapter).uid = "self"

	cluster.OnMessage(&ClusterMessage{
		Type: HEARTBEAT,
	}, "")

	if cluster.nodesMap.Len() != 0 {
		t.Fatal("message with empty uid was tracked as a cluster node")
	}
	if count, err := cluster.ServerCount(); err != nil || count != 1 {
		t.Fatalf("ServerCount() = %d, %v; want 1, nil", count, err)
	}
}

func TestHeartbeatCloseStopsPendingHeartbeatBeforeAdapterCloseCompletes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const heartbeatInterval = 10 * time.Millisecond

		opts := DefaultClusterAdapterOptions()
		opts.SetHeartbeatInterval(heartbeatInterval)
		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := NewClusterAdapterWithHeartbeat(nsp, opts).(*clusterAdapterWithHeartbeat)
		releaseClose := make(chan struct{})
		published := make(chan MessageType, 4)
		transport := &testClusterAdapter{
			ClusterAdapter: cluster,
			onPublish: func(message *ClusterMessage) {
				published <- message.Type
				if message.Type == ADAPTER_CLOSE {
					<-releaseClose
				}
			},
		}
		cluster.Prototype(transport)

		cluster.Publish(newTestBroadcastClusterMessage())
		synctest.Wait()
		if got := <-published; got != BROADCAST {
			t.Fatalf("first published message = %d, want BROADCAST", got)
		}

		closeDone := make(chan struct{})
		go func() {
			cluster.Close()
			close(closeDone)
		}()
		synctest.Wait()
		if got := <-published; got != ADAPTER_CLOSE {
			t.Fatalf("second published message = %d, want ADAPTER_CLOSE", got)
		}
		select {
		case <-closeDone:
		default:
			t.Fatal("Close() waited for the ADAPTER_CLOSE transport publish")
		}

		time.Sleep(2 * heartbeatInterval)
		synctest.Wait()
		select {
		case messageType := <-published:
			t.Fatalf("published message %d while ADAPTER_CLOSE was blocked", messageType)
		default:
		}

		close(releaseClose)
		synctest.Wait()
		select {
		case <-closeDone:
		default:
			t.Fatal("Close() did not complete")
		}
		select {
		case messageType := <-published:
			t.Fatalf("published message %d after ADAPTER_CLOSE", messageType)
		default:
		}
	})
}

func TestHeartbeatTransportCallbackCanCloseAdapter(t *testing.T) {
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := NewClusterAdapterWithHeartbeat(nsp, nil).(*clusterAdapterWithHeartbeat)
	published := make(chan MessageType, 2)
	transport := &testClusterAdapter{ClusterAdapter: cluster}
	transport.onPublish = func(message *ClusterMessage) {
		published <- message.Type
		if message.Type != ADAPTER_CLOSE {
			cluster.Close()
		}
	}
	cluster.Prototype(transport)

	result := make(chan error, 1)
	go func() {
		result <- cluster.ServerSideEmit([]any{"event"})
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("publish error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("transport callback deadlocked while closing the adapter")
	}

	for _, want := range []MessageType{SERVER_SIDE_EMIT, ADAPTER_CLOSE} {
		select {
		case got := <-published:
			if got != want {
				t.Fatalf("published message = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for published message %d", want)
		}
	}
}

func TestHeartbeatCleanupDoesNotRemoveRefreshedNode(t *testing.T) {
	cluster := MakeClusterAdapterWithHeartbeat().(*clusterAdapterWithHeartbeat)
	const (
		uid       ServerId = "node"
		lastSeen           = int64(1)
		refreshed          = int64(2)
	)
	cluster.nodesMap.Store(uid, lastSeen)
	cluster.nodesMap.Store(uid, refreshed)

	cluster.removeNode(uid, lastSeen)

	if got, ok := cluster.nodesMap.Load(uid); !ok || got != refreshed {
		t.Fatalf("refreshed node timestamp = %d, %v; want %d, true", got, ok, refreshed)
	}
}

func TestHeartbeatResponseAndRemoveNodeCallOnce(t *testing.T) {
	cluster := MakeClusterAdapterWithHeartbeat().(*clusterAdapterWithHeartbeat)
	const (
		requestId          = "request"
		uid       ServerId = "node"
	)
	var calls atomic.Int64
	request := &CustomClusterRequest{
		Type:        SERVER_SIDE_EMIT,
		Resolve:     func(*types.Slice[any]) { calls.Add(1) },
		Timeout:     new(atomic.Pointer[utils.Timer]),
		MissingUids: types.NewSet(uid),
		Responses:   types.NewSlice[any](),
	}
	cluster.nodesMap.Store(uid, 1)
	cluster.customRequests.Store(requestId, request)

	response := &ClusterResponse{
		Uid:  uid,
		Type: SERVER_SIDE_EMIT_RESPONSE,
		Data: &ServerSideEmitResponse{
			RequestId: requestId,
			Packet:    "response",
		},
	}
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			cluster.OnResponse(response)
		})
		wg.Go(func() {
			cluster.removeNode(uid)
		})
	}
	wg.Wait()

	if calls.Load() != 1 {
		t.Fatalf("Resolve() calls = %d, want 1", calls.Load())
	}
	if cluster.customRequests.Len() != 0 {
		t.Fatal("completed request was not removed")
	}
	for _, response := range request.Responses.All() {
		if response != "response" {
			t.Fatalf("response = %#v, want response", response)
		}
	}
}

func TestClusterAdapterWithHeartbeatBuilder(t *testing.T) {
	builder := &ClusterAdapterWithHeartbeatBuilder{
		Opts: nil,
	}

	cluster := builder.New(socket.NewNamespace(socket.NewServer(nil, nil), "/test"))
	t.Cleanup(cluster.Close)
}
