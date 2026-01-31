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
		defer client.Close()

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
		defer client.Close()

		rc := NewRedisClient(nil, client)

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
		defer client.Close()

		rc := NewRedisClient(ctx, client)

		// Test that EventEmitter is properly initialized
		called := false
		rc.On("test", func(args ...any) {
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
		defer client.Close()

		rc := NewRedisClient(ctx, client)

		var receivedError error
		rc.On("error", func(args ...any) {
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
		defer client.Close()

		rc := NewRedisClient(ctx, client)

		if rc == nil {
			t.Fatal("Expected non-nil RedisClient")
		}
		if rc.Client == nil {
			t.Fatal("Expected non-nil Client")
		}
	})
}
