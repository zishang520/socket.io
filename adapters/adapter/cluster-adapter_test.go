package adapter

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const overflowingTimerMilliseconds float64 = 18_446_744_074_709

type fixedServerCountAdapter struct {
	socket.Adapter
	serverCount int64
}

type ackAdapter struct {
	socket.Adapter
	args              []any
	broadcasts        atomic.Int64
	broadcastsWithAck atomic.Int64
}

type fetchSocketsErrorAdapter struct {
	socket.Adapter
	err   error
	calls atomic.Int64
}

type scopedOperationAdapter struct {
	socket.Adapter
	calls atomic.Int64
}

type ackLifecycleAdapter struct {
	socket.Adapter
	broadcasts atomic.Int64
}

type blockingPublishAdapter struct {
	ClusterAdapter
	started chan *ClusterMessage
	release <-chan struct{}
}

type countedCloseReader struct {
	reader *bytes.Reader
	reads  atomic.Int64
	closes atomic.Int64
}

type prototypeClusterAdapter struct {
	ClusterAdapter
	count            int64
	countErr         error
	serverCountCalls atomic.Int64
	publishCount     atomic.Int64
	responseCount    atomic.Int64
	published        chan struct{}
}

func (a *prototypeClusterAdapter) Publish(*ClusterMessage) {
	a.publishCount.Add(1)
	if a.published != nil {
		a.published <- struct{}{}
	}
}

func (a *prototypeClusterAdapter) OnResponse(*ClusterResponse) {
	a.responseCount.Add(1)
}

func (a *prototypeClusterAdapter) ServerCount() (int64, error) {
	a.serverCountCalls.Add(1)
	return a.count, a.countErr
}

func (a *fetchSocketsErrorAdapter) FetchSockets(*socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(callback func([]socket.SocketDetails, error)) {
		a.calls.Add(1)
		callback([]socket.SocketDetails{nil}, a.err)
	}
}

func (a *scopedOperationAdapter) FetchSockets(*socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	a.calls.Add(1)
	return func(func([]socket.SocketDetails, error)) {}
}

func (a *scopedOperationAdapter) AddSockets(*socket.BroadcastOptions, []socket.Room) {
	a.calls.Add(1)
}

func (a *scopedOperationAdapter) DelSockets(*socket.BroadcastOptions, []socket.Room) {
	a.calls.Add(1)
}

func (a *scopedOperationAdapter) DisconnectSockets(*socket.BroadcastOptions, bool) {
	a.calls.Add(1)
}

func (a *ackAdapter) BroadcastWithAck(_ *parser.Packet, _ *socket.BroadcastOptions, clientCount func(uint64), ack socket.Ack) {
	a.broadcastsWithAck.Add(1)
	clientCount(1)
	ack(a.args, nil)
}

func (a *ackAdapter) Broadcast(*parser.Packet, *socket.BroadcastOptions) {
	a.broadcasts.Add(1)
}

func (a *ackLifecycleAdapter) BroadcastWithAck(packet *parser.Packet, opts *socket.BroadcastOptions, clientCount func(uint64), ack socket.Ack) {
	a.broadcasts.Add(1)
	a.Adapter.BroadcastWithAck(packet, opts, clientCount, ack)
}

func (a *blockingPublishAdapter) DoPublish(message *ClusterMessage) (Offset, error) {
	a.started <- message
	<-a.release
	return "", nil
}

func (r *countedCloseReader) Read(data []byte) (int, error) {
	r.reads.Add(1)
	return r.reader.Read(data)
}

func (r *countedCloseReader) Close() error {
	r.closes.Add(1)
	return nil
}

func newTestBroadcastClusterMessage() *ClusterMessage {
	return &ClusterMessage{
		Type: BROADCAST,
		Data: &BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
			Opts:   EncodeOptions(nil),
		},
	}
}

func TestClusterAdapterUsesPrototypeDispatch(t *testing.T) {
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := MakeClusterAdapter().(*clusterAdapter)
	cluster.Adapter = socket.NewAdapter(nsp)
	prototype := &prototypeClusterAdapter{
		ClusterAdapter: cluster,
		count:          1,
		published:      make(chan struct{}, 1),
	}
	cluster.Prototype(prototype)
	cluster.Construct(nsp)

	cluster.OnMessage(&ClusterMessage{
		Uid:  "remote",
		Nsp:  nsp.Name(),
		Type: BROADCAST_ACK,
	}, "")
	if prototype.responseCount.Load() != 1 {
		t.Fatal("OnMessage() did not dispatch the response to the prototype")
	}

	cluster.BroadcastWithAck(
		&parser.Packet{Type: parser.EVENT},
		&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: new(float64(1))}},
		func(uint64) {},
		func([]any, error) {},
	)
	select {
	case <-prototype.published:
	case <-time.After(time.Second):
		t.Fatal("BroadcastWithAck() did not publish through the prototype")
	}
}

func TestClusterRejectsMalformedBroadcastMessages(t *testing.T) {
	validMessage := func() *BroadcastMessage {
		requestId := "request"
		return &BroadcastMessage{
			Packet:    &parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
			Opts:      EncodeOptions(nil),
			RequestId: &requestId,
		}
	}

	tests := []struct {
		name string
		data func() any
	}{
		{name: "missing message", data: func() any { return nil }},
		{name: "typed nil message", data: func() any { return (*BroadcastMessage)(nil) }},
		{name: "missing packet", data: func() any {
			data := validMessage()
			data.Packet = nil
			return data
		}},
		{name: "missing options", data: func() any {
			data := validMessage()
			data.Opts = nil
			return data
		}},
		{name: "missing rooms", data: func() any {
			data := validMessage()
			data.Opts.Rooms = nil
			return data
		}},
		{name: "missing except rooms", data: func() any {
			data := validMessage()
			data.Opts.Except = nil
			return data
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
			local := &ackAdapter{Adapter: socket.NewAdapter(nsp)}
			cluster := MakeClusterAdapter().(*clusterAdapter)
			cluster.Adapter = local
			cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
			cluster.Construct(nsp)

			cluster.OnMessage(&ClusterMessage{
				Uid:  "remote",
				Nsp:  nsp.Name(),
				Type: BROADCAST,
				Data: test.data(),
			}, "")

			if got := local.broadcasts.Load(); got != 0 {
				t.Fatalf("local Broadcast() calls = %d, want 0", got)
			}
			if got := local.broadcastsWithAck.Load(); got != 0 {
				t.Fatalf("local BroadcastWithAck() calls = %d, want 0", got)
			}
		})
	}
}

