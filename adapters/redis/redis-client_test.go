package redis

import (
	"context"
	"testing"

	rds "github.com/redis/go-redis/v9"
)

func TestNewRedisClient(t *testing.T) {
	t.Run("with valid context and client", func(t *testing.T) {
		ctx := context.Background()
		client := rds.NewClient(&rds.Options{
			Addr: "localhost:6379",
		})
		defer func() { _ = client.Close() }()

		rc := NewRedisClient(ctx, client)

		if rc == nil {
			t.Fatal("Expected non-nil RedisClient")
		}
		if rc.Client == nil {
			t.Fatal("Expected non-nil Client")
		}
		if rc.Context != ctx {
			t.Fatal("Context mismatch")
		}
	})

	t.Run("with nil context", func(t *testing.T) {
		client := rds.NewClient(&rds.Options{
			Addr: "localhost:6379",
		})
		defer func() { _ = client.Close() }()

		rc := NewRedisClient(context.TODO(), client)

		if rc == nil {
			t.Fatal("Expected non-nil RedisClient")
		}
		if rc.Context == nil {
			t.Fatal("Expected non-nil Context (should default to Background)")
		}
	})

	t.Run("event emitter functionality", func(t *testing.T) {
		ctx := context.Background()
		client := rds.NewClient(&rds.Options{
			Addr: "localhost:6379",
		})
		defer func() { _ = client.Close() }()

		rc := NewRedisClient(ctx, client)

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

		rc := NewRedisClient(ctx, client)

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

		rc := NewRedisClient(ctx, client)

		if rc == nil {
			t.Fatal("Expected non-nil RedisClient")
		}
		if rc.Client == nil {
			t.Fatal("Expected non-nil Client")
		}
	})
}

func TestNewRedisClientWithSub(t *testing.T) {
	t.Run("with separate sub client", func(t *testing.T) {
		ctx := context.Background()
		pubClient := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		subClient := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		defer func() { _ = pubClient.Close() }()
		defer func() { _ = subClient.Close() }()

		rc := NewRedisClientWithSub(ctx, pubClient, subClient)

		if rc == nil {
			t.Fatal("Expected non-nil RedisClient")
		}
		if rc.Client != pubClient {
			t.Fatal("Expected Client to be pubClient")
		}
		if rc.SubClient != subClient {
			t.Fatal("Expected SubClient to be subClient")
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

		rc := NewRedisClientWithSub(context.Background(), pubClient, subClient)

		if rc.Sub() != subClient {
			t.Fatal("Sub() should return SubClient")
		}
	})

	t.Run("falls back to Client when SubClient is nil", func(t *testing.T) {
		client := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		defer func() { _ = client.Close() }()

		rc := NewRedisClient(context.Background(), client)

		if rc.Sub() != client {
			t.Fatal("Sub() should fall back to Client when SubClient is nil")
		}
	})

	t.Run("backward compatibility with NewRedisClient", func(t *testing.T) {
		client := rds.NewClient(&rds.Options{Addr: "localhost:6379"})
		defer func() { _ = client.Close() }()

		rc := NewRedisClient(context.Background(), client)

		if rc.SubClient != nil {
			t.Fatal("SubClient should be nil when using NewRedisClient")
		}
		if rc.Sub() != rc.Client {
			t.Fatal("Sub() should return Client when SubClient is nil")
		}
	})
}
