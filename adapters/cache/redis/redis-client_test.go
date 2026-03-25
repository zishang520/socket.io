package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
)

// newTestClient spins up a miniredis server and returns a connected RedisClient
// together with a cleanup function.
func newTestClient(t *testing.T) (*RedisClient, func()) {
	t.Helper()
	srv := miniredis.RunT(t)
	rdb := rds.NewClient(&rds.Options{Addr: srv.Addr()})
	client := NewRedisClient(context.Background(), rdb)
	return client, func() { _ = rdb.Close() }
}

// --- interface compliance ---

var _ cache.CacheClient = (*RedisClient)(nil)
var _ cache.CacheSubscription = (*redisSubscription)(nil)

// --- constructor ---

func TestNewRedisClient_NilContext(t *testing.T) {
	srv := miniredis.RunT(t)
	rdb := rds.NewClient(&rds.Options{Addr: srv.Addr()})
	c := NewRedisClient(nil, rdb) //nolint:staticcheck
	if c.Context() == nil {
		t.Fatal("Context() must not return nil when constructed with nil context")
	}
	if c.Context() != context.Background() {
		t.Error("nil context should be replaced with context.Background()")
	}
}

func TestNewRedisClient_ContextPreserved(t *testing.T) {
	srv := miniredis.RunT(t)
	rdb := rds.NewClient(&rds.Options{Addr: srv.Addr()})
	ctx := t.Context()
	c := NewRedisClient(ctx, rdb)
	if c.Context() != ctx {
		t.Error("Context() should return the context passed to NewRedisClient")
	}
}

// --- key-value ---

func TestRedisClient_Set_GetDel(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()
	ctx := context.Background()

	if err := client.Set(ctx, "key1", "value1", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := client.GetDel(ctx, "key1")
	if err != nil {
		t.Fatalf("GetDel: %v", err)
	}
	if got != "value1" {
		t.Errorf("GetDel: got %q, want %q", got, "value1")
	}

	// Key is deleted; second call must return ErrNil.
	_, err = client.GetDel(ctx, "key1")
	if !errors.Is(err, cache.ErrNil) {
		t.Errorf("GetDel on missing key: got %v, want cache.ErrNil", err)
	}
}

func TestRedisClient_Set_WithTTL(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()
	ctx := context.Background()

	if err := client.Set(ctx, "ttl-key", "hello", 5*time.Second); err != nil {
		t.Fatalf("Set with TTL: %v", err)
	}
	got, err := client.GetDel(ctx, "ttl-key")
	if err != nil {
		t.Fatalf("GetDel after Set with TTL: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestRedisClient_GetDel_MissingKey_ReturnsErrNil(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()

	_, err := client.GetDel(context.Background(), "does-not-exist")
	if !errors.Is(err, cache.ErrNil) {
		t.Errorf("expected cache.ErrNil for missing key, got %v", err)
	}
}

// --- streams ---

func TestRedisClient_XAdd_XRange(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()
	ctx := context.Background()

	id, err := client.XAdd(ctx, "stream1", 0, false, map[string]any{
		"field1": "val1",
		"field2": "val2",
	})
	if err != nil {
		t.Fatalf("XAdd: %v", err)
	}
	if id == "" {
		t.Fatal("XAdd returned empty ID")
	}

	entries, err := client.XRange(ctx, "stream1", "-", "+")
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("XRange: got %d entries, want 1", len(entries))
	}
	if entries[0].ID != id {
		t.Errorf("entry ID: got %q, want %q", entries[0].ID, id)
	}
	if entries[0].Values["field1"] != "val1" {
		t.Errorf("field1: got %v, want %q", entries[0].Values["field1"], "val1")
	}
}

func TestRedisClient_XAdd_WithMaxLen(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()
	ctx := context.Background()

	for i := range 5 {
		if _, err := client.XAdd(ctx, "capped", 3, true, map[string]any{"n": i}); err != nil {
			t.Fatalf("XAdd[%d]: %v", i, err)
		}
	}

	// miniredis enforces exact trim even for approximate; we just check no error.
	entries, err := client.XRange(ctx, "capped", "-", "+")
	if err != nil {
		t.Fatalf("XRange after capped XAdd: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected at least one stream entry")
	}
}

func TestRedisClient_XRangeN(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()
	ctx := context.Background()

	for i := range 5 {
		if _, err := client.XAdd(ctx, "stream2", 0, false, map[string]any{"i": i}); err != nil {
			t.Fatalf("XAdd[%d]: %v", i, err)
		}
	}

	entries, err := client.XRangeN(ctx, "stream2", "-", "+", 3)
	if err != nil {
		t.Fatalf("XRangeN: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("XRangeN(count=3): got %d entries, want 3", len(entries))
	}
}

func TestRedisClient_XRead(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()
	ctx := context.Background()

	if _, err := client.XAdd(ctx, "stream3", 0, false, map[string]any{"msg": "hello"}); err != nil {
		t.Fatalf("XAdd: %v", err)
	}

	streams, err := client.XRead(ctx, []string{"stream3"}, "0", 10, 0)
	if err != nil {
		t.Fatalf("XRead: %v", err)
	}
	if len(streams) == 0 {
		t.Fatal("XRead: no streams returned")
	}
	if len(streams[0].Messages) == 0 {
		t.Fatal("XRead: no messages in stream")
	}
	if streams[0].Messages[0].Values["msg"] != "hello" {
		t.Errorf("XRead message: got %v, want %q", streams[0].Messages[0].Values["msg"], "hello")
	}
}

// --- pub/sub ---

func TestRedisClient_Subscribe_Publish(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sub := client.Subscribe(ctx, "test-channel")
	defer sub.Close() //nolint:errcheck

	// Give the subscription goroutine time to register with miniredis.
	time.Sleep(50 * time.Millisecond)

	if err := client.Publish(ctx, "test-channel", "hello"); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case msg, ok := <-sub.C():
		if !ok {
			t.Fatal("subscription channel closed unexpectedly")
		}
		if msg.Channel != "test-channel" {
			t.Errorf("Channel: got %q, want %q", msg.Channel, "test-channel")
		}
		if string(msg.Payload) != "hello" {
			t.Errorf("Payload: got %q, want %q", string(msg.Payload), "hello")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for pub/sub message")
	}
}

func TestRedisClient_Unsubscribe_ClosesChannel(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sub := client.Subscribe(ctx, "close-channel")
	time.Sleep(50 * time.Millisecond)

	if err := sub.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Channel must drain and close within the timeout.
	for {
		select {
		case _, ok := <-sub.C():
			if !ok {
				return // channel closed as expected
			}
		case <-ctx.Done():
			t.Fatal("channel not closed after subscription Close()")
		}
	}
}

func TestRedisClient_PubSubNumSub(t *testing.T) {
	client, cleanup := newTestClient(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	sub := client.Subscribe(ctx, "numsub-chan")
	defer sub.Close() //nolint:errcheck
	time.Sleep(50 * time.Millisecond)

	counts, err := client.PubSubNumSub(ctx, "numsub-chan")
	if err != nil {
		t.Fatalf("PubSubNumSub: %v", err)
	}
	if counts["numsub-chan"] < 1 {
		t.Errorf("expected at least 1 subscriber on numsub-chan, got %d", counts["numsub-chan"])
	}
}