func TestClusterRejectsMissingOptionsForScopedMessages(t *testing.T) {
	tests := []struct {
		name        string
		messageType MessageType
		data        any
	}{
		{name: "join", messageType: SOCKETS_JOIN, data: new(SocketsJoinLeaveMessage)},
		{name: "leave", messageType: SOCKETS_LEAVE, data: new(SocketsJoinLeaveMessage)},
		{name: "disconnect", messageType: DISCONNECT_SOCKETS, data: new(DisconnectSocketsMessage)},
		{name: "fetch", messageType: FETCH_SOCKETS, data: new(FetchSocketsMessage)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
			local := &scopedOperationAdapter{Adapter: socket.NewAdapter(nsp)}
			cluster := MakeClusterAdapter().(*clusterAdapter)
			cluster.Adapter = local
			cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
			cluster.Construct(nsp)

			cluster.OnMessage(&ClusterMessage{
				Uid:  "remote",
				Nsp:  nsp.Name(),
				Type: test.messageType,
				Data: test.data,
			}, "")

			if got := local.calls.Load(); got != 0 {
				t.Fatalf("local operation calls = %d, want 0", got)
			}
		})
	}
}

func TestClusterAdapterReturnsServerCountErrors(t *testing.T) {
	countErr := errors.New("count failed")
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := MakeClusterAdapter().(*clusterAdapter)
	cluster.Adapter = socket.NewAdapter(nsp)
	cluster.Prototype(&prototypeClusterAdapter{
		ClusterAdapter: cluster,
		countErr:       countErr,
	})
	cluster.Construct(nsp)

	var fetchErr error
	cluster.FetchSockets(nil)(func(_ []socket.SocketDetails, err error) {
		fetchErr = err
	})
	if !errors.Is(fetchErr, countErr) {
		t.Fatalf("FetchSockets() error = %v, want %v", fetchErr, countErr)
	}

	err := cluster.ServerSideEmit([]any{"event", func([]any, error) {}})
	if !errors.Is(err, countErr) {
		t.Fatalf("ServerSideEmit() error = %v, want %v", err, countErr)
	}
	if cluster.requests.Len() != 0 {
		t.Fatal("request was stored after server count failed")
	}
}

func TestClusterFetchSocketsReturnsLocalError(t *testing.T) {
	localErr := errors.New("local fetch failed")
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	local := &fetchSocketsErrorAdapter{Adapter: socket.NewAdapter(nsp), err: localErr}
	cluster := MakeClusterAdapter().(*clusterAdapter)
	cluster.Adapter = local
	transport := &prototypeClusterAdapter{ClusterAdapter: cluster, count: 2}
	cluster.Prototype(transport)
	cluster.Construct(nsp)

	var (
		callbackCount int
		sockets       []socket.SocketDetails
		gotErr        error
	)
	cluster.FetchSockets(nil)(func(result []socket.SocketDetails, err error) {
		callbackCount++
		sockets = result
		gotErr = err
	})

	if callbackCount != 1 {
		t.Fatalf("FetchSockets() callback count = %d, want 1", callbackCount)
	}
	if sockets != nil {
		t.Fatalf("FetchSockets() sockets = %#v, want nil", sockets)
	}
	if gotErr != localErr {
		t.Fatalf("FetchSockets() error = %v, want %v", gotErr, localErr)
	}
	if local.calls.Load() != 1 {
		t.Fatalf("local FetchSockets() calls = %d, want 1", local.calls.Load())
	}
	if transport.serverCountCalls.Load() != 0 {
		t.Fatalf("ServerCount() calls = %d, want 0", transport.serverCountCalls.Load())
	}
	if transport.publishCount.Load() != 0 {
		t.Fatalf("Publish() calls = %d, want 0", transport.publishCount.Load())
	}
	if cluster.requests.Len() != 0 {
		t.Fatal("request was stored after local FetchSockets() failed")
	}
}

func TestHeartbeatFetchSocketsReturnsLocalError(t *testing.T) {
	localErr := errors.New("local fetch failed")
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := NewClusterAdapterWithHeartbeat(nsp, nil).(*clusterAdapterWithHeartbeat)
	defer cluster.Close()

	local := &fetchSocketsErrorAdapter{Adapter: socket.NewAdapter(nsp), err: localErr}
	cluster.ClusterAdapter.(*clusterAdapter).Adapter = local
	transport := &testClusterAdapter{ClusterAdapter: cluster}
	cluster.Prototype(transport)
	cluster.nodesMap.Store("remote", time.Now().UnixMilli())

	var (
		callbackCount int
		sockets       []socket.SocketDetails
		gotErr        error
	)
	cluster.FetchSockets(nil)(func(result []socket.SocketDetails, err error) {
		callbackCount++
		sockets = result
		gotErr = err
	})

	if callbackCount != 1 {
		t.Fatalf("FetchSockets() callback count = %d, want 1", callbackCount)
	}
	if sockets != nil {
		t.Fatalf("FetchSockets() sockets = %#v, want nil", sockets)
	}
	if gotErr != localErr {
		t.Fatalf("FetchSockets() error = %v, want %v", gotErr, localErr)
	}
	if local.calls.Load() != 1 {
		t.Fatalf("local FetchSockets() calls = %d, want 1", local.calls.Load())
	}
	if transport.published.Load() != 0 {
		t.Fatalf("Publish() calls = %d, want 0", transport.published.Load())
	}
	if cluster.customRequests.Len() != 0 {
		t.Fatal("request was stored after local FetchSockets() failed")
	}
}

func TestServerSideEmitResponseStoresPacketValue(t *testing.T) {
	for _, packet := range []any{"response", []any{"response"}, nil} {
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
				Packet:    packet,
			},
		})

		responses := request.Responses.All()
		if len(responses) != 1 || !reflect.DeepEqual(responses[0], packet) {
			t.Fatalf("response = %#v, want %#v", responses, packet)
		}
	}
}

