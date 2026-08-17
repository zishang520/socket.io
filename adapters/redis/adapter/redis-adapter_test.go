package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
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
}

type processErrorHook struct {
	err error
}

func (h *processErrorHook) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *processErrorHook) ProcessHook(rds.ProcessHook) rds.ProcessHook {
	return func(context.Context, rds.Cmder) error { return h.err }
}

func (h *processErrorHook) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func (a *recordingLocalAdapter) Broadcast(*parser.Packet, *socket.BroadcastOptions) {
	a.broadcasts++
}

func (a *recordingLocalAdapter) BroadcastWithAck(*parser.Packet, *socket.BroadcastOptions, func(uint64), socket.Ack) {
	a.broadcastsWithAck++
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
		request.Resolve = func(values *types.Slice[any]) {
			resolved = values != nil && values.Len() == 0
		}
		adapter.requests.Store("request", request)

		adapter.onResponse("", []byte(`{"requestId":"request"}`))

		if !resolved {
			t.Fatal("missing SOCKETS payload did not resolve the request")
		}
		if _, ok := adapter.requests.Load("request"); ok {
			t.Fatal("completed SOCKETS request was not deleted")
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
		request.Resolve = func(values *types.Slice[any]) {
			resolved = values != nil && values.Len() == 0
		}
		adapter.requests.Store("request", request)

		adapter.onResponse("", []byte(`{"requestId":"request"}`))

		if !resolved {
			t.Fatal("missing REMOTE_FETCH payload did not resolve the request")
		}
		if _, ok := adapter.requests.Load("request"); ok {
			t.Fatal("completed REMOTE_FETCH request was not deleted")
		}
	})
}

func TestRedisAdapterMissingRoomsResponseCompletesRequest(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	request := &RedisRequest{
		Type:   redis.ALL_ROOMS,
		NumSub: 1,
		Rooms:  types.NewSet[socket.Room](),
	}
	resolved := false
	request.Resolve = func(*types.Slice[any]) { resolved = true }
	adapter.requests.Store("request", request)

	adapter.onResponse("", []byte(`{"requestId":"request"}`))

	if !resolved {
		t.Fatal("missing ALL_ROOMS payload did not resolve the request")
	}
	if _, ok := adapter.requests.Load("request"); ok {
		t.Fatal("completed ALL_ROOMS request was not deleted")
	}
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

		adapter.onResponse("", []byte(`{"requestId":"request","sockets":["one","shared"]}`))
		if resolveCount != 0 {
			t.Fatal("SOCKETS request resolved before all responses arrived")
		}
		if _, ok := adapter.requests.Load("request"); !ok {
			t.Fatal("incomplete SOCKETS request was deleted")
		}

		adapter.onResponse("", []byte(`{"requestId":"request","sockets":["two","shared"]}`))
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

		adapter.onResponse("", []byte(`{"requestId":"request","sockets":["late"]}`))
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

		adapter.onResponse("", []byte(`{"requestId":"request","sockets":[]}`))

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

		adapter.onResponse("", []byte(`{"requestId":"request","sockets":[{"id":"one","handshake":null,"rooms":[],"data":null}]}`))

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

		adapter.onResponse("", []byte(`{"requestId":"request","sockets":[]}`))

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

	adapter.onMessage(adapter.channel+"*", adapter.channel, []byte("payload"))

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

	adapter.onResponse("", payload)

	want := []any{[]any{"first", "second"}}
	if !reflect.DeepEqual(response, want) {
		t.Fatalf("acknowledgement = %#v, want %#v", response, want)
	}
}
