package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	baseadapter "github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type recordingParser struct {
	decodeCalled bool
	encodeErr    error
	packet       *Packet
}

func (p *recordingParser) Encode(any) ([]byte, error) { return nil, p.encodeErr }

func (p *recordingParser) Decode(_ []byte, value any) error {
	p.decodeCalled = true
	switch packet := value.(type) {
	case *Packet:
		*packet = *p.packet
	case **Packet:
		*packet = p.packet
	}
	return nil
}

type recordingLocalAdapter struct {
	socket.Adapter
	broadcasts        int
	broadcastsWithAck int
	adds              int
	dels              int
	disconnects       int
	broadcastStarted  chan struct{}
	broadcastRelease  chan struct{}
	invokeClientCount bool
}

type processErrorHook struct {
	err error
}

func (h *processErrorHook) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *processErrorHook) ProcessHook(rds.ProcessHook) rds.ProcessHook {
	return func(context.Context, rds.Cmder) error {
		return h.err
	}
}

func (h *processErrorHook) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func (a *recordingLocalAdapter) Broadcast(*parser.Packet, *socket.BroadcastOptions) {
	if a.broadcastStarted != nil {
		close(a.broadcastStarted)
		<-a.broadcastRelease
	}
	a.broadcasts++
}

func (a *recordingLocalAdapter) BroadcastWithAck(_ *parser.Packet, _ *socket.BroadcastOptions, clientCountCallback func(uint64), _ socket.Ack) {
	a.broadcastsWithAck++
	if a.invokeClientCount {
		clientCountCallback(0)
	}
}

func (a *recordingLocalAdapter) AddSockets(*socket.BroadcastOptions, []socket.Room) {
	a.adds++
}

func (a *recordingLocalAdapter) DelSockets(*socket.BroadcastOptions, []socket.Room) {
	a.dels++
}

func (a *recordingLocalAdapter) DisconnectSockets(*socket.BroadcastOptions, bool) {
	a.disconnects++
}

func TestClassicBroadcastStopsOnEncodeError(t *testing.T) {
	encodeErr := errors.New("encode failed")
	for _, test := range []struct {
		name string
		run  func(*redisAdapter, *parser.Packet)
	}{
		{
			name: "broadcast",
			run: func(adapter *redisAdapter, packet *parser.Packet) {
				adapter.Broadcast(packet, nil)
			},
		},
		{
			name: "broadcast with acknowledgement",
			run: func(adapter *redisAdapter, packet *parser.Packet) {
				adapter.BroadcastWithAck(packet, nil, func(uint64) {}, func([]any, error) {})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
			local := &recordingLocalAdapter{Adapter: socket.NewAdapter(nsp)}
			goRedisClient := rds.NewClient(&rds.Options{Addr: "unused"})
			t.Cleanup(func() { _ = goRedisClient.Close() })
			client := mustRedisClient(t, context.Background(), goRedisClient)
			var emitted error
			if err := client.On("error", func(args ...any) {
				emitted, _ = args[0].(error)
			}); err != nil {
				t.Fatal(err)
			}

			adapter := MakeRedisAdapter().(*redisAdapter)
			adapter.Adapter = local
			adapter.redisClient = client
			adapter.parser = &recordingParser{encodeErr: encodeErr}

			test.run(adapter, &parser.Packet{Type: parser.EVENT})

			if !errors.Is(emitted, encodeErr) {
				t.Fatalf("emitted error = %v, want %v", emitted, encodeErr)
			}
			if local.broadcasts != 0 || local.broadcastsWithAck != 0 {
				t.Fatalf("local broadcasts = %d/%d, want 0/0", local.broadcasts, local.broadcastsWithAck)
			}
			if adapter.ackRequests.Len() != 0 {
				t.Fatal("acknowledgement request was stored after encoding failed")
			}
		})
	}
}

