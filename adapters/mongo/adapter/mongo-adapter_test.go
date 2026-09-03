package adapter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongod "go.mongodb.org/mongo-driver/v2/mongo"
)

type recordingAdapter struct {
	adapter.Adapter
	broadcasts atomic.Int32
	operations atomic.Int32
}

func (a *recordingAdapter) Broadcast(*parser.Packet, *socket.BroadcastOptions) {
	a.broadcasts.Add(1)
}

func (a *recordingAdapter) AddSockets(*socket.BroadcastOptions, []socket.Room) {
	a.operations.Add(1)
}

func (a *recordingAdapter) DelSockets(*socket.BroadcastOptions, []socket.Room) {
	a.operations.Add(1)
}

func (a *recordingAdapter) DisconnectSockets(*socket.BroadcastOptions, bool) {
	a.operations.Add(1)
}

func (a *recordingAdapter) FetchSockets(*socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	a.operations.Add(1)
	return func(callback func([]socket.SocketDetails, error)) {
		callback(nil, nil)
	}
}

func captureErrors(t *testing.T, client *mongo.MongoClient) <-chan error {
	t.Helper()
	events := make(chan error, 1)
	if err := client.On("error", func(args ...any) {
		events <- args[0].(error)
	}); err != nil {
		t.Fatal(err)
	}
	return events
}

