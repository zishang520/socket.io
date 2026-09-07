package adapter

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	miniredisserver "github.com/alicebob/miniredis/v2/server"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func newValkeyAdapterTestClient(t *testing.T, address string) *valkey.ValkeyClient {
	t.Helper()
	rawClient := newValkeyRawClient(t, address)
	client, err := valkey.NewValkeyClient(context.Background(), rawClient)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func newClassicPubSubTestClient(t *testing.T) (*valkey.ValkeyClient, *classicValkeyPubSub) {
	t.Helper()
	server := miniredis.RunT(t)
	client := newValkeyAdapterTestClient(t, server.Addr())
	pubSub := newClassicValkeyPubSub(client.Context(), client.Sub(), func(err error) {
		t.Errorf("unexpected Pub/Sub error: %v", err)
	})
	t.Cleanup(pubSub.Close)
	return client, pubSub
}

func TestClassicValkeyPubSubPreservesChannelAndPatternOrder(t *testing.T) {
	client, pubSub := newClassicPubSubTestClient(t)
	started := make(chan struct{})
	unblock := make(chan struct{})
	var unblockOnce sync.Once
	release := func() { unblockOnce.Do(func() { close(unblock) }) }
	t.Cleanup(release)

	order := make(chan string, 2)
	requests := pubSub.newSubscription(func([]byte, string) {
		close(started)
		<-unblock
		order <- "request"
	})
	broadcasts := pubSub.newSubscription(func([]byte, string) {
		order <- "broadcast"
	})
	requests.Subscribe("request")
	broadcasts.PSubscribe("broadcast:*")
	if err := pubSub.flush(t.Context()); err != nil {
		t.Fatal(err)
	}

	if err := client.Publish(t.Context(), "request", []byte("join")); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("request handler did not start")
	}
	if err := client.Publish(t.Context(), "broadcast:room", []byte("event")); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-order:
		t.Fatalf("message %q overtook the blocked request", got)
	case <-time.After(50 * time.Millisecond):
	}

	release()
	for _, want := range []string{"request", "broadcast"} {
		select {
		case got := <-order:
			if got != want {
				t.Fatalf("delivery order = %q before %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("missing %s delivery", want)
		}
	}
}

func TestValkeyNamespacesShareClassicPubSub(t *testing.T) {
	server := miniredis.RunT(t)
	client := newValkeyAdapterTestClient(t, server.Addr())
	io := socket.NewServer(nil, nil)
	builder := &ValkeyAdapterBuilder{Valkey: client}
	first := builder.New(socket.NewNamespace(io, "/first")).(*valkeyAdapter)
	second := builder.New(socket.NewNamespace(io, "/second")).(*valkeyAdapter)
	if first.pubSub != second.pubSub {
		t.Fatal("namespaces on the same server did not share classic Pub/Sub")
	}

	shared := first.pubSub
	first.Close()
	if err := shared.ctx.Err(); err != nil {
		t.Fatal("shared Pub/Sub closed before its last adapter")
	}
	second.Close()
	if err := shared.ctx.Err(); err == nil {
		t.Fatal("shared Pub/Sub remained open after its last adapter")
	}
}

func TestClassicValkeyPubSubCloseRemovesSubscriptions(t *testing.T) {
	client, pubSub := newClassicPubSubTestClient(t)
	pubSub.newSubscription(func([]byte, string) {}).Subscribe("close:exact")
	pubSub.newSubscription(func([]byte, string) {}).PSubscribe("close:*")
	if err := pubSub.flush(t.Context()); err != nil {
		t.Fatal(err)
	}

	counts, err := client.PubSubNumSub(t.Context(), "close:exact")
	if err != nil {
		t.Fatal(err)
	}
	patterns, err := client.Client().Do(t.Context(), client.Client().B().PubsubNumpat().Build()).AsInt64()
	if err != nil {
		t.Fatal(err)
	}
	if counts["close:exact"] != 1 || patterns != 1 {
		t.Fatalf("active subscriptions = %d exact, %d pattern", counts["close:exact"], patterns)
	}

	pubSub.Close()
	deadline := time.Now().Add(time.Second)
	for {
		counts, err = client.PubSubNumSub(t.Context(), "close:exact")
		if err == nil {
			patterns, err = client.Client().Do(t.Context(), client.Client().B().PubsubNumpat().Build()).AsInt64()
		}
		if err == nil && counts["close:exact"] == 0 && patterns == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("subscriptions after Close = %d exact, %d pattern (error: %v)", counts["close:exact"], patterns, err)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestClassicValkeyPubSubWaitsForPatternBeforeChannel(t *testing.T) {
	server := miniredis.RunT(t)
	server.Server().SetPreHook(func(peer *miniredisserver.Peer, command string, _ ...string) bool {
		if command != "PSUBSCRIBE" {
			return false
		}
		peer.WriteError("NOPERM pattern subscription denied")
		return true
	})
	t.Cleanup(func() { server.Server().SetPreHook(nil) })

	rawClient := newValkeyRawClient(t, server.Addr())
	client, err := valkey.NewValkeyClient(context.Background(), rawClient)
	if err != nil {
		t.Fatal(err)
	}
	errors := make(chan error, 1)
	pubSub := newClassicValkeyPubSub(client.Context(), client.Sub(), func(err error) { errors <- err })
	t.Cleanup(pubSub.Close)

	pubSub.newSubscription(func([]byte, string) {}).PSubscribe("ready:*")
	pubSub.newSubscription(func([]byte, string) {}).Subscribe("ready:request")
	if err = pubSub.flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-errors:
	case <-time.After(time.Second):
		t.Fatal("pattern subscription failure was not reported")
	}
	counts, err := client.PubSubNumSub(t.Context(), "ready:request")
	if err != nil {
		t.Fatal(err)
	}
	if counts["ready:request"] != 0 {
		t.Fatal("request channel was exposed before the broadcast pattern")
	}

	server.Server().SetPreHook(nil)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		counts, countErr := client.PubSubNumSub(t.Context(), "ready:request")
		patterns, patternErr := client.Client().Do(t.Context(), client.Client().B().PubsubNumpat().Build()).AsInt64()
		if countErr == nil && patternErr == nil && counts["ready:request"] == 1 && patterns == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("normal subscription did not follow the restored pattern subscription")
}

func TestClassicValkeyPubSubRetriesChannelSubscribeFailure(t *testing.T) {
	server := miniredis.RunT(t)
	var denyChannel atomic.Bool
	denyChannel.Store(true)
	server.Server().SetPreHook(func(peer *miniredisserver.Peer, command string, _ ...string) bool {
		if command == "SUBSCRIBE" && denyChannel.Load() {
			peer.WriteError("NOPERM channel subscription denied")
			return true
		}
		return false
	})

	client := newValkeyAdapterTestClient(t, server.Addr())
	errors := make(chan error, 4)
	pubSub := newClassicValkeyPubSub(client.Context(), client.Sub(), func(err error) {
		select {
		case errors <- err:
		default:
		}
	})
	t.Cleanup(pubSub.Close)

	received := make(chan string, 2)
	pubSub.newSubscription(func([]byte, string) { received <- "pattern" }).PSubscribe("half:*")
	pubSub.newSubscription(func([]byte, string) { received <- "channel" }).Subscribe("half:request")
	if err := pubSub.flush(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-errors:
	case <-time.After(time.Second):
		t.Fatal("channel subscription failure was not reported")
	}

	denyChannel.Store(false)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		counts, countErr := client.PubSubNumSub(t.Context(), "half:request")
		patterns, patternErr := client.Client().Do(t.Context(), client.Client().B().PubsubNumpat().Build()).AsInt64()
		if countErr == nil && patternErr == nil && counts["half:request"] == 1 && patterns == 1 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	counts, countErr := client.PubSubNumSub(t.Context(), "half:request")
	patterns, patternErr := client.Client().Do(t.Context(), client.Client().B().PubsubNumpat().Build()).AsInt64()
	if countErr != nil || patternErr != nil || counts["half:request"] != 1 || patterns != 1 {
		t.Fatalf("channel subscription was not added: channel=%d pattern=%d (errors: %v, %v)", counts["half:request"], patterns, countErr, patternErr)
	}

	if err := client.Publish(t.Context(), "half:request", []byte("payload")); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool, 2)
	for len(seen) != 2 {
		select {
		case delivery := <-received:
			seen[delivery] = true
		case <-time.After(time.Second):
			t.Fatalf("deliveries = %v", seen)
		}
	}
}

func TestClassicValkeyPubSubRestoresSubscriptions(t *testing.T) {
	server := miniredis.RunT(t)
	address := server.Addr()
	rawClient := newValkeyRawClient(t, address)
	client, err := valkey.NewValkeyClient(context.Background(), rawClient)
	if err != nil {
		t.Fatal(err)
	}
	errors := make(chan error, 4)
	pubSub := newClassicValkeyPubSub(client.Context(), client.Sub(), func(err error) {
		select {
		case errors <- err:
		default:
		}
	})
	t.Cleanup(pubSub.Close)

	received := make(chan string, 2)
	exact := pubSub.newSubscription(func([]byte, string) { received <- "exact" })
	pattern := pubSub.newSubscription(func([]byte, string) { received <- "pattern" })
	exact.Subscribe("recover:exact")
	pattern.PSubscribe("recover:*")
	if err := pubSub.flush(t.Context()); err != nil {
		t.Fatal(err)
	}

	server.Close()
	select {
	case <-errors:
	case <-time.After(3 * time.Second):
		t.Fatal("Pub/Sub disconnect was not observed")
	}
	if err := server.Restart(); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	restored := false
	for time.Now().Before(deadline) {
		counts, countErr := client.PubSubNumSub(t.Context(), "recover:exact")
		patterns, patternErr := client.Client().Do(t.Context(), client.Client().B().PubsubNumpat().Build()).AsInt64()
		if countErr == nil && patternErr == nil && counts["recover:exact"] == 1 && patterns == 1 {
			restored = true
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !restored {
		t.Fatal("normal and pattern subscriptions were not restored")
	}
	deadline = time.Now().Add(time.Second)
	for {
		if err := client.Publish(t.Context(), "recover:exact", []byte("payload")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("publisher did not reconnect")
		}
		time.Sleep(time.Millisecond)
	}
	seen := make(map[string]bool, 2)
	for len(seen) != 2 {
		select {
		case delivery := <-received:
			seen[delivery] = true
		case <-time.After(time.Second):
			t.Fatalf("restored deliveries = %v", seen)
		}
	}
	if !seen["exact"] || !seen["pattern"] {
		t.Fatalf("restored deliveries = %v", seen)
	}
}
