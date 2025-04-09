package emitter

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/servers/engine/v3/utils"
	"github.com/zishang520/socket.io/adapters/redis/v3/types"
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

func TestEmitter(t *testing.T) {
	redisClient := types.NewRedisClient(context.TODO(), redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Username: "",
		Password: "root",
		DB:       0,
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
		emit.SocketsJoin("room")
	})

	t.Run("SocketsLeave", func(t *testing.T) {
		emit.SocketsLeave("room")
	})

	t.Run("DisconnectSockets", func(t *testing.T) {
		emit.DisconnectSockets(false)
	})

	t.Run("ServerSideEmit", func(t *testing.T) {
		err := emit.ServerSideEmit("false", "aaa", func(...any) {})
		if err == nil {
			t.Fatal("ServerSideEmit error must not be nil")
		}
		err = emit.ServerSideEmit("false", "aaa")
		if err != nil {
			t.Fatalf(`ServerSideEmit error not as expected: %v, want match for %v`, nil, err)
		}
	})
}

func TestBroadcastOperator(t *testing.T) {
	redisClient := types.NewRedisClient(context.TODO(), redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Username: "",
		Password: "root",
		DB:       0,
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
		b.SocketsJoin("room")
	})

	t.Run("SocketsLeave", func(t *testing.T) {
		b.SocketsLeave("room")
	})

	t.Run("DisconnectSockets", func(t *testing.T) {
		b.DisconnectSockets(false)
	})
}