func TestClassicPublishErrorIsEmitted(t *testing.T) {
	publishErr := errors.New("publish failed")
	goRedisClient := rds.NewClient(&rds.Options{Addr: "unused"})
	goRedisClient.AddHook(&processErrorHook{err: publishErr})
	t.Cleanup(func() { _ = goRedisClient.Close() })
	client := mustRedisClient(t, context.Background(), goRedisClient)
	var emitted atomic.Int64
	if err := client.On("error", func(...any) { emitted.Add(1) }); err != nil {
		t.Fatal(err)
	}

	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.Adapter = socket.NewAdapter(nsp)
	adapter.redisClient = client
	adapter.ctx = context.Background()

	adapter.publish("socket.io#/test#", []byte("message"))

	if emitted.Load() != 1 {
		t.Fatalf("error event count = %d, want 1", emitted.Load())
	}
}

func TestClassicBroadcastWithAckUsesDefaultTimeout(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	redisClient := mustRedisClient(t, context.Background(), client)
	if err := redisClient.On("error", func(...any) {}); err != nil {
		t.Fatal(err)
	}
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	redisAdapter := NewRedisAdapter(nsp, redisClient, nil).(*redisAdapter)
	t.Cleanup(func() {
		redisAdapter.Close()
		_ = client.Close()
	})

	redisAdapter.BroadcastWithAck(
		&parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
		nil,
		func(uint64) {},
		func([]any, error) {},
	)
	time.Sleep(20 * time.Millisecond)

	if redisAdapter.ackRequests.Len() != 1 {
		t.Fatal("acknowledgement request expired before the default timeout")
	}
}

func TestClassicAckRequestFollowsAdapterContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.ctx = ctx
	adapter.registerAckRequest("request", &AckRequest{}, time.Minute)

	cancel()
	deadline := time.Now().Add(time.Second)
	for adapter.ackRequests.Len() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if adapter.ackRequests.Len() != 0 {
		t.Fatal("context cancellation did not remove the acknowledgement request")
	}
}

func TestClassicServerCountErrorsAreReturned(t *testing.T) {
	countErr := errors.New("count failed")
	client := rds.NewClient(&rds.Options{Addr: "unused"})
	client.AddHook(&processErrorHook{err: countErr})

	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.Adapter = socket.NewAdapter(nsp)
	adapter.redisClient = mustRedisClient(t, context.Background(), client)
	adapter.ctx = context.Background()
	adapter.requestChannel = "socket.io-request#/#"

	var allRoomsErr error
	adapter.AllRooms()(func(_ *types.Set[socket.Room], err error) {
		allRoomsErr = err
	})
	if !errors.Is(allRoomsErr, countErr) {
		t.Fatalf("AllRooms() error = %v, want %v", allRoomsErr, countErr)
	}

	var fetchSocketsErr error
	adapter.FetchSockets(nil)(func(_ []socket.SocketDetails, err error) {
		fetchSocketsErr = err
	})
	if !errors.Is(fetchSocketsErr, countErr) {
		t.Fatalf("FetchSockets() error = %v, want %v", fetchSocketsErr, countErr)
	}

	ackCalled := false
	err := adapter.ServerSideEmit([]any{"event", func([]any, error) {
		ackCalled = true
	}})
	if !errors.Is(err, countErr) {
		t.Fatalf("ServerSideEmit() error = %v, want %v", err, countErr)
	}
	if ackCalled {
		t.Fatal("acknowledgement was called after server count failed")
	}
	if adapter.requests.Len() != 0 {
		t.Fatal("request was stored after server count failed")
	}
}

