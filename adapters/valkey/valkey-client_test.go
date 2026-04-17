package valkey_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	vk "github.com/valkey-io/valkey-go"
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// newMiniValkeyClient starts an in-memory Redis server (miniredis) and returns
// a ValkeyClient connected to it. The server is cleaned up when the test ends.
func newMiniValkeyClient(t *testing.T) (*valkey.ValkeyClient, *miniredis.Miniredis) {
	t.Helper()
	s := miniredis.RunT(t)
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{s.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("failed to connect to miniredis: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return valkey.NewValkeyClient(context.Background(), client), s
}

// --- Constructor tests ---

func TestNewValkeyClient(t *testing.T) {
	vc, _ := newMiniValkeyClient(t)
	if vc == nil {
		t.Fatal("expected non-nil ValkeyClient")
	}
	if vc.Client == nil {
		t.Fatal("expected non-nil Client field")
	}
}

func TestNewValkeyClient_NilContext(t *testing.T) {
	s := miniredis.RunT(t)
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{s.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	//nolint:staticcheck
	vc := valkey.NewValkeyClient(nil, client)
	if vc.Context == nil {
		t.Fatal("expected non-nil Context when nil was passed")
	}
}

func TestNewValkeyClientWithSub(t *testing.T) {
	t.Run("with separate sub client", func(t *testing.T) {
		s := miniredis.RunT(t)
		pubClient, err := vk.NewClient(vk.ClientOption{
			InitAddress:  []string{s.Addr()},
			DisableCache: true,
		})
		if err != nil {
			t.Fatalf("failed to connect pubClient: %v", err)
		}
		t.Cleanup(func() { pubClient.Close() })

		subClient, err := vk.NewClient(vk.ClientOption{
			InitAddress:  []string{s.Addr()},
			DisableCache: true,
		})
		if err != nil {
			t.Fatalf("failed to connect subClient: %v", err)
		}
		t.Cleanup(func() { subClient.Close() })

		vc := valkey.NewValkeyClientWithSub(context.Background(), pubClient, subClient)

		if vc == nil {
			t.Fatal("expected non-nil ValkeyClient")
		}
		if vc.Client != pubClient {
			t.Fatal("expected Client to be pubClient")
		}
		if vc.SubClient != subClient {
			t.Fatal("expected SubClient to be subClient")
		}
		if vc.Sub() != subClient {
			t.Fatal("Sub() should return SubClient when set")
		}
	})

	t.Run("nil context defaults to Background", func(t *testing.T) {
		s := miniredis.RunT(t)
		pubClient, err := vk.NewClient(vk.ClientOption{
			InitAddress:  []string{s.Addr()},
			DisableCache: true,
		})
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		t.Cleanup(func() { pubClient.Close() })

		subClient, err := vk.NewClient(vk.ClientOption{
			InitAddress:  []string{s.Addr()},
			DisableCache: true,
		})
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		t.Cleanup(func() { subClient.Close() })

		//nolint:staticcheck
		vc := valkey.NewValkeyClientWithSub(nil, pubClient, subClient)
		if vc.Context == nil {
			t.Fatal("expected non-nil Context when nil was passed")
		}
	})
}

// --- Sub() routing tests ---

func TestValkeyClient_Sub(t *testing.T) {
	t.Run("returns SubClient when set", func(t *testing.T) {
		s := miniredis.RunT(t)
		pubClient, err := vk.NewClient(vk.ClientOption{
			InitAddress:  []string{s.Addr()},
			DisableCache: true,
		})
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		t.Cleanup(func() { pubClient.Close() })

		subClient, err := vk.NewClient(vk.ClientOption{
			InitAddress:  []string{s.Addr()},
			DisableCache: true,
		})
		if err != nil {
			t.Fatalf("failed to connect: %v", err)
		}
		t.Cleanup(func() { subClient.Close() })

		vc := valkey.NewValkeyClientWithSub(context.Background(), pubClient, subClient)
		if vc.Sub() != subClient {
			t.Fatal("Sub() should return SubClient")
		}
	})

	t.Run("falls back to Client when SubClient is nil", func(t *testing.T) {
		vc, _ := newMiniValkeyClient(t)
		if vc.Sub() != vc.Client {
			t.Fatal("Sub() should fall back to Client when SubClient is nil")
		}
	})

	t.Run("backward compatibility with NewValkeyClient", func(t *testing.T) {
		vc, _ := newMiniValkeyClient(t)
		if vc.SubClient != nil {
			t.Fatal("SubClient should be nil when using NewValkeyClient")
		}
		if vc.Sub() != vc.Client {
			t.Fatal("Sub() should return Client when SubClient is nil")
		}
	})
}

// --- Pub/Sub lifecycle tests ---

func TestValkeyPubSub_Close(t *testing.T) {
	vc, _ := newMiniValkeyClient(t)

	pubsub := vc.Subscribe(vc.Context, "test-channel")
	if closeErr := pubsub.Close(); closeErr != nil {
		t.Fatalf("expected no error on Close, got %v", closeErr)
	}

	if closeErr := pubsub.Close(); closeErr != nil {
		t.Fatalf("expected no error on second Close (idempotent), got %v", closeErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, err := pubsub.ReceiveMessage(ctx)
	if err == nil {
		t.Fatal("expected error after Close, got nil")
	}
}

func TestValkeyClient_PubSub(t *testing.T) {
	vc, _ := newMiniValkeyClient(t)
	ctx := vc.Context

	channel := "sio:test:pubsub"
	pubsub := vc.Subscribe(ctx, channel)
	defer pubsub.Close() //nolint:errcheck

	done := make(chan string, 1)
	go func() {
		msg, recvErr := pubsub.ReceiveMessage(ctx)
		if recvErr == nil {
			done <- msg.Payload
		} else {
			done <- ""
		}
	}()

	time.Sleep(100 * time.Millisecond)
	if err := vc.Publish(ctx, channel, []byte("hello")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case payload := <-done:
		if payload != "hello" {
			t.Fatalf("expected 'hello', got %q", payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for message")
	}
}

// --- Unsubscribe tests (the bug the maintainer flagged) ---

func TestValkeyPubSub_Unsubscribe_PerChannel(t *testing.T) {
	vc, _ := newMiniValkeyClient(t)
	ctx := vc.Context

	ch1 := "sio:test:unsub:ch1"
	ch2 := "sio:test:unsub:ch2"
	pubsub := vc.Subscribe(ctx, ch1, ch2)
	defer pubsub.Close() //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	// Unsubscribe from ch1 only — must not kill the entire subscription.
	if err := pubsub.Unsubscribe(ctx, ch1); err != nil {
		t.Fatalf("Unsubscribe ch1: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// ch2 should still be active.
	if err := vc.Publish(ctx, ch2, []byte("still-alive")); err != nil {
		t.Fatalf("Publish to ch2: %v", err)
	}

	msgCtx, msgCancel := context.WithTimeout(ctx, 3*time.Second)
	defer msgCancel()

	msg, err := pubsub.ReceiveMessage(msgCtx)
	if err != nil {
		t.Fatalf("expected message on ch2 after unsubscribing ch1, got error: %v", err)
	}
	if msg.Payload != "still-alive" {
		t.Fatalf("expected 'still-alive', got %q", msg.Payload)
	}
}

func TestValkeyPubSub_Unsubscribe_NoMessagesOnUnsubbed(t *testing.T) {
	vc, _ := newMiniValkeyClient(t)
	ctx := vc.Context

	ch1 := "sio:test:unsub2:ch1"
	ch2 := "sio:test:unsub2:ch2"
	pubsub := vc.Subscribe(ctx, ch1, ch2)
	defer pubsub.Close() //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	if err := pubsub.Unsubscribe(ctx, ch1); err != nil {
		t.Fatalf("Unsubscribe ch1: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Publish to the unsubscribed channel, then to the subscribed one.
	if err := vc.Publish(ctx, ch1, []byte("should-not-arrive")); err != nil {
		t.Fatalf("Publish ch1: %v", err)
	}
	if err := vc.Publish(ctx, ch2, []byte("expected")); err != nil {
		t.Fatalf("Publish ch2: %v", err)
	}

	msgCtx, msgCancel := context.WithTimeout(ctx, 3*time.Second)
	defer msgCancel()

	msg, err := pubsub.ReceiveMessage(msgCtx)
	if err != nil {
		t.Fatalf("ReceiveMessage: %v", err)
	}
	if msg.Payload != "expected" {
		t.Fatalf("received unexpected payload %q (should have been 'expected' from ch2)", msg.Payload)
	}
	if msg.Channel != ch2 {
		t.Fatalf("received message from wrong channel %q (expected %q)", msg.Channel, ch2)
	}
}

func TestValkeyPubSub_PSubscribe_And_PUnsubscribe(t *testing.T) {
	vc, _ := newMiniValkeyClient(t)
	ctx := vc.Context

	pubsub := vc.PSubscribe(ctx, "sio:test:punsub:*")
	defer pubsub.Close() //nolint:errcheck

	time.Sleep(100 * time.Millisecond)

	if err := vc.Publish(ctx, "sio:test:punsub:foo", []byte("pattern-msg")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	msgCtx, msgCancel := context.WithTimeout(ctx, 3*time.Second)
	defer msgCancel()

	msg, err := pubsub.ReceiveMessage(msgCtx)
	if err != nil {
		t.Fatalf("expected message from pattern subscription, got error: %v", err)
	}
	if msg.Payload != "pattern-msg" {
		t.Fatalf("expected 'pattern-msg', got %q", msg.Payload)
	}
	if msg.Pattern == "" {
		t.Fatal("expected non-empty Pattern for pattern subscription message")
	}

	// PUnsubscribe should issue the command without error.
	if err := pubsub.PUnsubscribe(ctx, "sio:test:punsub:*"); err != nil {
		t.Fatalf("PUnsubscribe: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// After PUnsubscribe, publishing should not deliver messages.
	if err := vc.Publish(ctx, "sio:test:punsub:bar", []byte("should-not-arrive")); err != nil {
		t.Fatalf("Publish after PUnsubscribe: %v", err)
	}

	timeoutCtx, timeoutCancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer timeoutCancel()

	_, err = pubsub.ReceiveMessage(timeoutCtx)
	if err == nil {
		t.Fatal("expected timeout or error after PUnsubscribe, got message")
	}
}

// --- Read/write separation tests ---

func TestValkeyPubSub_WithSubClient(t *testing.T) {
	s := miniredis.RunT(t)

	pubClient, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{s.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("failed to connect pubClient: %v", err)
	}
	t.Cleanup(func() { pubClient.Close() })

	subClient, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{s.Addr()},
		DisableCache: true,
	})
	if err != nil {
		t.Fatalf("failed to connect subClient: %v", err)
	}
	t.Cleanup(func() { subClient.Close() })

	vc := valkey.NewValkeyClientWithSub(context.Background(), pubClient, subClient)
	ctx := vc.Context

	channel := "sio:test:subclient:pubsub"
	pubsub := vc.Subscribe(ctx, channel)
	defer pubsub.Close() //nolint:errcheck

	done := make(chan string, 1)
	go func() {
		msg, recvErr := pubsub.ReceiveMessage(ctx)
		if recvErr == nil {
			done <- msg.Payload
		} else {
			done <- ""
		}
	}()

	time.Sleep(100 * time.Millisecond)
	if err := vc.Publish(ctx, channel, []byte("via-subclient")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case payload := <-done:
		if payload != "via-subclient" {
			t.Fatalf("expected 'via-subclient', got %q", payload)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for message with SubClient")
	}
}

// --- Key-value tests ---

func TestValkeyClient_SetGetDel(t *testing.T) {
	vc, _ := newMiniValkeyClient(t)
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

	val2, err := vc.GetDel(ctx, key)
	if err != nil {
		t.Fatalf("GetDel on missing key: %v", err)
	}
	if val2 != "" {
		t.Fatalf("expected empty string for missing key, got %q", val2)
	}
}

func TestValkeyClient_XAddXRange(t *testing.T) {
	vc, _ := newMiniValkeyClient(t)
	ctx := vc.Context
	stream := "sio:test:stream"

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

// --- Pure-logic tests (no server needed) ---

func TestSubscriptionMode_ShouldUseDynamicChannel(t *testing.T) {
	tests := []struct {
		mode  valkey.SubscriptionMode
		room  string
		wants bool
	}{
		{valkey.StaticSubscriptionMode, "room1", false},
		{valkey.StaticSubscriptionMode, "abcdefghijklmnopqrst", false},
		{valkey.DynamicSubscriptionMode, "room1", true},
		{valkey.DynamicSubscriptionMode, "abcdefghijklmnopqrst", false},
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
	var p *valkey.ValkeyPacket
	data, err := p.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON of nil: %v", err)
	}
	if string(data) != "null" {
		t.Fatalf("expected 'null', got %s", data)
	}

	if err := p.UnmarshalJSON([]byte(`["uid"]`)); err == nil {
		t.Fatal("expected error when unmarshaling into nil ValkeyPacket")
	}
}
