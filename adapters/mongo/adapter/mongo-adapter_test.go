package adapter

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	clusteradapter "github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestSetOptsAcceptsClusterAdapterOptions(t *testing.T) {
	a := &mongoAdapter{
		heartbeatInterval: time.Second,
		heartbeatTimeout:  1_000,
		requestsTimeout:   2 * time.Second,
	}
	opts := clusteradapter.DefaultClusterAdapterOptions()
	opts.SetHeartbeatInterval(3 * time.Second)
	opts.SetHeartbeatTimeout(4_000)

	a.SetOpts(opts)

	if a.heartbeatInterval != 3*time.Second || a.heartbeatTimeout != 4_000 {
		t.Fatal("cluster adapter options were not applied")
	}
	if a.requestsTimeout != 2*time.Second {
		t.Fatal("cluster adapter options changed MongoDB-specific values")
	}
}

func TestConstructAppliesNodeDefaults(t *testing.T) {
	a := MakeMongoAdapter().(*mongoAdapter)
	opts := DefaultMongoAdapterOptions()
	opts.SetUid("")
	opts.SetRequestsTimeout(0)
	opts.SetHeartbeatInterval(0)
	opts.SetHeartbeatTimeout(0)
	a.SetOpts(opts)
	a.Construct(socket.NewServer(nil, nil).Sockets())

	if a.uid == "" {
		t.Fatal("uid was not generated")
	}
	if a.requestsTimeout != DefaultRequestsTimeout ||
		a.heartbeatInterval != DefaultHeartbeatInterval ||
		a.heartbeatTimeout != DefaultHeartbeatTimeout {
		t.Fatal("zero values did not fall back to Node.js defaults")
	}
}

func TestSetOptsMergesOnlyPresentValues(t *testing.T) {
	a := &mongoAdapter{
		uid:               "node-1",
		requestsTimeout:   time.Second,
		heartbeatInterval: time.Second,
		heartbeatTimeout:  1_000,
		addCreatedAtField: true,
	}
	opts := DefaultMongoAdapterOptions()
	opts.SetRequestsTimeout(2 * time.Second)
	opts.SetHeartbeatInterval(0)
	opts.SetAddCreatedAtField(false)

	a.SetOpts(opts)

	if a.uid != "node-1" || a.heartbeatTimeout != 1_000 {
		t.Fatal("unset options changed existing values")
	}
	if a.requestsTimeout != 2*time.Second || a.heartbeatInterval != 0 || a.addCreatedAtField {
		t.Fatal("present options were not applied")
	}
}

func TestServerSideEmitResponseKeepsScalarShape(t *testing.T) {
	a := &mongoAdapter{}
	resolved := make(chan []any, 1)
	request := &mongo.Request{
		Expected:  1,
		Responses: []any{},
		Resolve: func(values []any) {
			resolved <- values
		},
	}
	request.Timeout = utils.SetTimeout(func() {}, time.Hour)
	a.requests.Store("request", request)

	a.OnResponse(&ClusterResponse{
		Type: mongo.SERVER_SIDE_EMIT_RESPONSE,
		Data: &ServerSideEmitResponse{RequestId: "request", Packet: "value"},
	})

	values := <-resolved
	if len(values) != 1 || values[0] != "value" {
		t.Fatalf("unexpected responses: %#v", values)
	}
}

func TestRequestResponseCompletesOnceConcurrently(t *testing.T) {
	a := &mongoAdapter{}
	var calls atomic.Int32
	request := &mongo.Request{
		Expected:  1,
		Responses: []any{},
		Resolve: func([]any) {
			calls.Add(1)
		},
		Timeout: utils.SetTimeout(func() {}, time.Hour),
	}
	a.requests.Store("request", request)
	response := &ClusterResponse{
		Type: mongo.SERVER_SIDE_EMIT_RESPONSE,
		Data: &ServerSideEmitResponse{RequestId: "request", Packet: "value"},
	}

	var waitGroup sync.WaitGroup
	for range 32 {
		waitGroup.Go(func() {
			a.OnResponse(response)
		})
	}
	waitGroup.Wait()

	if calls.Load() != 1 {
		t.Fatalf("resolve calls = %d, want 1", calls.Load())
	}
}

func TestBroadcastAckKeepsScalarShape(t *testing.T) {
	a := &mongoAdapter{}
	response := make(chan []any, 1)
	a.ackRequests.Store("request", &mongo.AckRequest{
		Ack: func(values []any, _ error) {
			response <- values
		},
	})

	a.OnResponse(&ClusterResponse{
		Type: mongo.BROADCAST_ACK,
		Data: &BroadcastAck{RequestId: "request", Packet: "value"},
	})

	values := <-response
	if len(values) != 1 || values[0] != "value" {
		t.Fatalf("unexpected acknowledgement: %#v", values)
	}
}

func TestCloseRunsCleanupOnceConcurrently(t *testing.T) {
	a := &mongoAdapter{}
	var calls atomic.Int32
	a.Cleanup(func() {
		calls.Add(1)
	})

	var waitGroup sync.WaitGroup
	for range 32 {
		waitGroup.Go(a.Close)
	}
	waitGroup.Wait()

	if calls.Load() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", calls.Load())
	}
}

func TestHeartbeatSchedulingDoesNotSurviveClose(t *testing.T) {
	a := &mongoAdapter{heartbeatInterval: time.Hour}

	var waitGroup sync.WaitGroup
	for range 32 {
		waitGroup.Go(a.scheduleHeartbeat)
	}
	waitGroup.Go(a.Close)
	waitGroup.Wait()

	if timer := a.heartbeatTimer.Load(); timer != nil {
		timer.Stop()
		t.Fatal("heartbeat timer was retained after close")
	}
}

func TestHeartbeatSchedulingReusesTimer(t *testing.T) {
	a := &mongoAdapter{heartbeatInterval: time.Hour}
	a.scheduleHeartbeat()
	first := a.heartbeatTimer.Load()
	if first == nil {
		t.Fatal("heartbeat timer was not scheduled")
	}

	a.scheduleHeartbeat()
	if timer := a.heartbeatTimer.Load(); timer != first {
		t.Fatal("heartbeat timer was replaced instead of refreshed")
	}
	a.Close()
}

func TestServerCountRemovesExpiredNodes(t *testing.T) {
	a := &mongoAdapter{heartbeatTimeout: 10_000}
	a.nodesMap.Store("active", time.Now().UnixMilli())
	a.nodesMap.Store("expired", time.Now().Add(-11*time.Second).UnixMilli())

	if count, err := a.ServerCount(); err != nil || count != 2 {
		t.Fatalf("server count = %d, %v; want 2, nil", count, err)
	}
	if _, exists := a.nodesMap.Load("expired"); exists {
		t.Fatal("expired node was not removed")
	}
}
