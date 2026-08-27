package redis

import (
	"context"
	"errors"
	"testing"

	rds "github.com/redis/go-redis/v9"
)

type universalClientWrapper struct {
	rds.UniversalClient
}

func mustNewRedisClient(t *testing.T, ctx context.Context, client rds.UniversalClient) *RedisClient {
	t.Helper()
	rc, err := NewRedisClient(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	return rc
}

func mustNewRedisClientWithSub(t *testing.T, ctx context.Context, client, subClient rds.UniversalClient) *RedisClient {
	t.Helper()
	rc, err := NewRedisClientWithSub(ctx, client, subClient)
	if err != nil {
		t.Fatal(err)
	}
	return rc
}

func TestNewRedisClient(t *testing.T) {
	t.Run("with valid context and client", func(t *testing.T) {
		ctx := context.Background()
		client := rds.NewClient(&rds.Options{
			Addr: "localhost:6379",
		})
		defer func() { _ = client.Close() }()

		rc := mustNewRedisClient(t, ctx, client)

		if rc == nil {
			t.Fatal("Expected non-nil RedisClient")
		}
		if rc.Client() == nil {
			t.Fatal("Expected non-nil Client")
		}
		if rc.Context() != ctx {
			t.Fatal("Context mismatch")
		}
	})

	t.Run("with nil context", func(t *testing.T) {
		client := rds.NewClient(&rds.Options{
			Addr: "localhost:6379",
		})
		defer func() { _ = client.Close() }()

		var ctx context.Context
		rc := mustNewRedisClient(t, ctx, client)

		if rc == nil {
			t.Fatal("Expected non-nil RedisClient")
		}
		if rc.Context() == nil {
			t.Fatal("Expected non-nil Context (should default to Background)")
		}
	})

	t.Run("event emitter functionality", func(t *testing.T) {
		ctx := context.Background()
		client := rds.NewClient(&rds.Options{
			Addr: "localhost:6379",
		})
		defer func() { _ = client.Close() }()

		rc := mustNewRedisClient(t, ctx, client)

		// Test that EventEmitter is properly initialized
		called := false
		_ = rc.On("test", func(args ...any) {
			called = true
		})

		rc.Emit("test")

		if !called {
			t.Fatal("Event handler was not called")
		}
	})

	t.Run("error event handling", func(t *testing.T) {
		ctx := context.Background()
		client := rds.NewClient(&rds.Options{
			Addr: "localhost:6379",
		})
		defer func() { _ = client.Close() }()

		rc := mustNewRedisClient(t, ctx, client)

		var receivedError error
		_ = rc.On("error", func(args ...any) {
			if len(args) > 0 {
				if err, ok := args[0].(error); ok {
					receivedError = err
				}
			}
		})

		testErr := context.DeadlineExceeded
		rc.Emit("error", testErr)

		if receivedError != testErr {
			t.Fatalf("Expected error %v, got %v", testErr, receivedError)
		}
	})
}

func TestRedisClient_WithClusterClient(t *testing.T) {
	t.Run("cluster client creation", func(t *testing.T) {
		ctx := context.Background()
		client := rds.NewClusterClient(&rds.ClusterOptions{
			Addrs: []string{"localhost:7000", "localhost:7001", "localhost:7002"},
		})
		defer func() { _ = client.Close() }()

		rc := mustNewRedisClient(t, ctx, client)

		if rc == nil {
			t.Fatal("Expected non-nil RedisClient")
		}
		if rc.Client() == nil {
			t.Fatal("Expected non-nil Client")
		}
	})
}

func TestRedisClientRejectsReadOnlyPrimaryCluster(t *testing.T) {
	for name, options := range map[string]*rds.ClusterOptions{
		"read-only":        {ReadOnly: true},
		"route by latency": {RouteByLatency: true},
		"route randomly":   {RouteRandomly: true},
	} {
		t.Run(name, func(t *testing.T) {
			options.Addrs = []string{"127.0.0.1:7000"}
			primary := rds.NewClusterClient(options)
			t.Cleanup(func() { _ = primary.Close() })

			client, err := NewRedisClient(context.Background(), primary)
			if client != nil {
				t.Fatal("expected nil RedisClient")
			}
			if !errors.Is(err, ErrReadOnlyRedisClient) {
				t.Fatalf("error = %v, want %v", err, ErrReadOnlyRedisClient)
			}
		})
	}

	write := rds.NewClusterClient(&rds.ClusterOptions{Addrs: []string{"127.0.0.1:7000"}})
	read := rds.NewClusterClient(&rds.ClusterOptions{
		Addrs:    []string{"127.0.0.1:7000"},
		ReadOnly: true,
	})
	t.Cleanup(func() {
		_ = write.Close()
		_ = read.Close()
	})
	if _, err := NewRedisClientWithSub(context.Background(), write, read); err != nil {
		t.Fatalf("read-only subscription client was rejected: %v", err)
	}
}

func TestRedisClientOwnsSingleErrorFallback(t *testing.T) {
	client := rds.NewClient(&rds.Options{Addr: "127.0.0.1:6379"})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustNewRedisClient(t, context.Background(), client)

	if got := redisClient.ListenerCount("error"); got != 1 {
		t.Fatalf("error listeners = %d, want fallback listener", got)
	}
}

func TestRedisClientRequiresPrimaryClient(t *testing.T) {
	client, err := NewRedisClient(context.Background(), nil)
	if client != nil {
		t.Fatal("expected nil RedisClient")
	}
	if !errors.Is(err, ErrRedisClientRequired) {
		t.Fatalf("error = %v, want %v", err, ErrRedisClientRequired)
	}

	var typedNil *rds.Client
	client, err = NewRedisClient(context.Background(), typedNil)
	if client != nil || !errors.Is(err, ErrRedisClientRequired) {
		t.Fatalf("typed nil client = (%v, %v), want (nil, ErrRedisClientRequired)", client, err)
	}
}

func TestRedisClientRejectsRing(t *testing.T) {
	t.Run("write client", func(t *testing.T) {
		ring := rds.NewRing(&rds.RingOptions{Addrs: map[string]string{"shard": "127.0.0.1:6379"}})
		t.Cleanup(func() { _ = ring.Close() })

		client, err := NewRedisClient(context.Background(), ring)
		if client != nil {
			t.Fatal("expected nil RedisClient")
		}
		if !errors.Is(err, ErrUnsupportedRedisClient) {
			t.Fatalf("error = %v, want %v", err, ErrUnsupportedRedisClient)
		}
	})

	t.Run("subscription client", func(t *testing.T) {
		client := rds.NewClient(&rds.Options{Addr: "127.0.0.1:6379"})
		ring := rds.NewRing(&rds.RingOptions{Addrs: map[string]string{"shard": "127.0.0.1:6380"}})
		t.Cleanup(func() {
			_ = ring.Close()
			_ = client.Close()
		})

		redisClient, err := NewRedisClientWithSub(context.Background(), client, ring)
		if redisClient != nil {
			t.Fatal("expected nil RedisClient")
		}
		if !errors.Is(err, ErrUnsupportedRedisClient) {
			t.Fatalf("error = %v, want %v", err, ErrUnsupportedRedisClient)
		}
	})
}

func TestRedisClientAcceptsSupportedClients(t *testing.T) {
	clients := map[string]rds.UniversalClient{
		"standalone": rds.NewClient(&rds.Options{Addr: "127.0.0.1:6379"}),
		"cluster": rds.NewClusterClient(&rds.ClusterOptions{
			Addrs: []string{"127.0.0.1:7000"},
		}),
		"wrapper": &universalClientWrapper{rds.NewClient(&rds.Options{Addr: "127.0.0.1:6379"})},
	}
	for name, client := range clients {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(func() { _ = client.Close() })
			got := mustNewRedisClient(t, context.Background(), client)
			if got.Client() != client {
				t.Fatalf("Client = %T, want %T", got.Client(), client)
			}
		})
	}
}

