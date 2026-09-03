package emitter

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/redis/v3"
)

type customRedisStreamsEmitterOptions struct {
	*RedisStreamsEmitterOptions
}

var (
	_ RedisStreamsEmitterOptionsInterface = (*RedisStreamsEmitterOptions)(nil)
	_ RedisStreamsEmitterOptionsInterface = (*customRedisStreamsEmitterOptions)(nil)
)

func newStreamsEmitterTestClient(t *testing.T) (*redis.RedisClient, *rds.Client, <-chan error) {
	t.Helper()
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, client)
	redisErrors := make(chan error, 2)
	_ = redisClient.On("error", func(args ...any) {
		if len(args) > 0 {
			if err, ok := args[0].(error); ok {
				redisErrors <- err
			}
		}
	})
	return redisClient, client, redisErrors
}

func TestRedisStreamsEmitterOptions(t *testing.T) {
	opts := DefaultRedisStreamsEmitterOptions()
	if opts.GetRawStreamName() != nil || opts.GetRawStreamCount() != nil || opts.GetRawMaxLen() != nil {
		t.Fatal("default options contain explicitly set values")
	}
	var typedNil *RedisStreamsEmitterOptions
	if result := opts.Assign(typedNil); result != opts {
		t.Fatal("typed nil assignment did not preserve the target")
	}

	values := DefaultRedisStreamsEmitterOptions()
	values.SetStreamName("")
	values.SetStreamCount(0)
	values.SetMaxLen(0)
	opts.Assign(values)
	if opts.GetRawStreamName() == nil || opts.StreamName() != "" {
		t.Fatal("explicit empty stream name was not preserved")
	}
	if raw := opts.GetRawStreamCount(); raw == nil || raw.Get() != 0 {
		t.Fatal("explicit raw zero stream count was not preserved")
	}
	if opts.StreamCount() != 0 {
		t.Fatal("explicit zero stream count was not preserved by the getter")
	}
	values.SetStreamCount(-2)
	opts.Assign(values)
	if raw := opts.GetRawStreamCount(); raw == nil || raw.Get() != -2 {
		t.Fatal("explicit raw negative stream count was not preserved")
	}
	if opts.StreamCount() != -2 {
		t.Fatal("explicit negative stream count was not preserved by the getter")
	}
	if opts.GetRawMaxLen() == nil || opts.MaxLen() != 0 {
		t.Fatal("explicit zero max length was not preserved")
	}
}

func TestRedisStreamsEmitterOptionsDefaults(t *testing.T) {
	emitter := NewRedisStreamsEmitter(nil, nil)
	if emitter.opts.StreamName() != DefaultStreamName || emitter.opts.StreamCount() != DefaultStreamCount ||
		emitter.opts.MaxLen() != DefaultStreamMaxLen {
		t.Fatalf("options = %q, %d, %d", emitter.opts.StreamName(), emitter.opts.StreamCount(), emitter.opts.MaxLen())
	}

	opts := DefaultRedisStreamsEmitterOptions()
	opts.SetStreamName("")
	opts.SetStreamCount(-2)
	opts.SetMaxLen(0)
	emitter = NewRedisStreamsEmitter(nil, opts)
	if emitter.opts.StreamName() != "" || emitter.opts.StreamCount() != -2 || emitter.opts.MaxLen() != 0 {
		t.Fatalf("explicit options = %q, %d, %d", emitter.opts.StreamName(), emitter.opts.StreamCount(), emitter.opts.MaxLen())
	}
	scoped := emitter.Of("/chat")
	if scoped.opts.StreamName() != "" || scoped.opts.StreamCount() != -2 || scoped.opts.MaxLen() != 0 {
		t.Fatalf("scoped options = %q, %d, %d", scoped.opts.StreamName(), scoped.opts.StreamCount(), scoped.opts.MaxLen())
	}
	if raw := emitter.opts.GetRawStreamCount(); raw == nil || raw.Get() != -2 {
		t.Fatalf("emitter raw stream count = %v, want -2", raw)
	}
	if raw := scoped.opts.GetRawStreamCount(); raw == nil || raw.Get() != -2 {
		t.Fatalf("scoped raw stream count = %v, want -2", raw)
	}
}

func TestRedisStreamsEmitterRoutesStreamCount(t *testing.T) {
	tests := []struct {
		name        string
		streamCount int
		custom      bool
		wantStream  string
	}{
		{
			name:        "zero",
			streamCount: 0,
			wantStream:  "events",
		},
		{
			name:        "negative custom options",
			streamCount: -2,
			custom:      true,
			wantStream:  "events",
		},
		{
			name:        "normal",
			streamCount: 5,
			wantStream:  "events--3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redisClient, client, redisErrors := newStreamsEmitterTestClient(t)

			opts := DefaultRedisStreamsEmitterOptions()
			opts.SetStreamName("events")
			if tt.custom {
				custom := &customRedisStreamsEmitterOptions{DefaultRedisStreamsEmitterOptions()}
				custom.SetStreamCount(tt.streamCount)
				opts.Assign(custom)
			} else {
				opts.SetStreamCount(tt.streamCount)
			}

			emitter := NewRedisStreamsEmitter(redisClient, opts)
			scoped := emitter.Of("/namespace-0")
			if emitter.opts.StreamCount() != tt.streamCount || scoped.opts.StreamCount() != tt.streamCount {
				t.Fatalf("stream counts = %d, %d, want raw value %d", emitter.opts.StreamCount(), scoped.opts.StreamCount(), tt.streamCount)
			}
			if got := redis.StreamNameForNamespace(scoped.opts.StreamName(), scoped.nsp, scoped.opts.StreamCount()); got != tt.wantStream {
				t.Fatalf("routed stream = %q, want %q", got, tt.wantStream)
			}
			if err := scoped.Emit("event"); err != nil {
				t.Fatal(err)
			}
			entries, err := client.XRange(context.Background(), tt.wantStream, "-", "+").Result()
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("entries in routed stream = %d, want 1", len(entries))
			}

			select {
			case err := <-redisErrors:
				t.Fatalf("stream count emitted an error: %v", err)
			default:
			}
		})
	}
}

func TestRedisStreamsBroadcastOperatorRoutesNonPositiveStreamCountToBaseStream(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  int
	}{
		{name: "zero", raw: 0},
		{name: "negative", raw: -2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			redisClient, client, redisErrors := newStreamsEmitterTestClient(t)
			opts := DefaultRedisStreamsEmitterOptions()
			opts.SetStreamName("events")
			opts.SetStreamCount(tt.raw)

			operator := NewRedisStreamsBroadcastOperator(redisClient, "/chat", opts, nil, nil, nil)
			derived := operator.To("room").(*RedisStreamsBroadcastOperator)
			if operator.opts.StreamCount() != tt.raw || derived.opts.StreamCount() != tt.raw {
				t.Fatalf("stream counts = %d, %d, want raw value %d", operator.opts.StreamCount(), derived.opts.StreamCount(), tt.raw)
			}
			if raw := derived.opts.GetRawStreamCount(); raw == nil || raw.Get() != tt.raw {
				t.Fatalf("raw stream count = %v, want %d", raw, tt.raw)
			}
			if err := derived.Emit("event"); err != nil {
				t.Fatal(err)
			}
			entries, err := client.XRange(context.Background(), "events", "-", "+").Result()
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 {
				t.Fatalf("entries in routed stream = %d, want 1", len(entries))
			}
			select {
			case err := <-redisErrors:
				t.Fatalf("non-positive stream count emitted an error: %v", err)
			default:
			}
		})
	}
}
