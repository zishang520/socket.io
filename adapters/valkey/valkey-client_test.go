package valkey_test

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	miniredisserver "github.com/alicebob/miniredis/v2/server"
	vk "github.com/valkey-io/valkey-go"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
)

type embeddedValkeyClient struct {
	vk.Client
}

func newRawClient(t *testing.T, address string) vk.Client {
	t.Helper()
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{address},
		DisableCache: true,
		AlwaysRESP2:  true,
	})
	if err != nil {
		t.Fatalf("valkey.NewClient: %v", err)
	}
	t.Cleanup(client.Close)
	return client
}

func newMiniValkeyClient(t *testing.T) (*miniredis.Miniredis, *valkey.ValkeyClient) {
	t.Helper()
	server := miniredis.RunT(t)
	client, err := valkey.NewValkeyClient(context.Background(), newRawClient(t, server.Addr()))
	if err != nil {
		t.Fatalf("NewValkeyClient: %v", err)
	}
	return server, client
}

func TestNewValkeyClient(t *testing.T) {
	_, client := newMiniValkeyClient(t)
	if client.Client() == nil || client.Sub() != client.Client() || client.Context() == nil {
		t.Fatal("expected one initialized client for both roles")
	}
}

func TestNewValkeyClientRequiresClient(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		client, err := valkey.NewValkeyClient(context.Background(), nil)
		if client != nil || !errors.Is(err, valkey.ErrValkeyClientRequired) {
			t.Fatalf("NewValkeyClient() = (%v, %v)", client, err)
		}
	})

	t.Run("typed nil", func(t *testing.T) {
		var raw *embeddedValkeyClient
		client, err := valkey.NewValkeyClient(context.Background(), raw)
		if client != nil || !errors.Is(err, valkey.ErrValkeyClientRequired) {
			t.Fatalf("NewValkeyClient() = (%v, %v)", client, err)
		}
	})
}