func TestClusterAckPacketWireShape(t *testing.T) {
	responseTypes := []struct {
		name     string
		value    func(any) any
		decoded  func() any
		packetOf func(any) any
	}{
		{
			name: "server-side emit",
			value: func(packet any) any {
				return &ServerSideEmitResponse{RequestId: "request", Packet: packet}
			},
			decoded:  func() any { return new(ServerSideEmitResponse) },
			packetOf: func(value any) any { return value.(*ServerSideEmitResponse).Packet },
		},
		{
			name: "broadcast acknowledgement",
			value: func(packet any) any {
				return &BroadcastAck{RequestId: "request", Packet: packet}
			},
			decoded:  func() any { return new(BroadcastAck) },
			packetOf: func(value any) any { return value.(*BroadcastAck).Packet },
		},
	}
	packets := []struct {
		name    string
		value   any
		json    string
		present bool
	}{
		{name: "scalar", value: "response", json: `,"packet":"response"`, present: true},
		{name: "array", value: []any{"response"}, json: `,"packet":["response"]`, present: true},
		{name: "undefined", present: true},
	}

	for _, responseType := range responseTypes {
		for _, packet := range packets {
			t.Run(responseType.name+"/"+packet.name, func(t *testing.T) {
				jsonData, err := json.Marshal(responseType.value(packet.value))
				if err != nil {
					t.Fatal(err)
				}
				if got, want := string(jsonData), `{"requestId":"request"`+packet.json+`}`; got != want {
					t.Fatalf("JSON response = %s, want Node.js shape %s", got, want)
				}
				jsonDecoded := responseType.decoded()
				if unmarshalErr := json.Unmarshal(jsonData, jsonDecoded); unmarshalErr != nil {
					t.Fatal(unmarshalErr)
				}
				if got := responseType.packetOf(jsonDecoded); !reflect.DeepEqual(got, packet.value) {
					t.Fatalf("decoded JSON packet = %#v, want %#v", got, packet.value)
				}

				msgpackData, err := msgpack.Marshal(responseType.value(packet.value))
				if err != nil {
					t.Fatal(err)
				}
				var msgpackWire map[string]any
				if unmarshalErr := msgpack.Unmarshal(msgpackData, &msgpackWire); unmarshalErr != nil {
					t.Fatal(unmarshalErr)
				}
				if wirePacket, present := msgpackWire["packet"]; present != packet.present || !reflect.DeepEqual(wirePacket, packet.value) {
					t.Fatalf("MessagePack packet = %#v, %t; want %#v, %t", wirePacket, present, packet.value, packet.present)
				}
				msgpackDecoded := responseType.decoded()
				if unmarshalErr := msgpack.Unmarshal(msgpackData, msgpackDecoded); unmarshalErr != nil {
					t.Fatal(unmarshalErr)
				}
				if got := responseType.packetOf(msgpackDecoded); !reflect.DeepEqual(got, packet.value) {
					t.Fatalf("decoded MessagePack packet = %#v, want %#v", got, packet.value)
				}
			})
		}
	}
}

func TestClusterMessageRequiredWireFields(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		fields []string
	}{
		{"cluster envelope", ClusterMessage{}, []string{"uid", "nsp", "type"}},
		{"broadcast", BroadcastMessage{}, []string{"opts", "packet"}},
		{"sockets join/leave", SocketsJoinLeaveMessage{}, []string{"opts", "rooms"}},
		{"disconnect sockets", DisconnectSocketsMessage{}, []string{"opts", "close"}},
		{"fetch sockets", FetchSocketsMessage{}, []string{"opts", "requestId"}},
		{"fetch sockets response", FetchSocketsResponse{}, []string{"requestId", "sockets"}},
		{"server-side emit", ServerSideEmitMessage{}, []string{"packet"}},
		{"server-side emit response", ServerSideEmitResponse{}, []string{"requestId"}},
		{"broadcast client count", BroadcastClientCount{}, []string{"requestId", "clientCount"}},
		{"broadcast acknowledgement", BroadcastAck{}, []string{"requestId"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			var jsonFields map[string]json.RawMessage
			if err = json.Unmarshal(jsonData, &jsonFields); err != nil {
				t.Fatal(err)
			}

			msgpackData, err := msgpack.Marshal(tt.value)
			if err != nil {
				t.Fatal(err)
			}
			var msgpackFields map[string]any
			if err = msgpack.Unmarshal(msgpackData, &msgpackFields); err != nil {
				t.Fatal(err)
			}

			for _, field := range tt.fields {
				if _, ok := jsonFields[field]; !ok {
					t.Fatalf("JSON field %q is missing", field)
				}
				if _, ok := msgpackFields[field]; !ok {
					t.Fatalf("MessagePack field %q is missing", field)
				}
			}
		})
	}
}

func TestServerSideEmitWithoutRemoteNodesReturnsEmptyResponses(t *testing.T) {
	cluster := MakeClusterAdapter().(*clusterAdapter)
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

func (a *fixedServerCountAdapter) ServerCount() (int64, error) {
	return a.serverCount, nil
}

type testClusterAdapter struct {
	ClusterAdapter
	published  atomic.Int64
	response   atomic.Pointer[ClusterResponse]
	publishErr error
	onPublish  func(*ClusterMessage)
	onResponse func(*ClusterResponse)
}

func (a *testClusterAdapter) DoPublish(message *ClusterMessage) (Offset, error) {
	a.published.Add(1)
	if a.onPublish != nil {
		a.onPublish(message)
	}
	return "", a.publishErr
}

func (a *testClusterAdapter) DoPublishResponse(_ ServerId, response *ClusterResponse) error {
	a.response.Store(response)
	if a.onResponse != nil {
		a.onResponse(response)
	}
	return nil
}

func newClusterPublishTestAdapter(t *testing.T, publishErr error) (*clusterAdapter, *testClusterAdapter) {
	t.Helper()
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := MakeClusterAdapter().(*clusterAdapter)
	cluster.Adapter = &fixedServerCountAdapter{
		Adapter:     socket.NewAdapter(nsp),
		serverCount: 2,
	}
	transport := &testClusterAdapter{ClusterAdapter: cluster, publishErr: publishErr}
	cluster.Prototype(transport)
	cluster.Construct(nsp)
	return cluster, transport
}

func TestClusterServerSideEmitReturnsPublishError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		publishErr := errors.New("publish failed")
		cluster, _ := newClusterPublishTestAdapter(t, publishErr)

		if err := cluster.ServerSideEmit([]any{"event"}); !errors.Is(err, publishErr) {
			t.Fatalf("ServerSideEmit() error = %v, want %v", err, publishErr)
		}

		var ackCalls atomic.Int64
		if err := cluster.ServerSideEmit([]any{"event", func([]any, error) {
			ackCalls.Add(1)
		}}); !errors.Is(err, publishErr) {
			t.Fatalf("ServerSideEmit() with ack error = %v, want %v", err, publishErr)
		}
		if cluster.requests.Len() != 0 {
			t.Fatal("request was retained after publish failed")
		}

		time.Sleep(DEFAULT_TIMEOUT)
		synctest.Wait()
		if ackCalls.Load() != 0 {
			t.Fatalf("acknowledgement calls = %d, want 0", ackCalls.Load())
		}
	})
}