func TestClassicResponsePreservesRequiredValues(t *testing.T) {
	t.Run("empty socket IDs", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request", Sockets: []socket.SocketId{}})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","sockets":[]}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("missing socket IDs", func(t *testing.T) {
		adapter := MakeRedisAdapter().(*redisAdapter)
		request := &RedisRequest{
			Type:    redis.SOCKETS,
			NumSub:  1,
			Sockets: types.NewSet[socket.SocketId](),
		}
		resolved := false
		request.Resolve = func(*types.Slice[any]) { resolved = true }
		adapter.requests.Store("request", request)

		adapter.onResponse([]byte(`{"requestId":"request"}`))

		if resolved {
			t.Fatal("missing SOCKETS payload resolved the request")
		}
		if request.MsgCount.Load() != 0 {
			t.Fatal("missing SOCKETS payload was counted")
		}
		if _, ok := adapter.requests.Load("request"); !ok {
			t.Fatal("incomplete SOCKETS request was deleted")
		}
	})

	t.Run("empty socket details", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request", Sockets: []baseadapter.SocketResponse{}})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","sockets":[]}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("zero client count", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request", ClientCount: new(uint64(0))})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","clientCount":0}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("empty rooms", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request", Rooms: []socket.Room{}})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","rooms":[]}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("nil acknowledgement", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request"})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request"}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("Node.js Buffer acknowledgement", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request", Data: []byte{1, 2}})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","data":{"type":"Buffer","data":[1,2]}}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("socket fields", func(t *testing.T) {
		data, err := json.Marshal(&Response{
			RequestId: "request",
			Sockets: []baseadapter.SocketResponse{{
				Id:    "socket",
				Rooms: []socket.Room{},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","sockets":[{"id":"socket","handshake":null,"rooms":[],"data":null}]}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("missing socket details", func(t *testing.T) {
		adapter := MakeRedisAdapter().(*redisAdapter)
		request := &RedisRequest{
			Type:      redis.REMOTE_FETCH,
			NumSub:    1,
			Responses: types.NewSlice[any](),
		}
		resolved := false
		request.Resolve = func(*types.Slice[any]) { resolved = true }
		adapter.requests.Store("request", request)

		adapter.onResponse([]byte(`{"requestId":"request"}`))

		if resolved {
			t.Fatal("missing REMOTE_FETCH payload resolved the request")
		}
		if request.MsgCount.Load() != 0 {
			t.Fatal("missing REMOTE_FETCH payload was counted")
		}
		if _, ok := adapter.requests.Load("request"); !ok {
			t.Fatal("incomplete REMOTE_FETCH request was deleted")
		}
	})
}

func TestRedisAdapterMissingRoomsResponseDoesNotCompleteRequest(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	request := &RedisRequest{
		Type:   redis.ALL_ROOMS,
		NumSub: 1,
		Rooms:  types.NewSet[socket.Room](),
	}
	resolved := false
	request.Resolve = func(*types.Slice[any]) { resolved = true }
	adapter.requests.Store("request", request)

	adapter.onResponse([]byte(`{"requestId":"request"}`))

	if resolved {
		t.Fatal("missing ALL_ROOMS payload resolved the request")
	}
	if request.MsgCount.Load() != 0 {
		t.Fatal("missing ALL_ROOMS payload was counted")
	}
	if _, ok := adapter.requests.Load("request"); !ok {
		t.Fatal("incomplete ALL_ROOMS request was deleted")
	}
}

func TestRedisAdapterAllRoomsMergesBeforeCompleting(t *testing.T) {
	const responseCount = 512

	adapter := MakeRedisAdapter().(*redisAdapter)
	request := &RedisRequest{
		Type:   redis.ALL_ROOMS,
		NumSub: responseCount,
		Rooms:  types.NewSet[socket.Room](),
	}
	resolved := make(chan int, 1)
	request.Resolve = func(*types.Slice[any]) {
		resolved <- request.Rooms.Len()
	}
	adapter.requests.Store("request", request)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range responseCount {
		wg.Go(func() {
			<-start
			adapter.processResponse(request, &Response{
				RequestId: "request",
				Rooms:     []socket.Room{socket.Room(fmt.Sprintf("room-%d", i))},
			})
		})
	}
	close(start)

	select {
	case roomCount := <-resolved:
		if roomCount != responseCount {
			t.Fatalf("resolved with %d rooms, want %d", roomCount, responseCount)
		}
	case <-time.After(time.Second):
		t.Fatal("ALL_ROOMS request did not resolve")
	}
	wg.Wait()
}

func TestRequestOptionFlags(t *testing.T) {
	opts := &socket.BroadcastOptions{
		Rooms:  types.NewSet[socket.Room]("room"),
		Except: types.NewSet[socket.Room]("except"),
		Flags:  &socket.BroadcastFlags{WriteOptions: socket.WriteOptions{Volatile: true}},
	}

	encoded := baseadapter.EncodeOptions(opts)
	for _, test := range []struct {
		name          string
		messageType   baseadapter.MessageType
		preserveFlags bool
	}{
		{name: "selection", messageType: redis.REMOTE_FETCH},
		{name: "broadcast", messageType: redis.BROADCAST, preserveFlags: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload, err := json.Marshal(&Request{Type: test.messageType, Opts: encoded})
			if err != nil {
				t.Fatal(err)
			}
			var wire Request
			if err := json.Unmarshal(payload, &wire); err != nil {
				t.Fatal(err)
			}
			flags := baseadapter.DecodeOptions(wire.Opts).Flags
			if test.preserveFlags != (flags != nil && flags.Volatile) {
				t.Fatalf("flags = %#v, preserve = %t", flags, test.preserveFlags)
			}
		})
	}
}

func TestRedisAdapterOnResponseSeparatesSocketPayloads(t *testing.T) {
	t.Run("socket IDs", func(t *testing.T) {
		adapter := MakeRedisAdapter().(*redisAdapter)
		request := &RedisRequest{
			Type:    redis.SOCKETS,
			NumSub:  3,
			Sockets: types.NewSet[socket.SocketId]("local"),
		}
		request.MsgCount.Store(1)
		resolveCount := 0
		resolved := types.NewSet[socket.SocketId]()
		request.Resolve = func(values *types.Slice[any]) {
			resolveCount++
			for _, value := range values.All() {
				resolved.Add(value.(socket.SocketId))
			}
		}
		adapter.requests.Store("request", request)

		adapter.onResponse([]byte(`{"requestId":"request","sockets":["one","shared"]}`))
		if resolveCount != 0 {
			t.Fatal("SOCKETS request resolved before all responses arrived")
		}
		if _, ok := adapter.requests.Load("request"); !ok {
			t.Fatal("incomplete SOCKETS request was deleted")
		}

		adapter.onResponse([]byte(`{"requestId":"request","sockets":["two","shared"]}`))
		if resolveCount != 1 {
			t.Fatalf("resolve count = %d, want 1", resolveCount)
		}
		for _, socketId := range []socket.SocketId{"local", "one", "two", "shared"} {
			if !resolved.Has(socketId) {
				t.Fatalf("resolved sockets do not contain %q", socketId)
			}
		}
		if resolved.Len() != 4 {
			t.Fatalf("resolved socket count = %d, want 4", resolved.Len())
		}
		if _, ok := adapter.requests.Load("request"); ok {
			t.Fatal("completed SOCKETS request was not deleted")
		}

		adapter.onResponse([]byte(`{"requestId":"request","sockets":["late"]}`))
		if resolveCount != 1 {
			t.Fatalf("late response changed resolve count to %d", resolveCount)
		}
	})

	t.Run("empty socket IDs", func(t *testing.T) {
		adapter := MakeRedisAdapter().(*redisAdapter)
		request := &RedisRequest{
			Type:    redis.SOCKETS,
			NumSub:  1,
			Sockets: types.NewSet[socket.SocketId](),
		}
		resolved := false
		request.Resolve = func(values *types.Slice[any]) {
			resolved = true
			if values == nil || values.Len() != 0 {
				t.Fatalf("resolved sockets = %#v, want empty", values)
			}
		}
		adapter.requests.Store("request", request)

		adapter.onResponse([]byte(`{"requestId":"request","sockets":[]}`))

		if !resolved {
			t.Fatal("empty SOCKETS response did not resolve the request")
		}
		if _, ok := adapter.requests.Load("request"); ok {
			t.Fatal("completed empty SOCKETS request was not deleted")
		}
	})

	t.Run("socket details", func(t *testing.T) {
		adapter := MakeRedisAdapter().(*redisAdapter)
		request := &RedisRequest{
			Type:      redis.REMOTE_FETCH,
			NumSub:    1,
			Responses: types.NewSlice[any](),
		}
		var resolved []any
		request.Resolve = func(values *types.Slice[any]) {
			resolved = values.All()
		}
		adapter.requests.Store("request", request)

		adapter.onResponse([]byte(`{"requestId":"request","sockets":[{"id":"one","handshake":null,"rooms":[],"data":null}]}`))

		if len(resolved) != 1 {
			t.Fatalf("resolved sockets = %#v", resolved)
		}
		client := resolved[0].(socket.SocketDetails)
		if client.Id() != "one" || client.Rooms() == nil || client.Rooms().Len() != 0 {
			t.Fatalf("resolved socket = %#v", client)
		}
		if _, ok := adapter.requests.Load("request"); ok {
			t.Fatal("completed REMOTE_FETCH request was not deleted")
		}
	})

	t.Run("empty socket details", func(t *testing.T) {
		adapter := MakeRedisAdapter().(*redisAdapter)
		request := &RedisRequest{
			Type:      redis.REMOTE_FETCH,
			NumSub:    1,
			Responses: types.NewSlice[any](),
		}
		resolved := false
		request.Resolve = func(values *types.Slice[any]) {
			resolved = values != nil && values.Len() == 0
		}
		adapter.requests.Store("request", request)

		adapter.onResponse([]byte(`{"requestId":"request","sockets":[]}`))

		if !resolved {
			t.Fatal("empty REMOTE_FETCH response did not resolve the request")
		}
		if _, ok := adapter.requests.Load("request"); ok {
			t.Fatal("completed empty REMOTE_FETCH request was not deleted")
		}
	})
}

func TestRedisAdapterOnMessageAcceptsNamespaceChannel(t *testing.T) {
	parser := &recordingParser{packet: &Packet{Uid: baseadapter.ServerId("sender")}}
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.channel = "socket.io#/#"
	adapter.uid = "sender"
	adapter.parser = parser

	adapter.onMessage([]byte("payload"), adapter.channel)

	if !parser.decodeCalled {
		t.Fatal("expected namespace channel message to be decoded")
	}
}

func TestRedisAdapterOnResponseWrapsAckPacket(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	var response []any
	adapter.ackRequests.Store("request", &AckRequest{
		Ack: func(args []any, _ error) {
			response = args
		},
	})
	payload, err := json.Marshal(&Response{
		Type:      redis.BROADCAST_ACK,
		RequestId: "request",
		Packet:    []any{"first", "second"},
	})
	if err != nil {
		t.Fatal(err)
	}

	adapter.onResponse(payload)

	want := []any{[]any{"first", "second"}}
	if !reflect.DeepEqual(response, want) {
		t.Fatalf("acknowledgement = %#v, want %#v", response, want)
	}
}

func TestRedisAdapterMalformedAggregateResponsesAreNotCounted(t *testing.T) {
	for _, test := range []struct {
		name    string
		kind    baseadapter.MessageType
		payload string
	}{
		{name: "socket IDs wrong type", kind: redis.SOCKETS, payload: `{"requestId":"request","sockets":{}}`},
		{name: "socket IDs null", kind: redis.SOCKETS, payload: `{"requestId":"request","sockets":null}`},
		{name: "socket details wrong type", kind: redis.REMOTE_FETCH, payload: `{"requestId":"request","sockets":"invalid"}`},
		{name: "socket details null", kind: redis.REMOTE_FETCH, payload: `{"requestId":"request","sockets":null}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := MakeRedisAdapter().(*redisAdapter)
			request := &RedisRequest{
				Type:      test.kind,
				NumSub:    1,
				Sockets:   types.NewSet[socket.SocketId](),
				Responses: types.NewSlice[any](),
			}
			resolved := false
			request.Resolve = func(*types.Slice[any]) { resolved = true }
			adapter.requests.Store("request", request)

			adapter.onResponse([]byte(test.payload))

			if resolved {
				t.Fatal("malformed response resolved the request")
			}
			if request.MsgCount.Load() != 0 {
				t.Fatalf("message count = %d, want 0", request.MsgCount.Load())
			}
			if _, ok := adapter.requests.Load("request"); !ok {
				t.Fatal("request was deleted after a malformed response")
			}
		})
	}
}

func TestRedisAdapterRejectsMissingBroadcastClientCount(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	called := false
	adapter.ackRequests.Store("request", &AckRequest{
		ClientCountCallback: func(uint64) { called = true },
	})

	adapter.onResponse([]byte(`{"type":8,"requestId":"request"}`))

	if called {
		t.Fatal("missing clientCount payload invoked the callback")
	}
}

func TestClassicBulkSocketOperationsRunLocallyOnce(t *testing.T) {
	server := miniredis.RunT(t)
	goRedisClient := rds.NewClient(&rds.Options{Addr: server.Addr()})
	client := mustRedisClient(t, context.Background(), goRedisClient)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	adapter := NewRedisAdapter(nsp, client, nil).(*redisAdapter)
	local := &recordingLocalAdapter{Adapter: socket.NewAdapter(nsp)}
	adapter.Adapter = local
	t.Cleanup(func() {
		adapter.Close()
		_ = goRedisClient.Close()
	})

	adapter.AddSockets(nil, []socket.Room{"room"})
	adapter.DelSockets(nil, []socket.Room{"room"})
	adapter.DisconnectSockets(nil, false)

	// Allow the Redis self-loop to be delivered. It must be ignored because the
	// local operation is now performed explicitly by the publishing adapter.
	time.Sleep(30 * time.Millisecond)
	if local.adds != 1 || local.dels != 1 || local.disconnects != 1 {
		t.Fatalf("local operations = add:%d del:%d disconnect:%d, want 1 each", local.adds, local.dels, local.disconnects)
	}
}

func TestClassicRegisterRequestTimeoutDeletesBeforeCallback(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	request := &RedisRequest{}
	deletedBeforeCallback := make(chan bool, 1)
	var callbackCount atomic.Int64

	adapter.registerRequest("request", request, 10*time.Millisecond, func() {
		callbackCount.Add(1)
		_, exists := adapter.requests.Load("request")
		deletedBeforeCallback <- !exists
	})

	select {
	case deleted := <-deletedBeforeCallback:
		if !deleted {
			t.Fatal("timeout callback ran before the pending request was deleted")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout callback was not called")
	}

	time.Sleep(20 * time.Millisecond)
	if callbackCount.Load() != 1 {
		t.Fatalf("timeout callback count = %d, want 1", callbackCount.Load())
	}
	if adapter.requests.Len() != 0 {
		t.Fatal("timed-out request remained pending")
	}
}

func TestClassicFinishRequestStopsTimeout(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	resolved := make(chan struct{}, 1)
	var resolveCount atomic.Int64
	var timeoutCalled atomic.Bool
	request := &RedisRequest{
		Type: redis.REMOTE_JOIN,
		Resolve: func(*types.Slice[any]) {
			resolveCount.Add(1)
			resolved <- struct{}{}
		},
	}

	adapter.registerRequest("request", request, 100*time.Millisecond, func() {
		timeoutCalled.Store(true)
	})
	adapter.processResponse(request, &Response{RequestId: "request"})

	select {
	case <-resolved:
	case <-time.After(time.Second):
		t.Fatal("completed request was not resolved")
	}

	time.Sleep(150 * time.Millisecond)
	if resolveCount.Load() != 1 {
		t.Fatalf("resolve callback count = %d, want 1", resolveCount.Load())
	}
	if timeoutCalled.Load() {
		t.Fatal("completed request timeout callback was called")
	}
	if adapter.requests.Len() != 0 || request.Timeout.Load() != nil {
		t.Fatal("completed request retained pending state")
	}
}

func TestClassicFinishRequestResolvesOnce(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	var resolveCount atomic.Int64
	var timeoutCalled atomic.Bool
	request := &RedisRequest{
		Type: redis.REMOTE_JOIN,
		Resolve: func(*types.Slice[any]) {
			resolveCount.Add(1)
		},
	}
	adapter.registerRequest("request", request, time.Second, func() {
		timeoutCalled.Store(true)
	})

	const responseCount = 64
	var wg sync.WaitGroup
	for range responseCount {
		wg.Go(func() {
			adapter.processResponse(request, &Response{RequestId: "request"})
		})
	}
	wg.Wait()

	if resolveCount.Load() != 1 {
		t.Fatalf("resolve callback count = %d, want 1", resolveCount.Load())
	}
	if timeoutCalled.Load() {
		t.Fatal("completed request timeout callback was called")
	}
	if adapter.requests.Len() != 0 || request.Timeout.Load() != nil {
		t.Fatal("completed request retained pending state")
	}
}

func TestClassicCloseDoesNotWaitForInFlightMessage(t *testing.T) {
	goRedisClient := rds.NewClient(&rds.Options{Addr: "unused"})
	t.Cleanup(func() { _ = goRedisClient.Close() })
	client := mustRedisClient(t, context.Background(), goRedisClient)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	local := &recordingLocalAdapter{
		Adapter:          socket.NewAdapter(nsp),
		broadcastStarted: make(chan struct{}),
		broadcastRelease: make(chan struct{}),
	}
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.Adapter = local
	adapter.redisClient = client
	adapter.channel = "socket.io#/test#"
	adapter.uid = "self"
	adapter.parser = &recordingParser{packet: &Packet{
		Uid:    "sender",
		Packet: &parser.Packet{Type: parser.EVENT, Nsp: "/test"},
	}}
	messageDone := make(chan struct{})
	go func() {
		adapter.onMessage([]byte("payload"), adapter.channel)
		close(messageDone)
	}()
	<-local.broadcastStarted
	closed := make(chan struct{})
	go func() {
		adapter.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close waited for an in-flight local adapter call")
	}
	close(local.broadcastRelease)
	// The already-entered local call is allowed to finish independently.
	select {
	case <-messageDone:
	case <-time.After(time.Second):
		t.Fatal("in-flight broadcast did not finish after it was released")
	}
	if local.broadcasts != 1 {
		t.Fatalf("broadcast count = %d, want 1", local.broadcasts)
	}
}

func TestClassicCloseFromSynchronousCallbackDoesNotDeadlock(t *testing.T) {
	goRedisClient := rds.NewClient(&rds.Options{Addr: "unused"})
	t.Cleanup(func() { _ = goRedisClient.Close() })
	client := mustRedisClient(t, context.Background(), goRedisClient)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	local := &recordingLocalAdapter{Adapter: socket.NewAdapter(nsp), invokeClientCount: true}
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.Adapter = local
	adapter.redisClient = client
	adapter.ctx, adapter.cancel = context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		adapter.BroadcastWithAck(
			&parser.Packet{Type: parser.EVENT},
			&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Local: true}},
			func(uint64) { adapter.Close() },
			func([]any, error) {},
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close deadlocked inside a synchronous adapter callback")
	}
}