func TestNewValkeyClientWithSub(t *testing.T) {
	server := miniredis.RunT(t)
	primary := newRawClient(t, server.Addr())
	subscriber := newRawClient(t, server.Addr())

	//nolint:staticcheck
	client, err := valkey.NewValkeyClientWithSub(nil, primary, subscriber)
	if err != nil {
		t.Fatal(err)
	}
	if client.Client() != primary || client.Sub() != subscriber || client.Context() == nil {
		t.Fatal("client roles or default context were not preserved")
	}

	client, err = valkey.NewValkeyClientWithSub(context.Background(), primary, nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.Sub() != primary {
		t.Fatal("nil subscription client should use the primary client")
	}
}

func TestValkeyPubSubIsReadyOnReturn(t *testing.T) {
	_, client := newMiniValkeyClient(t)
	ctx := client.Context()
	channel := "sio:test:ready"
	pubSub := client.Subscribe(ctx, channel)
	defer pubSub.Close() //nolint:errcheck

	if err := client.Publish(ctx, channel, []byte("hello")); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	receiveCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	message, err := pubSub.ReceiveMessage(receiveCtx)
	if err != nil {
		t.Fatal(err)
	}
	if message.Channel != channel || message.Message != "hello" {
		t.Fatalf("unexpected message: %+v", message)
	}
}

func TestValkeyPatternPubSub(t *testing.T) {
	_, client := newMiniValkeyClient(t)
	ctx := client.Context()
	pubSub := client.PSubscribe(ctx, "sio:test:pattern:*")
	defer pubSub.Close() //nolint:errcheck

	if err := client.Publish(ctx, "sio:test:pattern:room", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	receiveCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	message, err := pubSub.ReceiveMessage(receiveCtx)
	if err != nil {
		t.Fatal(err)
	}
	if message.Pattern == "" || message.Message != "payload" {
		t.Fatalf("unexpected pattern message: %+v", message)
	}
}

func TestValkeyPubSubCloseIsIdempotent(t *testing.T) {
	_, client := newMiniValkeyClient(t)
	pubSub := client.Subscribe(client.Context(), "sio:test:close")
	if err := pubSub.Close(); err != nil {
		t.Fatal(err)
	}
	if err := pubSub.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := pubSub.ReceiveMessage(context.Background()); !errors.Is(err, valkey.ErrValkeyPubSubClosed) {
		t.Fatalf("ReceiveMessage error = %v", err)
	}
}

func TestValkeyPubSubCloseReturnsCleanupError(t *testing.T) {
	server, client := newMiniValkeyClient(t)
	pubSub := client.Subscribe(client.Context(), "sio:test:close-error")

	errorEvent := make(chan error, 1)
	if err := client.On("error", func(args ...any) {
		if err, ok := args[0].(error); ok {
			select {
			case errorEvent <- err:
			default:
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	server.Server().SetPreHook(func(peer *miniredisserver.Peer, command string, _ ...string) bool {
		if command != "UNSUBSCRIBE" {
			return false
		}
		attempts.Add(1)
		peer.WriteError("NOPERM unsubscribe denied")
		return true
	})
	defer server.Server().SetPreHook(nil)

	closeErr := pubSub.Close()
	if closeErr == nil || closeErr.Error() != "NOPERM unsubscribe denied" {
		t.Fatalf("Close() error = %v", closeErr)
	}
	if got := attempts.Load(); got != 1 {
		t.Fatalf("UNSUBSCRIBE attempts = %d, want 1", got)
	}

	select {
	case eventErr := <-errorEvent:
		if eventErr != closeErr {
			t.Fatalf("error event = %v, want %v", eventErr, closeErr)
		}
	case <-time.After(time.Second):
		t.Fatal("cleanup error event was not emitted")
	}
}

func TestValkeyPubSubCloseRetriesTryAgain(t *testing.T) {
	server, client := newMiniValkeyClient(t)
	channel := "sio:test:close-retry"
	pubSub := client.Subscribe(client.Context(), channel)

	var attempts atomic.Int32
	server.Server().SetPreHook(func(peer *miniredisserver.Peer, command string, _ ...string) bool {
		if command != "UNSUBSCRIBE" {
			return false
		}
		if attempts.Add(1) < 3 {
			peer.WriteError("TRYAGAIN unsubscribe retry")
			return true
		}
		return false
	})
	defer server.Server().SetPreHook(nil)

	if err := pubSub.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("UNSUBSCRIBE attempts = %d, want 3", got)
	}
	if got := server.PubSubNumSub(channel)[channel]; got != 0 {
		t.Fatalf("subscriber count after close = %d, want 0", got)
	}
}

func TestValkeyPubSubDeduplicatesTopicsAndUnsubscribesOnClose(t *testing.T) {
	server, client := newMiniValkeyClient(t)
	ctx := client.Context()
	channel := "sio:test:shared"
	pubSub := client.Subscribe(ctx, channel, channel)

	if got := server.PubSubNumSub(channel)[channel]; got != 1 {
		t.Fatalf("subscriber count = %d, want 1", got)
	}
	if err := pubSub.Close(); err != nil {
		t.Fatal(err)
	}
	if got := server.PubSubNumSub(channel)[channel]; got != 0 {
		t.Fatalf("subscriber count after close = %d, want 0", got)
	}
}

func TestValkeyPubSubSharesOverlappingSubscriptions(t *testing.T) {
	server, client := newMiniValkeyClient(t)
	ctx := client.Context()
	firstCtx, cancelFirst := context.WithCancel(ctx)
	secondCtx, cancelSecond := context.WithCancel(ctx)
	defer cancelFirst()
	defer cancelSecond()
	firstOnly := "sio:test:overlap:first"
	shared := "sio:test:overlap:shared"
	secondOnly := "sio:test:overlap:second"
	first := client.Subscribe(firstCtx, firstOnly, shared)
	second := client.Subscribe(secondCtx, shared, secondOnly)

	counts := server.PubSubNumSub(firstOnly, shared, secondOnly)
	if counts[firstOnly] != 1 || counts[shared] != 1 || counts[secondOnly] != 1 {
		t.Fatalf("subscriber counts = %v, want one physical subscription per topic", counts)
	}
	if err := client.Publish(ctx, shared, []byte("shared")); err != nil {
		t.Fatal(err)
	}
	for index, pubSub := range []*valkey.ValkeyPubSub{first, second} {
		receiveCtx, cancel := context.WithTimeout(ctx, time.Second)
		message, err := pubSub.ReceiveMessage(receiveCtx)
		cancel()
		if err != nil || message.Message != "shared" {
			t.Fatalf("subscription %d = %+v, %v", index, message, err)
		}
	}

	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	counts = server.PubSubNumSub(firstOnly, shared, secondOnly)
	if counts[firstOnly] != 0 || counts[shared] != 1 || counts[secondOnly] != 1 {
		t.Fatalf("subscriber counts after first close = %v", counts)
	}
	if err := client.Publish(ctx, shared, []byte("remaining")); err != nil {
		t.Fatal(err)
	}
	receiveCtx, cancel := context.WithTimeout(ctx, time.Second)
	message, err := second.ReceiveMessage(receiveCtx)
	cancel()
	if err != nil || message.Message != "remaining" {
		t.Fatalf("remaining subscription = %+v, %v", message, err)
	}

	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	counts = server.PubSubNumSub(firstOnly, shared, secondOnly)
	if counts[firstOnly] != 0 || counts[shared] != 0 || counts[secondOnly] != 0 {
		t.Fatalf("subscriber counts after final close = %v", counts)
	}
}

func TestValkeyPubSubReturnsAfterInitialSubscriptionError(t *testing.T) {
	server, client := newMiniValkeyClient(t)
	channel := "sio:test:initial-error"
	server.Server().SetPreHook(func(peer *miniredisserver.Peer, command string, _ ...string) bool {
		if command != "SUBSCRIBE" {
			return false
		}
		peer.WriteError("NOPERM subscription denied")
		return true
	})
	t.Cleanup(func() { server.Server().SetPreHook(nil) })

	errorEvent := make(chan error, 1)
	if err := client.On("error", func(args ...any) {
		if err, ok := args[0].(error); ok {
			select {
			case errorEvent <- err:
			default:
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	returned := make(chan *valkey.ValkeyPubSub, 1)
	go func() { returned <- client.Subscribe(client.Context(), channel) }()

	var pubSub *valkey.ValkeyPubSub
	select {
	case pubSub = <-returned:
	case <-time.After(time.Second):
		t.Fatal("Subscribe blocked after the initial server error")
	}
	defer pubSub.Close() //nolint:errcheck
	select {
	case err := <-errorEvent:
		if err.Error() != "NOPERM subscription denied" {
			t.Fatalf("error event = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("initial subscription error was not emitted")
	}

	server.Server().SetPreHook(nil)
	deadline := time.Now().Add(3 * time.Second)
	for server.PubSubNumSub(channel)[channel] != 1 {
		if time.Now().After(deadline) {
			t.Fatal("subscription did not recover")
		}
		time.Sleep(time.Millisecond)
	}
	if err := client.Publish(client.Context(), channel, []byte("recovered")); err != nil {
		t.Fatal(err)
	}
	receiveCtx, cancel := context.WithTimeout(client.Context(), time.Second)
	defer cancel()
	message, err := pubSub.ReceiveMessage(receiveCtx)
	if err != nil || message.Message != "recovered" {
		t.Fatalf("recovered subscription = %+v, %v", message, err)
	}
}

func TestValkeyPubSubWithNoTopicsIsInert(t *testing.T) {
	server, client := newMiniValkeyClient(t)
	var commands atomic.Int32
	server.Server().SetPreHook(func(_ *miniredisserver.Peer, command string, _ ...string) bool {
		if command == "SUBSCRIBE" || command == "UNSUBSCRIBE" {
			commands.Add(1)
		}
		return false
	})
	t.Cleanup(func() { server.Server().SetPreHook(nil) })

	pubSub := client.Subscribe(client.Context())
	if _, err := pubSub.ReceiveMessage(context.Background()); !errors.Is(err, valkey.ErrValkeyPubSubClosed) {
		t.Fatalf("ReceiveMessage error = %v", err)
	}
	if err := pubSub.Close(); err != nil {
		t.Fatal(err)
	}
	if got := commands.Load(); got != 0 {
		t.Fatalf("Pub/Sub commands = %d, want 0", got)
	}
}

func TestValkeyClientContextClosesExplicitContextSubscription(t *testing.T) {
	server := miniredis.RunT(t)
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	client, err := valkey.NewValkeyClient(ownerCtx, newRawClient(t, server.Addr()))
	if err != nil {
		t.Fatal(err)
	}
	channel := "sio:test:owner-context"
	pubSub := client.Subscribe(context.Background(), channel)
	cancelOwner()

	receiveDone := make(chan error, 1)
	go func() {
		_, receiveErr := pubSub.ReceiveMessage(context.Background())
		receiveDone <- receiveErr
	}()
	select {
	case receiveErr := <-receiveDone:
		if !errors.Is(receiveErr, valkey.ErrValkeyPubSubClosed) {
			t.Fatalf("ReceiveMessage error = %v", receiveErr)
		}
	case <-time.After(time.Second):
		t.Fatal("owner context did not close the subscription")
	}
	if err := pubSub.Close(); err != nil {
		t.Fatal(err)
	}
	if got := server.PubSubNumSub(channel)[channel]; got != 0 {
		t.Fatalf("subscriber count after owner cancellation = %d, want 0", got)
	}
}

func TestValkeyPubSubDoesNotConsumeBlockingPool(t *testing.T) {
	server := miniredis.RunT(t)
	raw, err := vk.NewClient(vk.ClientOption{
		InitAddress:      []string{server.Addr()},
		DisableCache:     true,
		AlwaysRESP2:      true,
		BlockingPoolSize: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(raw.Close)
	client, err := valkey.NewValkeyClient(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}

	first := client.Subscribe(client.Context(), "sio:test:pool:1")
	defer first.Close() //nolint:errcheck
	second := client.Subscribe(client.Context(), "sio:test:pool:2")
	defer second.Close() //nolint:errcheck

	ctx, cancel := context.WithTimeout(client.Context(), time.Second)
	defer cancel()
	entries, err := client.XRead(ctx, "sio:test:pool:stream", "0-0", 1, 10*time.Millisecond)
	if err != nil || len(entries) != 0 {
		t.Fatalf("XRead with active subscriptions = %+v, %v", entries, err)
	}
}

func TestSlowValkeyPubSubConsumerDoesNotBlockOtherSubscriptions(t *testing.T) {
	_, client := newMiniValkeyClient(t)
	ctx := client.Context()
	slow := client.Subscribe(ctx, "sio:test:slow")
	defer slow.Close() //nolint:errcheck
	fast := client.Subscribe(ctx, "sio:test:fast")
	defer fast.Close() //nolint:errcheck

	for i := range 100 {
		if err := client.Publish(ctx, "sio:test:slow", []byte(strconv.Itoa(i))); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.Publish(ctx, "sio:test:fast", []byte("delivered")); err != nil {
		t.Fatal(err)
	}
	receiveCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	message, err := fast.ReceiveMessage(receiveCtx)
	if err != nil || message.Message != "delivered" {
		t.Fatalf("fast subscription = %+v, %v", message, err)
	}
}

func TestValkeyPubSubReconnects(t *testing.T) {
	server, client := newMiniValkeyClient(t)
	ctx, cancel := context.WithTimeout(client.Context(), 5*time.Second)
	defer cancel()
	pubSub := client.Subscribe(ctx, "sio:test:reconnect")
	defer pubSub.Close() //nolint:errcheck

	server.Close()
	if err := server.Restart(); err != nil {
		t.Fatalf("restart miniredis: %v", err)
	}

	received := make(chan string, 1)
	go func() {
		message, err := pubSub.ReceiveMessage(ctx)
		if err == nil {
			received <- message.Message
		}
	}()

	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case payload := <-received:
			if payload != "after-reconnect" {
				t.Fatalf("unexpected payload %q", payload)
			}
			return
		case <-ticker.C:
			_ = client.Publish(ctx, "sio:test:reconnect", []byte("after-reconnect"))
		case <-ctx.Done():
			t.Fatal("subscription did not recover")
		}
	}
}

func TestValkeyClientUsesSeparateRoles(t *testing.T) {
	primaryServer := miniredis.RunT(t)
	subServer := miniredis.RunT(t)
	primary := newRawClient(t, primaryServer.Addr())
	subscriber := newRawClient(t, subServer.Addr())
	client, err := valkey.NewValkeyClientWithSub(context.Background(), primary, subscriber)
	if err != nil {
		t.Fatal(err)
	}
	ctx := client.Context()

	pubSub := client.Subscribe(ctx, "sio:test:roles")
	defer pubSub.Close() //nolint:errcheck
	if err = subscriber.Do(ctx,
		subscriber.B().Publish().Channel("sio:test:roles").Message("from-sub").Build(),
	).Error(); err != nil {
		t.Fatal(err)
	}
	message, err := pubSub.ReceiveMessage(ctx)
	if err != nil || message.Message != "from-sub" {
		t.Fatalf("subscription did not use Sub(): %+v, %v", message, err)
	}
	counts, err := client.PubSubNumSub(ctx, "sio:test:roles")
	if err != nil || counts["sio:test:roles"] != 1 {
		t.Fatalf("subscriber count = %v, %v", counts, err)
	}

	if err = client.Set(ctx, "sio:test:session", "value", time.Second); err != nil {
		t.Fatal(err)
	}
	value, err := client.GetDel(ctx, "sio:test:session")
	if err != nil || value != "value" {
		t.Fatalf("GetDel did not use primary: %q, %v", value, err)
	}

	if _, err = client.XAdd(ctx, "sio:test:primary-stream", valkey.RawClusterMessage{
		"uid": "1", "nsp": "/", "type": "2",
	}, 100); err != nil {
		t.Fatal(err)
	}
	entries, err := client.XRangeN(ctx, "sio:test:primary-stream", "-", "+", 1)
	if err != nil || len(entries) != 1 {
		t.Fatalf("XRangeN did not use primary: %d, %v", len(entries), err)
	}

	if err = subscriber.Do(ctx,
		subscriber.B().Xadd().Key("sio:test:sub-stream").Id("*").FieldValue().FieldValue("uid", "2").Build(),
	).Error(); err != nil {
		t.Fatal(err)
	}
	entries, err = client.XRead(ctx, "sio:test:sub-stream", "0-0", 10, time.Millisecond)
	if err != nil || len(entries) != 1 {
		t.Fatalf("XRead did not use Sub(): %d, %v", len(entries), err)
	}
}

func TestValkeyClientStreamCommands(t *testing.T) {
	server, client := newMiniValkeyClient(t)
	ctx := client.Context()
	stream := "sio:test:stream"

	first, err := client.XAdd(ctx, stream, valkey.RawClusterMessage{
		"uid": "first", "nsp": "/", "type": "2",
	}, 100)
	if err != nil || first == "" {
		t.Fatalf("XAdd = %q, %v", first, err)
	}
	second, err := client.XAdd(ctx, stream, valkey.RawClusterMessage{
		"uid": "second", "nsp": "/", "type": "3", "data": `{"value":"second"}`,
	}, 100)
	if err != nil || second == "" {
		t.Fatalf("XAdd = %q, %v", second, err)
	}
	entries, err := client.XRangeN(ctx, stream, "-", "+", 1)
	if err != nil || len(entries) != 1 || entries[0].ID != first {
		t.Fatalf("XRangeN = %+v, %v", entries, err)
	}
	entries, err = client.XRevRangeN(ctx, stream, "+", "-", 1)
	if err != nil || len(entries) != 1 || entries[0].ID != second {
		t.Fatalf("XRevRangeN = %+v, %v", entries, err)
	}

	if err := client.Set(ctx, "sio:test:px", "value", 1500*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if ttl := server.TTL("sio:test:px"); ttl != 1500*time.Millisecond {
		t.Fatalf("TTL = %s, want 1.5s", ttl)
	}
}
