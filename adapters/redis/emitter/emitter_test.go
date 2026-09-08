package emitter

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type encodeOnly struct {
	called bool
}

type recordingShardedClient struct {
	rds.UniversalClient
	channel string
	payload any
	calls   int
}

func (c *recordingShardedClient) SPublish(ctx context.Context, channel string, payload any) *rds.IntCmd {
	c.channel = channel
	c.payload = payload
	c.calls++
	cmd := rds.NewIntCmd(ctx)
	cmd.SetVal(1)
	return cmd
}

func (e *encodeOnly) Encode(value any) ([]byte, error) {
	e.called = true
	return utils.MsgPack().Encode(value)
}

var _ EmitterOptionsInterface = (*EmitterOptions)(nil)

func TestEmitterOptions(t *testing.T) {
	opts := DefaultEmitterOptions()
	opts.Assign(nil)

	t.Run("Key", func(t *testing.T) {
		if opts.GetRawKey() != nil {
			t.Fatal(`DefaultEmitterOptions.GetRawKey() value must be nil`)
		}
		if opts.Key() != "" {
			t.Fatal(`DefaultEmitterOptions.Key() value must be ""`)
		}
		opts.SetKey("test")
		if opts.Key() != "test" {
			t.Fatal(`DefaultEmitterOptions.Key() value must be "test"`)
		}
	})

	t.Run("Encoder", func(t *testing.T) {
		if opts.GetRawEncoder() != nil {
			t.Fatal(`DefaultEmitterOptions.GetRawEncoder() value must be nil`)
		}
	})
}