func TestClusterServerSideEmitCompletionWinsPublishError(t *testing.T) {
	publishErr := errors.New("publish failed")
	cluster, transport := newClusterPublishTestAdapter(t, publishErr)

	ackStarted := make(chan struct{})
	releaseAck := make(chan struct{})
	ackResult := make(chan []any, 1)
	transport.onPublish = func(message *ClusterMessage) {
		data, ok := message.Data.(*ServerSideEmitMessage)
		if !ok || data.RequestId == nil {
			return
		}
		go cluster.OnResponse(&ClusterResponse{
			Type: SERVER_SIDE_EMIT_RESPONSE,
			Data: &ServerSideEmitResponse{
				RequestId: *data.RequestId,
				Packet:    "response",
			},
		})
		<-ackStarted
	}

	returned := make(chan error, 1)
	go func() {
		returned <- cluster.ServerSideEmit([]any{"event", func(args []any, err error) {
			if err != nil {
				t.Errorf("unexpected acknowledgement error: %v", err)
			}
			close(ackStarted)
			<-releaseAck
			ackResult <- args
		}})
	}()

	var err error
	select {
	case err = <-returned:
	case <-time.After(time.Second):
		close(releaseAck)
		t.Fatal("ServerSideEmit() blocked on the acknowledgement callback")
	}
	close(releaseAck)
	responses := <-ackResult
	if err != nil {
		t.Fatalf("ServerSideEmit() error = %v, want nil after response completed", err)
	}
	if len(responses) != 1 || responses[0] != "response" {
		t.Fatalf("acknowledgement = %#v, want [response]", responses)
	}
	if cluster.requests.Len() != 0 {
		t.Fatal("completed request was retained")
	}
}

func TestClusterFetchSocketsReturnsPublishError(t *testing.T) {
	publishErr := errors.New("publish failed")
	cluster, _ := newClusterPublishTestAdapter(t, publishErr)

	var gotErr error
	cluster.FetchSockets(nil)(func(_ []socket.SocketDetails, err error) {
		gotErr = err
	})
	if !errors.Is(gotErr, publishErr) {
		t.Fatalf("FetchSockets() error = %v, want %v", gotErr, publishErr)
	}
	if cluster.requests.Len() != 0 {
		t.Fatal("request was retained after publish failed")
	}
}

func TestClusterBroadcastAckDoesNotWaitForPublish(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		local := &ackLifecycleAdapter{Adapter: socket.NewAdapter(nsp)}
		cluster := MakeClusterAdapter().(*clusterAdapter)
		cluster.Adapter = local

		release := make(chan struct{})
		var releaseOnce sync.Once
		releasePublish := func() {
			releaseOnce.Do(func() { close(release) })
		}
		defer releasePublish()

		transport := &blockingPublishAdapter{
			ClusterAdapter: cluster,
			started:        make(chan *ClusterMessage, 1),
			release:        release,
		}
		cluster.Prototype(transport)
		cluster.Construct(nsp)

		timeout := float64(10)
		packet := &parser.Packet{Type: parser.EVENT, Data: []any{"event"}}
		returned := make(chan struct{})
		var remoteAcks atomic.Int64
		go func() {
			cluster.BroadcastWithAck(
				packet,
				&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: &timeout}},
				func(uint64) {},
				func([]any, error) { remoteAcks.Add(1) },
			)
			close(returned)
		}()

		synctest.Wait()
		select {
		case <-returned:
		default:
			t.Fatal("BroadcastWithAck() waited for the transport publish")
		}
		if local.broadcasts.Load() != 1 || packet.Id == nil {
			t.Fatal("local BroadcastWithAck() did not run immediately")
		}

		var published *ClusterMessage
		select {
		case published = <-transport.started:
		default:
			t.Fatal("transport publish was not started")
		}
		requestId := *published.Data.(*BroadcastMessage).RequestId
		if cluster.ackRequests.Len() != 1 {
			t.Fatal("acknowledgement request was not stored")
		}

		time.Sleep(10 * time.Millisecond)
		synctest.Wait()
		if cluster.ackRequests.Len() != 0 {
			t.Fatal("acknowledgement request outlived its timeout while publish was blocked")
		}

		cluster.OnResponse(&ClusterResponse{
			Type: BROADCAST_ACK,
			Data: &BroadcastAck{RequestId: requestId, Packet: "late"},
		})
		if remoteAcks.Load() != 0 {
			t.Fatal("late acknowledgement was delivered after request cleanup")
		}

		releasePublish()
		synctest.Wait()
	})
}

func TestClusterBroadcastAckPreservesPublishOrder(t *testing.T) {
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := MakeClusterAdapter().(*clusterAdapter)
	cluster.Adapter = &ackLifecycleAdapter{Adapter: socket.NewAdapter(nsp)}
	release := make(chan struct{})
	transport := &blockingPublishAdapter{
		ClusterAdapter: cluster,
		started:        make(chan *ClusterMessage, 2),
		release:        release,
	}
	cluster.Prototype(transport)
	cluster.Construct(nsp)
	defer cluster.Close()

	timeout := float64(10_000)
	cluster.BroadcastWithAck(
		&parser.Packet{Type: parser.EVENT, Data: []any{"first"}},
		&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: &timeout}},
		func(uint64) {},
		func([]any, error) {},
	)

	first := <-transport.started
	firstPacket := first.Data.(*BroadcastMessage).Packet.Data.([]any)
	if firstPacket[0] != "first" {
		t.Fatalf("first published packet = %#v", firstPacket)
	}

	secondDone := make(chan struct{})
	secondStarted := make(chan struct{})
	go func() {
		close(secondStarted)
		cluster.Broadcast(
			&parser.Packet{Type: parser.EVENT, Data: []any{"second"}},
			&socket.BroadcastOptions{},
		)
		close(secondDone)
	}()
	<-secondStarted
	select {
	case message := <-transport.started:
		t.Fatalf("second publish overtook the blocked first publish: %#v", message)
	default:
	}

	close(release)
	second := <-transport.started
	secondPacket := second.Data.(*BroadcastMessage).Packet.Data.([]any)
	if secondPacket[0] != "second" {
		t.Fatalf("second published packet = %#v", secondPacket)
	}
	select {
	case <-secondDone:
	case <-time.After(time.Second):
		t.Fatal("second broadcast did not finish")
	}
}

