package adapter

import (
	"encoding/json"
	"errors"
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

type fixedServerCountAdapter struct {
	socket.Adapter
	serverCount int64
}

type ackAdapter struct {
	socket.Adapter
	args []any
}

type prototypeClusterAdapter struct {
	ClusterAdapter
	count         int64
	countErr      error
	publishCount  atomic.Int64
	responseCount atomic.Int64
}

func (a *prototypeClusterAdapter) Publish(*ClusterMessage) {
	a.publishCount.Add(1)
}

func (a *prototypeClusterAdapter) OnResponse(*ClusterResponse) {
	a.responseCount.Add(1)
}

func (a *prototypeClusterAdapter) ServerCount() (int64, error) {
	return a.count, a.countErr
}

func (a *ackAdapter) BroadcastWithAck(_ *parser.Packet, _ *socket.BroadcastOptions, clientCount func(uint64), ack socket.Ack) {
	clientCount(1)
	ack(a.args, nil)
}

func TestClusterAdapterUsesPrototypeDispatch(t *testing.T) {
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	cluster := MakeClusterAdapter().(*clusterAdapter)
	cluster.Adapter = socket.NewAdapter(nsp)
	prototype := &prototypeClusterAdapter{ClusterAdapter: cluster, count: 1}
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
		&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: new(int64(1))}},
		func(uint64) {},
		func([]any, error) {},
	)
	if prototype.publishCount.Load() != 1 {
		t.Fatal("BroadcastWithAck() did not publish through the prototype")
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
	published atomic.Int64
	response  atomic.Pointer[ClusterResponse]
}

func (a *testClusterAdapter) DoPublish(*ClusterMessage) (Offset, error) {
	a.published.Add(1)
	return "", nil
}

func (a *testClusterAdapter) DoPublishResponse(_ ServerId, response *ClusterResponse) error {
	a.response.Store(response)
	return nil
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
			transport := &testClusterAdapter{ClusterAdapter: cluster}
			cluster.Prototype(transport)
			cluster.Construct(nsp)

			requestId := "request"
			cluster.OnMessage(&ClusterMessage{
				Uid:  "remote",
				Nsp:  nsp.Name(),
				Type: BROADCAST,
				Data: &BroadcastMessage{RequestId: &requestId},
			}, "")

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
			transport := &testClusterAdapter{ClusterAdapter: cluster}
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
		timeout int64
		delay   time.Duration
	}{
		{name: "zero uses default", delay: DEFAULT_TIMEOUT},
		{name: "explicit timeout", timeout: 1, delay: time.Millisecond},
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
					Flags: &socket.BroadcastFlags{Timeout: &test.timeout},
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

func TestHeartbeatFetchSocketsZeroTimeoutUsesDefault(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
		cluster := NewClusterAdapterWithHeartbeat(nsp, nil).(*clusterAdapterWithHeartbeat)
		defer cluster.Close()
		cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
		cluster.nodesMap.Store("remote", time.Now().UnixMilli())

		result := make(chan error, 1)
		cluster.FetchSockets(&socket.BroadcastOptions{
			Flags: &socket.BroadcastFlags{Timeout: new(int64(0))},
		})(func(_ []socket.SocketDetails, err error) {
			result <- err
		})

		time.Sleep(DEFAULT_TIMEOUT - time.Nanosecond)
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