func TestSetOptsAcceptsClusterAdapterOptions(t *testing.T) {
	a := &mongoAdapter{
		heartbeatInterval: time.Second,
		heartbeatTimeout:  1_000,
		requestsTimeout:   2 * time.Second,
	}
	opts := adapter.DefaultClusterAdapterOptions()
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

func TestSetOptsIgnoresTypedNil(t *testing.T) {
	a := &mongoAdapter{
		heartbeatInterval: time.Second,
		heartbeatTimeout:  1_000,
		requestsTimeout:   2 * time.Second,
	}
	var opts *MongoAdapterOptions

	a.SetOpts(opts)

	if a.heartbeatInterval != time.Second || a.heartbeatTimeout != 1_000 || a.requestsTimeout != 2*time.Second {
		t.Fatal("typed-nil options changed adapter values")
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

func TestMalformedBroadcastMessageIsIgnored(t *testing.T) {
	a := NewMongoAdapter(socket.NewServer(nil, nil).Sockets(), nil, nil).(*mongoAdapter)
	defer a.Close()

	for _, data := range []any{
		nil,
		(*BroadcastMessage)(nil),
		&BroadcastMessage{},
	} {
		a.OnMessage(&ClusterMessage{
			Uid:  "remote",
			Nsp:  "/",
			Type: mongo.BROADCAST,
			Data: data,
		}, "")
	}
}

func TestOnMessageRejectsInvalidPacketOptions(t *testing.T) {
	nsp := socket.NewServer(nil, nil).Sockets()
	local := &recordingAdapter{Adapter: adapter.NewAdapter(nsp)}
	a := &mongoAdapter{Adapter: local, uid: "local"}
	invalid := &adapter.PacketOptions{}

	for _, message := range []*ClusterMessage{
		{Type: mongo.BROADCAST, Data: &BroadcastMessage{Packet: &parser.Packet{}, Opts: invalid}},
		{Type: mongo.SOCKETS_JOIN, Data: &SocketsJoinLeaveMessage{Opts: invalid}},
		{Type: mongo.SOCKETS_LEAVE, Data: &SocketsJoinLeaveMessage{Opts: invalid}},
		{Type: mongo.DISCONNECT_SOCKETS, Data: &DisconnectSocketsMessage{Opts: invalid}},
		{Type: mongo.FETCH_SOCKETS, Data: &FetchSocketsMessage{Opts: invalid}},
	} {
		message.Uid = "remote"
		message.Nsp = "/"
		a.OnMessage(message, "")
	}

	if count := local.broadcasts.Load(); count != 0 {
		t.Fatalf("local broadcast count = %d, want 0", count)
	}
	if count := local.operations.Load(); count != 0 {
		t.Fatalf("local operation count = %d, want 0", count)
	}
}

func TestOnEventEmitsDecodeErrors(t *testing.T) {
	client, err := mongo.NewMongoClient(context.Background(), new(mongod.Collection))
	if err != nil {
		t.Fatal(err)
	}
	decodeErrors := captureErrors(t, client)

	a := NewMongoAdapter(socket.NewServer(nil, nil).Sockets(), client, nil).(*mongoAdapter)
	defer a.Close()
	a.OnEvent(&mongo.AdapterEvent{
		Uid:  "remote",
		Nsp:  "/",
		Type: mongo.BROADCAST,
		Data: bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: []byte{1}},
	})

	select {
	case <-decodeErrors:
	default:
		t.Fatal("decode error was not emitted")
	}
}

func TestOnEventSkipsHeartbeatAndSessionData(t *testing.T) {
	client, err := mongo.NewMongoClient(context.Background(), new(mongod.Collection))
	if err != nil {
		t.Fatal(err)
	}
	decodeErrors := captureErrors(t, client)
	a := NewMongoAdapter(socket.NewServer(nil, nil).Sockets(), client, nil).(*mongoAdapter)
	defer a.Close()

	for _, messageType := range []mongo.EventType{mongo.HEARTBEAT, mongo.SESSION} {
		a.OnEvent(&mongo.AdapterEvent{
			Uid:  "remote",
			Nsp:  "/",
			Type: messageType,
			Data: bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: []byte{1}},
		})
	}

	if _, ok := a.nodesMap.Load("remote"); !ok {
		t.Fatal("heartbeat/session did not refresh the remote node")
	}
	select {
	case err := <-decodeErrors:
		t.Fatalf("heartbeat/session data was decoded: %v", err)
	default:
	}
}

func TestMongoAdapterClosesWithClientContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client, err := mongo.NewMongoClient(ctx, new(mongod.Collection))
	if err != nil {
		t.Fatal(err)
	}
	var typedNilOpts *MongoAdapterOptions
	builder := &MongoAdapterBuilder{Mongo: client, Opts: typedNilOpts}
	// Keep this unit test independent from a live MongoDB change stream.
	builder.cancel = func() {}
	current := builder.New(socket.NewServer(nil, nil).Sockets()).(*mongoAdapter)
	current.scheduleHeartbeat()

	cancel()
	deadline := time.Now().Add(time.Second)
	for (!current.isClosed.Load() || current.heartbeatTimer.Load() != nil || builder.adapters.Len() != 0) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}

	if !current.isClosed.Load() {
		t.Fatal("adapter remained open after client context cancellation")
	}
	if current.heartbeatTimer.Load() != nil {
		t.Fatal("heartbeat timer survived client context cancellation")
	}
	if builder.adapters.Len() != 0 {
		t.Fatal("closed adapter remained registered in the builder")
	}
}

func TestBroadcastStopsWhenPublishFails(t *testing.T) {
	client, err := mongo.NewMongoClient(context.Background(), new(mongod.Collection))
	if err != nil {
		t.Fatal(err)
	}
	publishErrors := captureErrors(t, client)

	local := &recordingAdapter{}
	a := &mongoAdapter{
		Adapter:         local,
		mongoCollection: client,
	}
	a.isClosed.Store(true)
	a.Broadcast(&parser.Packet{Type: parser.EVENT}, nil)

	if count := local.broadcasts.Load(); count != 0 {
		t.Fatalf("local broadcast count = %d, want 0", count)
	}
	select {
	case publishErr := <-publishErrors:
		if !errors.Is(publishErr, adapter.ErrAdapterClosed) {
			t.Fatalf("publish error = %v, want %v", publishErr, adapter.ErrAdapterClosed)
		}
	case <-time.After(time.Second):
		t.Fatal("publish error was not emitted")
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