func TestClusterPublishResponseBypassesBlockedPublishAndCloseIsFinal(t *testing.T) {
	type publishedEvent struct {
		kind        string
		messageType MessageType
	}

	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := MakeClusterAdapter().(*clusterAdapter)
	release := make(chan struct{})
	responseStarted := make(chan struct{})
	releaseResponse := make(chan struct{})
	published := make(chan publishedEvent, 4)
	transport := &testClusterAdapter{
		ClusterAdapter: cluster,
		onPublish: func(message *ClusterMessage) {
			published <- publishedEvent{kind: "message", messageType: message.Type}
			if message.Type == BROADCAST {
				<-release
			}
		},
		onResponse: func(response *ClusterResponse) {
			if response.Type == BROADCAST_CLIENT_COUNT {
				close(responseStarted)
				<-releaseResponse
			}
			published <- publishedEvent{kind: "response", messageType: response.Type}
		},
	}
	cluster.Prototype(transport)
	cluster.Construct(nsp)

	cluster.Publish(&ClusterMessage{
		Type: BROADCAST,
		Data: &BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"blocked"}},
			Opts:   EncodeOptions(nil),
		},
	})
	if event := <-published; event.kind != "message" || event.messageType != BROADCAST {
		t.Fatalf("first published event = %#v", event)
	}

	response := &ClusterResponse{
		Type: BROADCAST_ACK,
		Data: &BroadcastAck{RequestId: "request"},
	}
	cluster.PublishResponse("requester", response)
	select {
	case event := <-published:
		if event.kind != "response" || event.messageType != BROADCAST_ACK {
			t.Fatalf("published response = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("response waited for an unrelated blocked publish")
	}
	response.Type = SERVER_SIDE_EMIT_RESPONSE
	if got := transport.response.Load(); got == nil || got.Type != BROADCAST_ACK {
		t.Fatalf("published response changed after submit: %#v", got)
	}

	responseReturned := make(chan struct{})
	go func() {
		cluster.PublishResponse("requester", &ClusterResponse{
			Type: BROADCAST_CLIENT_COUNT,
			Data: &BroadcastClientCount{RequestId: "request"},
		})
		close(responseReturned)
	}()
	<-responseStarted
	select {
	case <-responseReturned:
	case <-time.After(time.Second):
		t.Fatal("PublishResponse() waited for the transport")
	}
	mutablePacket := map[string]any{"values": []any{"before"}}
	queuedResponse := &ClusterResponse{
		Type: BROADCAST_ACK,
		Data: &BroadcastAck{
			RequestId: "request",
			Packet:    mutablePacket,
		},
	}
	cluster.PublishResponse("requester", queuedResponse)
	queuedResponse.Type = SERVER_SIDE_EMIT_RESPONSE
	mutablePacket["values"].([]any)[0] = "after"

	cluster.closeWithMessage(&ClusterMessage{Type: ADAPTER_CLOSE})
	cluster.PublishResponse("requester", &ClusterResponse{
		Type: SERVER_SIDE_EMIT_RESPONSE,
		Data: &ServerSideEmitResponse{RequestId: "request"},
	})
	if _, err := cluster.PublishAndReturnOffset(&ClusterMessage{Type: HEARTBEAT}); !errors.Is(err, ErrAdapterClosed) {
		t.Fatalf("PublishAndReturnOffset() error after closeWithMessage = %v", err)
	}
	select {
	case event := <-published:
		t.Fatalf("unexpected event before release: %#v", event)
	default:
	}

	close(release)
	select {
	case event := <-published:
		t.Fatalf("ADAPTER_CLOSE overtook an in-flight response: %#v", event)
	default:
	}

	close(releaseResponse)
	for _, expected := range []publishedEvent{
		{kind: "response", messageType: BROADCAST_CLIENT_COUNT},
		{kind: "response", messageType: BROADCAST_ACK},
		{kind: "message", messageType: ADAPTER_CLOSE},
	} {
		select {
		case event := <-published:
			if event != expected {
				t.Fatalf("published event = %#v, want %#v", event, expected)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %#v", expected)
		}
	}
	lastResponse := transport.response.Load()
	if lastResponse == nil || lastResponse.Type != BROADCAST_ACK {
		t.Fatalf("last response = %#v, want BROADCAST_ACK", lastResponse)
	}
	ack := lastResponse.Data.(*BroadcastAck)
	values := ack.Packet.(map[string]any)["values"].([]any)
	if len(values) != 1 || values[0] != "before" {
		t.Fatalf("queued response packet = %#v, want immutable snapshot", ack.Packet)
	}
	select {
	case event := <-published:
		t.Fatalf("unexpected event after ADAPTER_CLOSE: %#v", event)
	default:
	}
}

func TestClusterPublishPanicReturnsErrorAndQueueContinues(t *testing.T) {
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := MakeClusterAdapter().(*clusterAdapter)
	transport := &testClusterAdapter{ClusterAdapter: cluster}
	var calls atomic.Int64
	transport.onPublish = func(*ClusterMessage) {
		if calls.Add(1) == 1 {
			panic("publish failed")
		}
	}
	cluster.Prototype(transport)
	cluster.Construct(nsp)
	defer cluster.Close()

	if _, err := cluster.PublishAndReturnOffset(&ClusterMessage{Type: HEARTBEAT}); err == nil {
		t.Fatal("PublishAndReturnOffset() did not return the transport panic")
	}
	if _, err := cluster.PublishAndReturnOffset(&ClusterMessage{Type: HEARTBEAT}); err != nil {
		t.Fatalf("PublishAndReturnOffset() after panic error = %v", err)
	}
}

func TestClusterTransportCallbackCanCloseAdapter(t *testing.T) {
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := MakeClusterAdapter().(*clusterAdapter)
	transport := &testClusterAdapter{ClusterAdapter: cluster}
	transport.onPublish = func(*ClusterMessage) {
		cluster.Close()
	}
	cluster.Prototype(transport)
	cluster.Construct(nsp)

	result := make(chan error, 1)
	go func() {
		_, err := cluster.PublishAndReturnOffset(&ClusterMessage{Type: HEARTBEAT})
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("PublishAndReturnOffset() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("transport callback deadlocked while closing the adapter")
	}
	if _, err := cluster.PublishAndReturnOffset(&ClusterMessage{Type: HEARTBEAT}); !errors.Is(err, ErrAdapterClosed) {
		t.Fatalf("publish after callback close error = %v, want ErrAdapterClosed", err)
	}
}

func TestClusterBroadcastAckSnapshotsPacketForAsyncPublish(t *testing.T) {
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	local := &ackLifecycleAdapter{Adapter: socket.NewAdapter(nsp)}
	cluster := MakeClusterAdapter().(*clusterAdapter)
	cluster.Adapter = local
	publishedMessages := make(chan *ClusterMessage, 1)
	transport := &testClusterAdapter{
		ClusterAdapter: cluster,
		onPublish: func(message *ClusterMessage) {
			publishedMessages <- message
		},
	}
	cluster.Prototype(transport)
	cluster.Construct(nsp)

	reader := &countedCloseReader{reader: bytes.NewReader([]byte("reader"))}
	mutable := map[string]any{"values": []any{"before"}}
	packet := &parser.Packet{
		Type: parser.EVENT,
		Data: []any{
			"event",
			reader,
			types.NewBytesBuffer([]byte("buffer")),
			mutable,
		},
	}
	timeout := float64(10_000)
	cluster.BroadcastWithAck(
		packet,
		&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: &timeout}},
		func(uint64) {},
		func([]any, error) {},
	)

	var published *ClusterMessage
	select {
	case published = <-publishedMessages:
	case <-time.After(time.Second):
		t.Fatal("transport publish was not started")
	}
	mutable["values"].([]any)[0] = "after"
	materializedReads := reader.reads.Load()
	if materializedReads == 0 || reader.closes.Load() != 1 {
		t.Fatalf("reader reads/closes = %d/%d, want non-zero/1", materializedReads, reader.closes.Load())
	}

	remotePacket := published.Data.(*BroadcastMessage).Packet
	if remotePacket == packet {
		t.Fatal("remote and local broadcasts share the mutable packet")
	}
	if local.broadcasts.Load() != 1 || packet.Nsp != nsp.Name() || packet.Id == nil {
		t.Fatalf("local packet fields = broadcasts %d, nsp %q, id %v", local.broadcasts.Load(), packet.Nsp, packet.Id)
	}
	localData := packet.Data.([]any)
	if !bytes.Equal(localData[1].([]byte), []byte("reader")) ||
		!bytes.Equal(localData[2].([]byte), []byte("buffer")) {
		t.Fatalf("local binary data = %#v", localData)
	}
	if remotePacket.Nsp != "" || remotePacket.Id != nil {
		t.Fatalf("remote packet inherited local fields: nsp %q, id %v", remotePacket.Nsp, remotePacket.Id)
	}

	payload, err := EncodeClusterMessage(published)
	if err != nil {
		t.Fatal(err)
	}
	if reader.reads.Load() != materializedReads || reader.closes.Load() != 1 {
		t.Fatal("reader was consumed again by the remote encoding path")
	}
	decoded, err := DecodeClusterMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	remoteData := decoded.Data.(*BroadcastMessage).Packet.Data.([]any)
	if !bytes.Equal(remoteData[1].([]byte), []byte("reader")) ||
		!bytes.Equal(remoteData[2].([]byte), []byte("buffer")) {
		t.Fatalf("remote binary data = %#v", remoteData)
	}
	remoteValues := remoteData[3].(map[string]any)["values"].([]any)
	if len(remoteValues) != 1 || remoteValues[0] != "before" {
		t.Fatalf("remote mutable data = %#v, want immutable snapshot", remoteData[3])
	}

	requestId := *published.Data.(*BroadcastMessage).RequestId
	cluster.ackRequests.Delete(requestId)
}

func TestClusterBroadcastAckCleanupWithoutTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := MakeClusterAdapter().(*clusterAdapter)
		cluster.Adapter = &ackAdapter{Adapter: socket.NewAdapter(nsp)}
		cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
		cluster.Construct(nsp)

		cluster.BroadcastWithAck(
			&parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
			&socket.BroadcastOptions{Flags: new(socket.BroadcastFlags)},
			func(uint64) {},
			func([]any, error) {},
		)
		if cluster.ackRequests.Len() != 1 {
			t.Fatal("acknowledgement request was not stored")
		}

		time.Sleep(time.Millisecond)
		synctest.Wait()
		if cluster.ackRequests.Len() != 0 {
			t.Fatal("acknowledgement request was not removed")
		}
	})
}