func mustRedisClient(t *testing.T, client rds.UniversalClient) *redis.RedisClient {
	t.Helper()

	redisClient, err := redis.NewRedisClient(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	return redisClient
}

func TestClassicEmitterNodeWire(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	redisClient := mustRedisClient(t, client)
	emit := NewEmitter(redisClient, nil, "/chat")

	room := socket.Room("abcdefghijklmnopqrst")
	channel := "socket.io#/chat#" + string(room) + "#"
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pubsub := client.Subscribe(ctx, channel)
	defer func() { _ = pubsub.Close() }()
	if _, err := pubsub.Receive(ctx); err != nil {
		t.Fatal(err)
	}

	if err := emit.To(room).Emit("event", "value"); err != nil {
		t.Fatal(err)
	}
	message, err := pubsub.ReceiveMessage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var packet redis.RedisPacket
	if err := utils.MsgPack().Decode([]byte(message.Payload), &packet); err != nil {
		t.Fatal(err)
	}
	if packet.Uid != "emitter" || packet.Packet.Nsp != "/chat" {
		t.Fatalf("packet = %#v", packet)
	}
	if !reflect.DeepEqual(packet.Opts.Rooms, []socket.Room{room}) || packet.Opts.Except == nil || packet.Opts.Flags == nil {
		t.Fatalf("options = %#v", packet.Opts)
	}
}

func TestClassicEmitterAcceptsEncodeOnlyEncoder(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	encoder := new(encodeOnly)
	options := DefaultEmitterOptions()
	options.SetEncoder(encoder)

	if err := NewEmitter(mustRedisClient(t, client), options).Emit("event", "value"); err != nil {
		t.Fatal(err)
	}
	if !encoder.called {
		t.Fatal("encode-only encoder was not called")
	}
}

func TestClassicEmitterTypedNilEncoderUsesDefault(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	options := DefaultEmitterOptions()
	options.SetEncoder((*encodeOnly)(nil))

	emit := NewEmitter(mustRedisClient(t, client), options)
	if utils.IsNil(emit.broadcastOptions.Encoder) {
		t.Fatal("typed nil encoder was not replaced with the default")
	}
	if err := emit.Emit("event", "value"); err != nil {
		t.Fatal(err)
	}
}

func TestClassicEmitterPreservesConstructorRoutingValues(t *testing.T) {
	emit := NewEmitter(nil, nil, "")
	if emit.broadcastOptions.Nsp != "" || emit.broadcastOptions.BroadcastChannel != "socket.io##" {
		t.Fatalf("namespace = %q, channel = %q", emit.broadcastOptions.Nsp, emit.broadcastOptions.BroadcastChannel)
	}
	if got := emit.Of("").broadcastOptions.Nsp; got != "/" {
		t.Fatalf("Of empty namespace = %q, want /", got)
	}
	if got := NewEmitter(nil, nil, "chat").broadcastOptions.Nsp; got != "chat" {
		t.Fatalf("direct namespace = %q, want raw chat", got)
	}
	if got := emit.Of("chat").broadcastOptions.Nsp; got != "/chat" {
		t.Fatalf("Of namespace = %q, want /chat", got)
	}

	options := DefaultEmitterOptions()
	options.SetKey("")
	emptyKeyEmitter := NewEmitter(nil, options)
	if got := emptyKeyEmitter.broadcastOptions.BroadcastChannel; got != "#/#" {
		t.Fatalf("explicit empty key channel = %q, want #/#", got)
	}
}

func TestShardedOperatorKeepsConcreteType(t *testing.T) {
	operator := MakeShardedBroadcastOperator()
	if _, ok := operator.To("room").(*ShardedBroadcastOperator); !ok {
		t.Fatal("To changed the sharded operator type")
	}
	if _, ok := operator.Except("room").(*ShardedBroadcastOperator); !ok {
		t.Fatal("Except changed the sharded operator type")
	}
	emit := MakeEmitter()
	emit.opts.SetSharded(true)
	if _, ok := emit.To("room").(*ShardedBroadcastOperator); !ok {
		t.Fatal("Emitter.To changed the sharded operator type")
	}
}

// Regression test for https://github.com/zishang520/socket.io/issues/136.
func TestShardedEmitterUsesSPublishWithClusterMessage(t *testing.T) {
	client := &recordingShardedClient{UniversalClient: rds.NewClient(new(rds.Options))}
	t.Cleanup(func() { _ = client.Close() })
	options := DefaultEmitterOptions()
	options.SetSharded(true)

	if err := NewEmitter(mustRedisClient(t, client), options).Emit("event", "value"); err != nil {
		t.Fatal(err)
	}
	if client.calls != 1 || client.channel != "socket.io#/#" {
		t.Fatalf("SPUBLISH calls/channel = %d/%q, want 1/socket.io#/#", client.calls, client.channel)
	}
	payload, ok := client.payload.([]byte)
	if !ok {
		t.Fatalf("SPUBLISH payload type = %T, want []byte", client.payload)
	}
	message, err := adapter.DecodeClusterMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	data, ok := message.Data.(*adapter.BroadcastMessage)
	if message.Uid != "emitter" || message.Nsp != "/" || message.Type != adapter.BROADCAST || !ok ||
		data.Packet == nil || data.Packet.Type != parser.EVENT ||
		!reflect.DeepEqual(data.Packet.Data, []any{"event", "value"}) {
		t.Fatalf("cluster message = %#v, data = %#v", message, message.Data)
	}
}

func TestShardedEmitterUsesCommonChannelForDefaultSocketRoom(t *testing.T) {
	client := &recordingShardedClient{UniversalClient: rds.NewClient(new(rds.Options))}
	t.Cleanup(func() { _ = client.Close() })
	options := DefaultEmitterOptions()
	options.SetSharded(true)
	room := socket.Room(utils.Base64Id().GenerateId())

	if err := NewEmitter(mustRedisClient(t, client), options).To(room).Emit("event"); err != nil {
		t.Fatal(err)
	}
	want := "socket.io#/#"
	if client.channel != want {
		t.Fatalf("SPUBLISH channel = %q, want %q", client.channel, want)
	}
}

func TestRedisStreamsOperatorKeepsConcreteType(t *testing.T) {
	options := DefaultRedisStreamsEmitterOptions()
	options.SetStreamName("events")
	options.SetStreamCount(5)
	options.SetMaxLen(100)
	operator := NewRedisStreamsBroadcastOperator(nil, "", options, nil, nil, nil)
	result, ok := operator.To("room").Except("excluded").Volatile().Compress(false).(*RedisStreamsBroadcastOperator)
	if !ok {
		t.Fatal("chaining changed the Redis Streams operator type")
	}
	if result.opts.StreamName() != options.StreamName() || result.opts.StreamCount() != options.StreamCount() ||
		result.opts.MaxLen() != options.MaxLen() {
		t.Fatal("chaining changed the Redis Streams options")
	}
	if !result.rooms.Has("room") || !result.exceptRooms.Has("excluded") {
		t.Fatal("chaining lost room selection")
	}
	if !result.flags.Volatile || result.flags.Compress == nil || *result.flags.Compress {
		t.Fatal("chaining lost broadcast flags")
	}
}

func TestRedisStreamsEmitterRoutesNamespaceToConfiguredStream(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	options := DefaultRedisStreamsEmitterOptions()
	options.SetStreamName("events")
	options.SetStreamCount(5)
	emit := NewRedisStreamsEmitter(mustRedisClient(t, client), options).Of("/namespace-0")

	if err := emit.Emit("event"); err != nil {
		t.Fatal(err)
	}

	entries, err := client.XRange(context.Background(), "events--3", "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries in routed stream = %d, want 1", len(entries))
	}
	if exists := server.Exists("events"); exists {
		t.Fatal("message was also written to the base stream")
	}
}

