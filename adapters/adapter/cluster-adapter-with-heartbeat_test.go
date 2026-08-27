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
	if cluster.registerRequest("request", request) {
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