func TestClusterBroadcastAckNormalizesOverflowingTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := MakeClusterAdapter().(*clusterAdapter)
		cluster.Adapter = &ackAdapter{Adapter: socket.NewAdapter(nsp)}
		cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
		cluster.Construct(nsp)
		timeout := overflowingTimerMilliseconds

		cluster.BroadcastWithAck(
			&parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
			&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: &timeout}},
			func(uint64) {},
			func([]any, error) {},
		)
		if cluster.ackRequests.Len() != 1 {
			t.Fatal("acknowledgement request was not stored")
		}

		time.Sleep(time.Millisecond - time.Nanosecond)
		synctest.Wait()
		if cluster.ackRequests.Len() != 1 {
			t.Fatal("acknowledgement request expired before one millisecond")
		}

		time.Sleep(time.Nanosecond)
		synctest.Wait()
		if cluster.ackRequests.Len() != 0 {
			t.Fatal("overflowing acknowledgement timeout was not normalized to one millisecond")
		}
	})
}

func TestClusterBroadcastAckUsesFirstArgument(t *testing.T) {
	for _, test := range []struct {
		name string
		args []any
		want any
	}{
		{name: "scalar", args: []any{"first", "ignored"}, want: "first"},
		{name: "array", args: []any{[]any{"first", "second"}}, want: []any{"first", "second"}},
		{name: "empty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
			cluster := MakeClusterAdapter().(*clusterAdapter)
			cluster.Adapter = &ackAdapter{
				Adapter: socket.NewAdapter(nsp),
				args:    test.args,
			}
			responseReady := make(chan MessageType, 2)
			transport := &testClusterAdapter{
				ClusterAdapter: cluster,
				onResponse: func(response *ClusterResponse) {
					responseReady <- response.Type
				},
			}
			cluster.Prototype(transport)
			cluster.Construct(nsp)

			requestId := "request"
			cluster.OnMessage(&ClusterMessage{
				Uid:  "remote",
				Nsp:  nsp.Name(),
				Type: BROADCAST,
				Data: &BroadcastMessage{
					Packet:    &parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
					Opts:      EncodeOptions(nil),
					RequestId: &requestId,
				},
			}, "")
			for _, expected := range []MessageType{BROADCAST_CLIENT_COUNT, BROADCAST_ACK} {
				select {
				case messageType := <-responseReady:
					if messageType != expected {
						t.Fatalf("response type = %d, want %d", messageType, expected)
					}
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for broadcast acknowledgement")
				}
			}

			response := transport.response.Load()
			if response == nil || response.Type != BROADCAST_ACK {
				t.Fatalf("response = %#v, want broadcast acknowledgement", response)
			}
			packet := response.Data.(*BroadcastAck).Packet
			if !reflect.DeepEqual(packet, test.want) {
				t.Fatalf("packet = %#v, want %#v", packet, test.want)
			}
		})
	}
}