func TestRedisStreamsEmitterNodeWire(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	options := DefaultRedisStreamsEmitterOptions()
	options.SetStreamName("events")
	options.SetMaxLen(100)
	emit := NewRedisStreamsEmitter(
		mustRedisClient(t, client),
		options,
	).Of("chat")

	if err := emit.Emit("event", []byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	entries, err := client.XRange(context.Background(), "events", "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	raw := redis.RawClusterMessage(entries[0].Values)
	if raw.Uid() != "emitter" || raw.Nsp() != "chat" {
		t.Fatalf("raw message = %#v", raw)
	}
	message, err := redis.DecodeStreamMessage(raw)
	if err != nil {
		t.Fatal(err)
	}
	data := message.Data.(*BroadcastMessage)
	if data.Packet.Nsp != "chat" || !reflect.DeepEqual(data.Packet.Data.([]any)[1], []byte{1, 2}) {
		t.Fatalf("packet = %#v", data.Packet)
	}
}

func TestRedisStreamsEmitterPreservesEmptyNamespace(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	options := DefaultRedisStreamsEmitterOptions()
	options.SetStreamName("events")
	emit := NewRedisStreamsEmitter(
		mustRedisClient(t, client),
		options,
	).Of("")

	if err := emit.Emit("event"); err != nil {
		t.Fatal(err)
	}
	entries, err := client.XRange(context.Background(), "events", "-", "+").Result()
	if err != nil {
		t.Fatal(err)
	}
	if got := redis.RawClusterMessage(entries[0].Values).Nsp(); got != "" {
		t.Fatalf("namespace = %q, want empty", got)
	}
}

func TestEmitter(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, client)
	operator := NewBroadcastOperator(redisClient, &BroadcastOptions{
		Nsp:              "/chat",
		BroadcastChannel: "custom#/chat#",
		RequestChannel:   "custom-request#/chat#",
		Encoder:          utils.MsgPack(),
	}, nil, nil, nil).To("selected").Except("excluded")

	for _, tt := range []struct {
		name     string
		operator BroadcastOperatorInterface
		channel  string
		opts     *adapter.PacketOptions
	}{
		{
			name:     "emitter",
			operator: NewEmitter(redisClient, nil),
			channel:  "socket.io-request#/#",
			opts:     &adapter.PacketOptions{Rooms: []socket.Room{}, Except: []socket.Room{}},
		},
		{
			name:     "operator",
			operator: operator,
			channel:  "custom-request#/chat#",
			opts:     &adapter.PacketOptions{Rooms: []socket.Room{"selected"}, Except: []socket.Room{"excluded"}},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			pubsub := client.Subscribe(ctx, tt.channel)
			defer func() { _ = pubsub.Close() }()
			if _, err := pubsub.Receive(ctx); err != nil {
				t.Fatal(err)
			}

			t.Run("Emit", func(t *testing.T) {
				if err := tt.operator.Emit("test", "data", "data"); err != nil {
					t.Fatal(err)
				}
			})

			for _, operation := range []struct {
				name    string
				publish func() error
				want    redis.RedisRequest
			}{
				{
					name:    "SocketsJoin",
					publish: func() error { return tt.operator.SocketsJoin("room") },
					want:    redis.RedisRequest{Type: redis.REMOTE_JOIN, Opts: tt.opts, Rooms: []socket.Room{"room"}},
				},
				{
					name:    "SocketsLeave",
					publish: func() error { return tt.operator.SocketsLeave("room") },
					want:    redis.RedisRequest{Type: redis.REMOTE_LEAVE, Opts: tt.opts, Rooms: []socket.Room{"room"}},
				},
				{
					name:    "DisconnectSockets",
					publish: func() error { return tt.operator.DisconnectSockets(false) },
					want:    redis.RedisRequest{Type: redis.REMOTE_DISCONNECT, Opts: tt.opts, Close: new(false)},
				},
			} {
				t.Run(operation.name, func(t *testing.T) {
					if err := operation.publish(); err != nil {
						t.Fatal(err)
					}
					message, err := pubsub.ReceiveMessage(ctx)
					if err != nil {
						t.Fatal(err)
					}
					var request redis.RedisRequest
					if err = json.Unmarshal([]byte(message.Payload), &request); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(request, operation.want) {
						t.Fatalf("request = %s, want %#v", message.Payload, operation.want)
					}
				})
			}

			t.Run("ServerSideEmit", func(t *testing.T) {
				err := tt.operator.ServerSideEmit("false", "aaa", func([]any, error) {})
				if !errors.Is(err, errAcknowledgementsNotSupported) {
					t.Fatalf("ServerSideEmit error = %v, want %v", err, errAcknowledgementsNotSupported)
				}
				if err = tt.operator.ServerSideEmit("false", "aaa"); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}