func TestNewRedisClientWithSub(t *testing.T) {
	t.Run("with separate sub client", func(t *testing.T) {
		ctx := context.Background()
		pubClient := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		subClient := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		defer func() { _ = pubClient.Close() }()
		defer func() { _ = subClient.Close() }()

		rc := mustNewRedisClientWithSub(t, ctx, pubClient, subClient)

		if rc == nil {
			t.Fatal("Expected non-nil RedisClient")
		}
		if rc.Client() != pubClient {
			t.Fatal("Expected Client to be pubClient")
		}
		if rc.Sub() != subClient {
			t.Fatal("Sub() should return SubClient when set")
		}
	})
}

func TestRedisClient_Sub(t *testing.T) {
	t.Run("returns SubClient when set", func(t *testing.T) {
		pubClient := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		subClient := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		defer func() { _ = pubClient.Close() }()
		defer func() { _ = subClient.Close() }()

		rc := mustNewRedisClientWithSub(t, context.Background(), pubClient, subClient)

		if rc.Sub() != subClient {
			t.Fatal("Sub() should return SubClient")
		}
	})

	t.Run("falls back to Client when SubClient is nil", func(t *testing.T) {
		client := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		defer func() { _ = client.Close() }()

		rc := mustNewRedisClient(t, context.Background(), client)

		if rc.Sub() != client {
			t.Fatal("Sub() should fall back to Client when SubClient is nil")
		}
	})

	t.Run("falls back to Client when SubClient is typed nil", func(t *testing.T) {
		client := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		defer func() { _ = client.Close() }()
		var subClient *rds.Client

		rc := mustNewRedisClientWithSub(t, context.Background(), client, subClient)

		if rc.Sub() != client {
			t.Fatal("Sub() should fall back to Client when SubClient is typed nil")
		}
	})

	t.Run("backward compatibility with NewRedisClient", func(t *testing.T) {
		client := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		defer func() { _ = client.Close() }()

		rc := mustNewRedisClient(t, context.Background(), client)

		if rc.Sub() != rc.Client() {
			t.Fatal("Sub() should return Client when SubClient is nil")
		}
	})
}