func TestClusterServerSideAckUsesFirstArgument(t *testing.T) {
	for _, test := range []struct {
		name string
		args []any
		want any
	}{
		{name: "scalar", args: []any{"first", "ignored"}, want: "first"},
		{name: "array", args: []any{[]any{"first", "second"}}, want: []any{"first", "second"}},
		{name: "empty"},
	} {
		t.Run(test.name, func(t *testing.T) {
			nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
			if err := nsp.On("event", func(args ...any) {
				args[len(args)-1].(socket.Ack)(test.args, nil)
			}); err != nil {
				t.Fatal(err)
			}

			cluster := MakeClusterAdapter().(*clusterAdapter)
			cluster.Adapter = socket.NewAdapter(nsp)
			responseReady := make(chan MessageType, 1)
			transport := &testClusterAdapter{
				ClusterAdapter: cluster,
				onResponse: func(response *ClusterResponse) {
					responseReady <- response.Type
				},
			}
			cluster.Prototype(transport)
			cluster.Construct(nsp)

			requestId := "request"
			cluster.OnMessage(&ClusterMessage{
				Uid:  "remote",
				Nsp:  nsp.Name(),
				Type: SERVER_SIDE_EMIT,
				Data: &ServerSideEmitMessage{
					RequestId: &requestId,
					Packet:    []any{"event"},
				},
			}, "")
			select {
			case <-responseReady:
			case <-time.After(time.Second):
				t.Fatal("timed out waiting for server-side acknowledgement")
			}

			response := transport.response.Load()
			if response == nil || response.Type != SERVER_SIDE_EMIT_RESPONSE {
				t.Fatalf("response = %#v, want server-side emit response", response)
			}
			packet := response.Data.(*ServerSideEmitResponse).Packet
			if !reflect.DeepEqual(packet, test.want) {
				t.Fatalf("packet = %#v, want %#v", packet, test.want)
			}
		})
	}
}

func TestClusterBroadcastAckWrapsPacketValue(t *testing.T) {
	for _, packet := range []any{"response", []any{"response"}, nil} {
		cluster := MakeClusterAdapter().(*clusterAdapter)
		var response []any
		cluster.ackRequests.Store("request", ClusterAckRequest{
			Ack: func(args []any, _ error) {
				response = args
			},
		})

		cluster.OnResponse(&ClusterResponse{
			Type: BROADCAST_ACK,
			Data: &BroadcastAck{RequestId: "request", Packet: packet},
		})

		if len(response) != 1 || !reflect.DeepEqual(response[0], packet) {
			t.Fatalf("acknowledgement = %#v, want one argument %#v", response, packet)
		}
	}
}

func TestFetchSocketsTimeout(t *testing.T) {
	for _, test := range []struct {
		name    string
		timeout *float64
		delay   time.Duration
	}{
		{name: "nil uses default", delay: DEFAULT_TIMEOUT},
		{name: "zero uses default", timeout: new(float64(0)), delay: DEFAULT_TIMEOUT},
		{name: "NaN uses default", timeout: new(math.NaN()), delay: DEFAULT_TIMEOUT},
		{name: "explicit timeout", timeout: new(float64(1)), delay: time.Millisecond},
		{name: "fractional timeout", timeout: new(16.5), delay: 16 * time.Millisecond},
		{name: "sub-millisecond timeout", timeout: new(0.5), delay: time.Millisecond},
		{name: "overflowing timeout", timeout: new(overflowingTimerMilliseconds), delay: time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
				cluster := MakeClusterAdapter().(*clusterAdapter)
				cluster.Adapter = &fixedServerCountAdapter{
					Adapter:     socket.NewAdapter(nsp),
					serverCount: 2,
				}
				cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
				cluster.Construct(nsp)

				result := make(chan error, 1)
				cluster.FetchSockets(&socket.BroadcastOptions{
					Flags: &socket.BroadcastFlags{Timeout: test.timeout},
				})(func(_ []socket.SocketDetails, err error) {
					result <- err
				})

				time.Sleep(test.delay - time.Nanosecond)
				synctest.Wait()
				select {
				case <-result:
					t.Fatal("FetchSockets() timed out too early")
				default:
				}
				if cluster.requests.Len() != 1 {
					t.Fatal("FetchSockets() removed the request too early")
				}

				time.Sleep(time.Nanosecond)
				synctest.Wait()
				select {
				case err := <-result:
					if err == nil {
						t.Fatal("FetchSockets() returned nil error after timeout")
					}
				default:
					t.Fatal("FetchSockets() did not time out")
				}
				if cluster.requests.Len() != 0 {
					t.Fatal("FetchSockets() did not remove the timed-out request")
				}
			})
		})
	}
}

