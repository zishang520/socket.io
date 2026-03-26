package valkey_test

import (
	"context"
	"testing"
	"time"

	vk "github.com/valkey-io/valkey-go"
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// mockValkeyClient creates a real Valkey client pointed at a local instance
// for integration tests. Unit tests use table-driven checks on public API.

func TestNewValkeyClient(t *testing.T) {
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:      []string{"127.0.0.1:6379"},
		DisableCache:     true,
		ConnWriteTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Skip("valkey not available:", err)
	}
	defer client.Close()

	vc := valkey.NewValkeyClient(context.Background(), client)
	if vc == nil {
		t.Fatal("expected non-nil ValkeyClient")
	}
	if vc.Client == nil {
		t.Fatal("expected non-nil Client field")
	}
}

func TestNewValkeyClient_NilContext(t *testing.T) {
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:      []string{"127.0.0.1:6379"},
		DisableCache:     true,
		ConnWriteTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Skip("valkey not available:", err)
	}
	defer client.Close()

	//nolint:staticcheck
	vc := valkey.NewValkeyClient(nil, client)
	if vc.Context == nil {
		t.Fatal("expected non-nil Context when nil was passed")
	}
}

func TestValkeyPubSub_Close(t *testing.T) {
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:      []string{"127.0.0.1:6379"},
		DisableCache:     true,
		ConnWriteTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Skip("valkey not available:", err)
	}
	defer client.Close()

	vc := valkey.NewValkeyClient(context.Background(), client)

	pubsub := vc.Subscribe(vc.Context, "test-channel")
	if err := pubsub.Close(); err != nil {
		t.Fatalf("expected no error on Close, got %v", err)
	}

	// Closing again should be idempotent.
	if err := pubsub.Close(); err != nil {
		t.Fatalf("expected no error on second Close, got %v", err)
	}

	// ReceiveMessage after close should return ErrValkeyPubSubClosed or ctx error.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err = pubsub.ReceiveMessage(ctx)
	if err == nil {
		t.Fatal("expected error after Close, got nil")
	}
}

func TestValkeyClient_SetGet(t *testing.T) {
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:      []string{"127.0.0.1:6379"},
		DisableCache:     true,
		ConnWriteTimeout: 500 * time.Millisecond,
	})
	if err != nil {
		t.Skip("valkey not available:", err)
	}
	defer client.Close()

	vc := valkey.NewValkeyClient(context.Background(), client)
	ctx := vc.Context

	key := "sio:test:setget"

	if err := vc.Set(ctx, key, "hello", 10*time.Second); err != nil {
		t.Fatalf("Set: %v", err)
	}

	val, err := vc.GetDel(ctx, key)
	if err != nil {
		t.Fatalf("GetDel: %v", err)
	}
	if val != "hello" {
		t.Fatalf("expected 'hello', got %q", val)
	}

	// Second GetDel on deleted key should return ("", nil).
	val2, err := vc.GetDel(ctx, key)
	if err != nil {
		t.Fatalf("GetDel on missing key: %v", err)
	}
	if val2 != "" {
		t.Fatalf("expected empty string for missing key, got %q", val2)
	}
}

func TestValkeyClient_XAddXRange(t *testing.T) {
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:      []string{"127.0.0.1:6379"},
		DisableCache:     true,
		ConnWriteTimeout: 500 * time.Millisecond,
	})
	if err != nil {
		t.Skip("valkey not available:", err)
	}
	defer client.Close()

	vc := valkey.NewValkeyClient(context.Background(), client)
	ctx := vc.Context
	stream := "sio:test:stream"

	// Clean up
	defer vc.Client.Do(ctx, vc.Client.B().Del().Key(stream).Build()) //nolint:errcheck

	entryID, err := vc.XAdd(ctx, stream, 1000, map[string]any{"uid": "test", "nsp": "/"})
	if err != nil {
		t.Fatalf("XAdd: %v", err)
	}
	if entryID == "" {
		t.Fatal("expected non-empty entry ID")
	}

	entries, err := vc.XRange(ctx, stream, "-", "+")
	if err != nil {
		t.Fatalf("XRange: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least one entry from XRange")
	}
}

func TestValkeyClient_PubSub(t *testing.T) {
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:      []string{"127.0.0.1:6379"},
		DisableCache:     true,
		ConnWriteTimeout: 500 * time.Millisecond,
	})
	if err != nil {
		t.Skip("valkey not available:", err)
	}
	defer client.Close()

	vc := valkey.NewValkeyClient(context.Background(), client)
	ctx := vc.Context

	channel := "sio:test:pubsub"
	pubsub := vc.Subscribe(ctx, channel)
	defer pubsub.Close() //nolint:errcheck

	done := make(chan string, 1)
	go func() {
		msg, err := pubsub.ReceiveMessage(ctx)
		if err == nil {
			done <- msg.Payload
		} else {
			done <- ""
		}
	}()

	time.Sleep(50 * time.Millisecond)
	if err := vc.Publish(ctx, channel, []byte("hello")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case payload := <-done:
		if payload != "hello" {
			t.Fatalf("expected 'hello', got %q", payload)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestSubscriptionMode_ShouldUseDynamicChannel(t *testing.T) {
	tests := []struct {
		mode  valkey.SubscriptionMode
		room  string
		wants bool
	}{
		{valkey.StaticSubscriptionMode, "room1", false},
		{valkey.StaticSubscriptionMode, "abcdefghijklmnopqrst", false}, // len 20 (private)
		{valkey.DynamicSubscriptionMode, "room1", true},
		{valkey.DynamicSubscriptionMode, "abcdefghijklmnopqrst", false}, // private room
		{valkey.DynamicPrivateSubscriptionMode, "abcdefghijklmnopqrst", true},
		{valkey.DynamicPrivateSubscriptionMode, "room1", true},
	}

	for _, tc := range tests {
		got := valkey.ShouldUseDynamicChannel(tc.mode, socket.Room(tc.room))
		if got != tc.wants {
			t.Errorf("ShouldUseDynamicChannel(%q, %q) = %v, want %v", tc.mode, tc.room, got, tc.wants)
		}
	}
}

func TestValkeyPacket_MarshalUnmarshal(t *testing.T) {
	// A nil packet should marshal to null.
	var p *valkey.ValkeyPacket
	data, err := p.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON of nil: %v", err)
	}
	if string(data) != "null" {
		t.Fatalf("expected 'null', got %s", data)
	}

	// Unmarshal into nil should return error.
	if err := p.UnmarshalJSON([]byte(`["uid"]`)); err == nil {
		t.Fatal("expected error when unmarshaling into nil ValkeyPacket")
	}
}
