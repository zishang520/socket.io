package emitter

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

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

	t.Run("Parser", func(t *testing.T) {
		if opts.GetRawParser() != nil {
			t.Fatal(`DefaultEmitterOptions.GetRawParser() value must be nil`)
		}
	})
}

func TestClassicEmitterNodeWire(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	redisClient := redis.NewRedisClient(context.Background(), client)
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
	if err := emit.opts.Parser().Decode([]byte(message.Payload), &packet); err != nil {
		t.Fatal(err)
	}
	if packet.Uid != "emitter" || packet.Packet.Nsp != "/chat" {
		t.Fatalf("packet = %#v", packet)
	}
	if !reflect.DeepEqual(packet.Opts.Rooms, []socket.Room{room}) || packet.Opts.Except == nil || packet.Opts.Flags == nil {
		t.Fatalf("options = %#v", packet.Opts)
	}
}

func TestClassicEmitterPreservesExplicitEmptyNamespace(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	emit := NewEmitter(redis.NewRedisClient(context.Background(), client), nil, "")
	if emit.nsp != "" || emit.broadcastOptions.BroadcastChannel != "socket.io##" {
		t.Fatalf("namespace = %q, channel = %q", emit.nsp, emit.broadcastOptions.BroadcastChannel)
	}
	if got := emit.Of("").nsp; got != "/" {
		t.Fatalf("Of empty namespace = %q, want /", got)
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

func TestRedisStreamsOperatorKeepsConcreteType(t *testing.T) {
	options := DefaultRedisStreamsEmitterOptions()
	options.SetStreamName("events")
	options.SetMaxLen(100)
	operator := NewRedisStreamsBroadcastOperator(nil, "", options, nil, nil, nil)
	result, ok := operator.To("room").Except("excluded").Volatile().Compress(false).(*RedisStreamsBroadcastOperator)
	if !ok {
		t.Fatal("chaining changed the Redis Streams operator type")
	}
	if result.opts.StreamName() != options.StreamName() || result.opts.MaxLen() != options.MaxLen() {
		t.Fatal("chaining changed the Redis Streams options")
	}
	if !result.rooms.Has("room") || !result.exceptRooms.Has("excluded") {
		t.Fatal("chaining lost room selection")
	}
	if !result.flags.Volatile || result.flags.Compress == nil || *result.flags.Compress {
		t.Fatal("chaining lost broadcast flags")
	}
}

func TestRedisStreamsEmitterNodeWire(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	options := DefaultRedisStreamsEmitterOptions()
	options.SetStreamName("events")
	options.SetMaxLen(100)
	emit := NewRedisStreamsEmitter(
		redis.NewRedisClient(context.Background(), client),
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
		redis.NewRedisClient(context.Background(), client),
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
	redisClient := redis.NewRedisClient(context.TODO(), rds.NewClient(&rds.Options{
		Addr: server.Addr(),
	}))

	emit := NewEmitter(redisClient, nil)

	t.Run("Of", func(t *testing.T) {
		emit.Of("test")
	})

	t.Run("Emit", func(t *testing.T) {
		if err := emit.Emit("test", "data", "data"); err != nil {
			t.Fatal(`emit.Emit() value must be nil`)
		}
	})

	t.Run("To", func(t *testing.T) {
		emit.To("test")
	})

	t.Run("In", func(t *testing.T) {
		emit.In("test")
	})

	t.Run("Except", func(t *testing.T) {
		emit.Except("test")
	})

	t.Run("Volatile", func(t *testing.T) {
		emit.Volatile()
	})

	t.Run("Compress", func(t *testing.T) {
		emit.Compress(false)
	})

	t.Run("SocketsJoin", func(t *testing.T) {
		_ = emit.SocketsJoin("room")
	})

	t.Run("SocketsLeave", func(t *testing.T) {
		_ = emit.SocketsLeave("room")
	})

	t.Run("DisconnectSockets", func(t *testing.T) {
		_ = emit.DisconnectSockets(false)
	})

	t.Run("ServerSideEmit", func(t *testing.T) {
		err := emit.ServerSideEmit("false", "aaa", func([]any, error) {})
		if !errors.Is(err, errAcknowledgementsNotSupported) {
			t.Fatalf("ServerSideEmit error = %v, want %v", err, errAcknowledgementsNotSupported)
		}
		err = emit.ServerSideEmit("false", "aaa")
		if err != nil {
			t.Fatalf(`ServerSideEmit error not as expected: %v, want match for %v`, nil, err)
		}
	})
}

func TestBroadcastOperator(t *testing.T) {
	server := miniredis.RunT(t)
	redisClient := redis.NewRedisClient(context.TODO(), rds.NewClient(&rds.Options{
		Addr: server.Addr(),
	}))

	b := NewBroadcastOperator(redisClient, &BroadcastOptions{
		Nsp:              "",
		BroadcastChannel: "",
		RequestChannel:   "",
		Parser:           utils.MsgPack(),
	}, nil, nil, nil)

	t.Run("Emit", func(t *testing.T) {
		if err := b.Emit("test", "data", "data"); err != nil {
			t.Fatalf(`emit.Emit() value must be nil: %v`, err)
		}
	})

	t.Run("To", func(t *testing.T) {
		b.To("test")
	})

	t.Run("In", func(t *testing.T) {
		b.In("test")
	})

	t.Run("Except", func(t *testing.T) {
		b.Except("test")
	})

	t.Run("Volatile", func(t *testing.T) {
		b.Volatile()
	})

	t.Run("Compress", func(t *testing.T) {
		b.Compress(false)
	})

	t.Run("SocketsJoin", func(t *testing.T) {
		_ = b.SocketsJoin("room")
	})

	t.Run("SocketsLeave", func(t *testing.T) {
		_ = b.SocketsLeave("room")
	})

	t.Run("DisconnectSockets", func(t *testing.T) {
		_ = b.DisconnectSockets(false)
	})
}