func TestHeartbeatFetchSocketsTimeout(t *testing.T) {
	for _, test := range []struct {
		name    string
		timeout *float64
		delay   time.Duration
	}{
		{name: "nil uses default", delay: DEFAULT_TIMEOUT},
		{name: "zero uses default", timeout: new(float64(0)), delay: DEFAULT_TIMEOUT},
		{name: "NaN uses default", timeout: new(math.NaN()), delay: DEFAULT_TIMEOUT},
		{name: "fractional timeout", timeout: new(16.5), delay: 16 * time.Millisecond},
		{name: "sub-millisecond timeout", timeout: new(0.5), delay: time.Millisecond},
		{name: "overflowing timeout", timeout: new(overflowingTimerMilliseconds), delay: time.Millisecond},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
				cluster := NewClusterAdapterWithHeartbeat(nsp, nil).(*clusterAdapterWithHeartbeat)
				defer cluster.Close()
				cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
				cluster.nodesMap.Store("remote", time.Now().UnixMilli())

				result := make(chan error, 1)
				cluster.FetchSockets(&socket.BroadcastOptions{
					Flags: &socket.BroadcastFlags{Timeout: test.timeout},
				})(func(_ []socket.SocketDetails, err error) {
					result <- err
				})

				time.Sleep(test.delay - time.Nanosecond)
				synctest.Wait()
				select {
				case <-result:
					t.Fatal("FetchSockets() timed out too early")
				default:
				}

				time.Sleep(time.Nanosecond)
				synctest.Wait()
				select {
				case err := <-result:
					if err == nil {
						t.Fatal("FetchSockets() returned nil error after timeout")
					}
				default:
					t.Fatal("FetchSockets() did not time out")
				}
			})
		})
	}
}

func TestServerSideEmitTimeoutAndResponseCallOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := MakeClusterAdapter().(*clusterAdapter)
		cluster.Adapter = &fixedServerCountAdapter{
			Adapter:     socket.NewAdapter(nsp),
			serverCount: 2,
		}
		cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
		cluster.Construct(nsp)

		type result struct {
			responses []any
			err       error
		}
		results := make(chan result, 2)
		if err := cluster.ServerSideEmit([]any{"event", func(responses []any, err error) {
			results <- result{responses: responses, err: err}
		}}); err != nil {
			t.Fatalf("ServerSideEmit() error = %v", err)
		}

		requestIds := cluster.requests.Keys()
		if len(requestIds) != 1 {
			t.Fatalf("pending requests = %d, want 1", len(requestIds))
		}
		go func() {
			time.Sleep(DEFAULT_TIMEOUT)
			cluster.OnResponse(&ClusterResponse{
				Type: SERVER_SIDE_EMIT_RESPONSE,
				Data: &ServerSideEmitResponse{
					RequestId: requestIds[0],
					Packet:    "response",
				},
			})
		}()

		time.Sleep(DEFAULT_TIMEOUT)
		synctest.Wait()

		if len(results) != 1 {
			t.Fatalf("acknowledgement calls = %d, want 1", len(results))
		}
		got := <-results
		if got.err == nil {
			if len(got.responses) != 1 || got.responses[0] != "response" {
				t.Fatalf("successful responses = %#v, want [response]", got.responses)
			}
		} else if len(got.responses) > 1 || len(got.responses) == 1 && got.responses[0] != "response" {
			t.Fatalf("timed-out responses = %#v, want [] or [response]", got.responses)
		}
		if cluster.requests.Len() != 0 {
			t.Fatal("completed request was not removed")
		}
	})
}

func TestHeartbeatConcurrentPublishAndClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := NewClusterAdapterWithHeartbeat(nsp, nil).(*clusterAdapterWithHeartbeat)
		var lastPublished atomic.Int64
		transport := &testClusterAdapter{
			ClusterAdapter: cluster,
			onPublish: func(message *ClusterMessage) {
				lastPublished.Store(int64(message.Type))
			},
		}
		cluster.Prototype(transport)

		var wg sync.WaitGroup
		for range 100 {
			wg.Go(func() {
				cluster.Publish(newTestBroadcastClusterMessage())
			})
		}
		wg.Go(cluster.Close)
		wg.Wait()
		synctest.Wait()

		if got := transport.published.Load(); got < 1 || got > 101 {
			t.Fatalf("published messages = %d, want between 1 and 101", got)
		}
		if got := MessageType(lastPublished.Load()); got != ADAPTER_CLOSE {
			t.Fatalf("last published message = %d, want ADAPTER_CLOSE", got)
		}
		if cluster.heartbeatTimer.Load() != nil || cluster.cleanupTimer.Load() != nil {
			t.Fatal("Close() retained heartbeat timers")
		}

		beforePublish := transport.published.Load()
		cluster.Publish(newTestBroadcastClusterMessage())
		synctest.Wait()
		if got := transport.published.Load(); got != beforePublish {
			t.Fatalf("Publish() after Close changed count from %d to %d", beforePublish, got)
		}
	})
}

func TestHeartbeatCloseBeforePublish(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := NewClusterAdapterWithHeartbeat(nsp, nil).(*clusterAdapterWithHeartbeat)
		transport := &testClusterAdapter{ClusterAdapter: cluster}
		cluster.Prototype(transport)

		cluster.Close()
		synctest.Wait()
		if got := transport.published.Load(); got != 1 {
			t.Fatalf("published messages after Close() = %d, want 1", got)
		}
		if cluster.heartbeatTimer.Load() != nil || cluster.cleanupTimer.Load() != nil {
			t.Fatal("Close() retained heartbeat timers")
		}

		cluster.Publish(newTestBroadcastClusterMessage())
		synctest.Wait()
		if got := transport.published.Load(); got != 1 {
			t.Fatalf("published messages after closed Publish() = %d, want 1", got)
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
		message := newTestBroadcastClusterMessage()

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
		if afterClosedPublish != afterClose {
			t.Fatalf("published messages after closed Publish() = %d, want %d", afterClosedPublish, afterClose)
		}
		if afterClosedInterval != afterClosedPublish {
			t.Fatalf("published messages after closed heartbeat = %d, want %d", afterClosedInterval, afterClosedPublish)
		}
		if cluster.heartbeatTimer.Load() != nil || cluster.cleanupTimer.Load() != nil {
			t.Fatal("Close() retained heartbeat timers")
		}
	})
}
