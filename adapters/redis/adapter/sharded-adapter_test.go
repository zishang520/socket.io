package adapter

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	miniredisserver "github.com/alicebob/miniredis/v2/server"
	rds "github.com/redis/go-redis/v9"
	clusteradapter "github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type shardedPubSubRecorder struct {
	mu           sync.Mutex
	subscribers  map[string]map[*miniredisserver.Peer]struct{}
	attempts     map[string]int
	reject       map[string]int
	rejectUnsub  map[string]int
	single       int
	batched      int
	disconnectAt int
	disconnected bool
}

func newShardedPubSubRecorder(t *testing.T, disconnectAt int) (*miniredis.Miniredis, *shardedPubSubRecorder) {
	t.Helper()
	server := miniredis.RunT(t)
	recorder := &shardedPubSubRecorder{
		subscribers:  make(map[string]map[*miniredisserver.Peer]struct{}),
		attempts:     make(map[string]int),
		reject:       make(map[string]int),
		rejectUnsub:  make(map[string]int),
		disconnectAt: disconnectAt,
	}
	if err := server.Server().Register("SSUBSCRIBE", recorder.handleSubscribe); err != nil {
		t.Fatal(err)
	}
	if err := server.Server().Register("SUNSUBSCRIBE", recorder.handleUnsubscribe); err != nil {
		t.Fatal(err)
	}
	return server, recorder
}

func (r *shardedPubSubRecorder) handleSubscribe(peer *miniredisserver.Peer, _ string, channels []string) {
	r.mu.Lock()
	if len(channels) != 1 {
		r.batched++
		r.mu.Unlock()
		peer.WriteError("CROSSSLOT Keys in request don't hash to the same slot")
		return
	}
	channel := channels[0]
	r.single++
	r.attempts[channel]++
	if r.reject[channel] > 0 {
		r.reject[channel]--
		r.mu.Unlock()
		peer.WriteError("TRYAGAIN injected subscription failure")
		return
	}
	disconnect := r.single == r.disconnectAt && !r.disconnected
	if disconnect {
		r.disconnected = true
	}
	peers := r.subscribers[channel]
	if peers == nil {
		peers = make(map[*miniredisserver.Peer]struct{})
		r.subscribers[channel] = peers
	}
	peers[peer] = struct{}{}
	r.mu.Unlock()

	peer.Block(func(writer *miniredisserver.Writer) {
		writer.WritePushLen(3)
		writer.WriteBulk("ssubscribe")
		writer.WriteBulk(channel)
		writer.WriteInt(1)
	})
	peer.Flush()
	if disconnect {
		peer.Close()
	}
}

func (r *shardedPubSubRecorder) handleUnsubscribe(peer *miniredisserver.Peer, _ string, channels []string) {
	for _, channel := range channels {
		r.mu.Lock()
		if r.rejectUnsub[channel] > 0 {
			r.rejectUnsub[channel]--
			r.mu.Unlock()
			peer.WriteError("NOPERM injected unsubscribe failure")
			return
		}
		r.mu.Unlock()
		r.remove(channel, peer)
		peer.Block(func(writer *miniredisserver.Writer) {
			writer.WritePushLen(3)
			writer.WriteBulk("sunsubscribe")
			writer.WriteBulk(channel)
			writer.WriteInt(0)
		})
	}
	peer.Flush()
}

func (r *shardedPubSubRecorder) remove(channel string, peer *miniredisserver.Peer) {
	r.mu.Lock()
	if peers := r.subscribers[channel]; peers != nil {
		delete(peers, peer)
		if len(peers) == 0 {
			delete(r.subscribers, channel)
		}
	}
	r.mu.Unlock()
}

func (r *shardedPubSubRecorder) rejectNext(channel string) {
	r.mu.Lock()
	r.reject[channel]++
	r.mu.Unlock()
}

func (r *shardedPubSubRecorder) rejectNextUnsubscribe(channel string) {
	r.mu.Lock()
	r.rejectUnsub[channel]++
	r.mu.Unlock()
}

func (r *shardedPubSubRecorder) activePeers(channel string) []*miniredisserver.Peer {
	r.mu.Lock()
	defer r.mu.Unlock()
	var active []*miniredisserver.Peer
	for peer := range r.subscribers[channel] {
		if !peer.Closed() {
			active = append(active, peer)
		}
	}
	return active
}

func (r *shardedPubSubRecorder) attemptCount(channel string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts[channel]
}

func (r *shardedPubSubRecorder) counts() (single, batched int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.single, r.batched
}

func (r *shardedPubSubRecorder) publish(channel string, payload []byte) int {
	peers := r.activePeers(channel)
	for _, peer := range peers {
		peer.Block(func(writer *miniredisserver.Writer) {
			writer.WritePushLen(3)
			writer.WriteBulk("smessage")
			writer.WriteBulk(channel)
			writer.WriteBulk(string(payload))
		})
		peer.Flush()
	}
	return len(peers)
}

func (r *shardedPubSubRecorder) pushSunsubscribe(channel string) bool {
	peers := r.activePeers(channel)
	if len(peers) == 0 {
		return false
	}
	peer := peers[0]
	r.remove(channel, peer)
	peer.Block(func(writer *miniredisserver.Writer) {
		writer.WritePushLen(3)
		writer.WriteBulk("sunsubscribe")
		writer.WriteBulk(channel)
		writer.WriteInt(0)
	})
	peer.Flush()
	return true
}

func waitForShardedState(t *testing.T, check func() bool) {
	waitForShardedStateWithin(t, 5*time.Second, check)
}

func waitForShardedStateWithin(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for !check() {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("timed out waiting for sharded Pub/Sub state")
		}
	}
}

func shardedServerSideEmitPayload(t *testing.T, nsp string) []byte {
	t.Helper()
	payload, err := redis.EncodeClusterMessage(&clusteradapter.ClusterMessage{
		Uid:  "remote",
		Nsp:  nsp,
		Type: clusteradapter.SERVER_SIDE_EMIT,
		Data: &clusteradapter.ServerSideEmitMessage{Packet: []any{"probe", "value"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestShardedBuilderSharesSubscriberAcrossNamespaces(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	socketServer := socket.NewServer(nil, nil)
	builder := &ShardedRedisAdapterBuilder{Redis: redisClient}

	const count = 12
	adapters := make([]*shardedRedisAdapter, 0, count)
	for i := range count {
		nsp := socket.NewNamespace(socketServer, fmt.Sprintf("/nsp-%d", i))
		adapters = append(adapters, builder.New(nsp).(*shardedRedisAdapter))
	}
	waitForShardedState(t, func() bool {
		for _, current := range adapters {
			if len(recorder.activePeers(current.channel)) != 1 || len(recorder.activePeers(current.response)) != 1 {
				return false
			}
		}
		return true
	})

	var sharedPeer *miniredisserver.Peer
	for _, current := range adapters {
		for _, channel := range []string{current.channel, current.response} {
			peers := recorder.activePeers(channel)
			if len(peers) != 1 {
				t.Fatalf("channel %q has %d subscribers, want 1", channel, len(peers))
			}
			if sharedPeer == nil {
				sharedPeer = peers[0]
			} else if sharedPeer != peers[0] {
				t.Fatal("namespaces did not reuse the standalone subscriber connection")
			}
		}
	}

	adapters[0].Close()
	if sharedPeer.Closed() {
		t.Fatal("closing one namespace closed the shared subscriber")
	}
	waitForShardedState(t, func() bool {
		return len(recorder.activePeers(adapters[0].channel)) == 0 &&
			len(recorder.activePeers(adapters[1].channel)) == 1
	})
	for _, current := range adapters[1:] {
		current.Close()
	}
	waitForShardedState(t, sharedPeer.Closed)
}

func TestShardedBuilderKeepsSocketServersIndependent(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	builder := &ShardedRedisAdapterBuilder{Redis: redisClient}
	first := builder.New(socket.NewNamespace(socket.NewServer(nil, nil), "/same")).(*shardedRedisAdapter)
	second := builder.New(socket.NewNamespace(socket.NewServer(nil, nil), "/same")).(*shardedRedisAdapter)
	defer second.Close()

	waitForShardedState(t, func() bool { return len(recorder.activePeers(first.channel)) == 2 })
	first.Close()
	waitForShardedState(t, func() bool { return len(recorder.activePeers(second.channel)) == 1 })
}

func TestNewShardedRedisAdapterSharesSubscriber(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	socketServer := socket.NewServer(nil, nil)
	first := NewShardedRedisAdapter(socket.NewNamespace(socketServer, "/first"), redisClient, nil).(*shardedRedisAdapter)
	second := NewShardedRedisAdapter(socket.NewNamespace(socketServer, "/second"), redisClient, nil).(*shardedRedisAdapter)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	waitForShardedState(t, func() bool {
		return len(recorder.activePeers(first.channel)) == 1 &&
			len(recorder.activePeers(first.response)) == 1 &&
			len(recorder.activePeers(second.channel)) == 1 &&
			len(recorder.activePeers(second.response)) == 1
	})

	sharedPeer := recorder.activePeers(first.channel)[0]
	for _, channel := range []string{first.response, second.channel, second.response} {
		if recorder.activePeers(channel)[0] != sharedPeer {
			t.Fatal("direct constructors did not reuse the subscriber connection")
		}
	}

	first.Close()
	if sharedPeer.Closed() {
		t.Fatal("closing one adapter closed the shared subscriber")
	}
	waitForShardedState(t, func() bool {
		return len(recorder.activePeers(first.channel)) == 0 &&
			len(recorder.activePeers(second.channel)) == 1
	})
	second.Close()
	waitForShardedState(t, sharedPeer.Closed)
}

func TestShardedBuildersShareSubscriber(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	socketServer := socket.NewServer(nil, nil)
	first := (&ShardedRedisAdapterBuilder{Redis: redisClient}).New(
		socket.NewNamespace(socketServer, "/first"),
	).(*shardedRedisAdapter)
	second := (&ShardedRedisAdapterBuilder{Redis: redisClient}).New(
		socket.NewNamespace(socketServer, "/second"),
	).(*shardedRedisAdapter)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	waitForShardedState(t, func() bool {
		return len(recorder.activePeers(first.channel)) == 1 &&
			len(recorder.activePeers(second.channel)) == 1
	})
	if recorder.activePeers(first.channel)[0] != recorder.activePeers(second.channel)[0] {
		t.Fatal("builders did not reuse the subscriber connection")
	}
}

func TestShardedAdaptersKeepRedisClientsIndependent(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	firstClient := rds.NewClient(&rds.Options{Addr: server.Addr()})
	secondClient := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = firstClient.Close() })
	t.Cleanup(func() { _ = secondClient.Close() })
	socketServer := socket.NewServer(nil, nil)
	first := NewShardedRedisAdapter(
		socket.NewNamespace(socketServer, "/first"),
		mustRedisClient(t, context.Background(), firstClient),
		nil,
	).(*shardedRedisAdapter)
	second := NewShardedRedisAdapter(
		socket.NewNamespace(socketServer, "/second"),
		mustRedisClient(t, context.Background(), secondClient),
		nil,
	).(*shardedRedisAdapter)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	waitForShardedState(t, func() bool {
		return len(recorder.activePeers(first.channel)) == 1 &&
			len(recorder.activePeers(second.channel)) == 1 &&
			len(recorder.activePeers(first.response)) == 1 &&
			len(recorder.activePeers(second.response)) == 1
	})
	firstPeer := recorder.activePeers(first.response)[0]
	secondPeer := recorder.activePeers(second.response)[0]
	if firstPeer == secondPeer {
		t.Fatal("different Redis clients reused the subscriber connection")
	}

	first.Close()
	waitForShardedState(t, func() bool {
		return firstPeer.Closed() &&
			!secondPeer.Closed() &&
			len(recorder.activePeers(second.channel)) == 1
	})
}

func TestShardedSubscriberRetainsDesiredChannelAfterFailure(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := t.Context()
	pubSub := newShardedPubSub(ctx, client, nil)
	defer pubSub.Close()
	channel := "socket.io#/#retry#"
	recorder.rejectNext(channel)
	subscription := pubSub.newSubscription(func([]byte, string) {})
	subscription.Subscribe(channel)
	if err := pubSub.flush(ctx); err != nil {
		t.Fatal(err)
	}

	waitForShardedState(t, func() bool {
		return recorder.attemptCount(channel) >= 2 && len(recorder.activePeers(channel)) == 1
	})
}

func TestShardedSubscriberFansOutSharedChannel(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	pubSub := newShardedPubSub(context.Background(), client, nil)
	defer pubSub.Close()
	channel := "socket.io#/#shared#"
	received := make(chan string, 3)
	first := pubSub.newSubscription(func([]byte, string) { received <- "first" })
	second := pubSub.newSubscription(func([]byte, string) { received <- "second" })
	first.Subscribe(channel)
	second.Subscribe(channel)
	if err := pubSub.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForShardedState(t, func() bool { return len(recorder.activePeers(channel)) == 1 })

	recorder.publish(channel, []byte("one"))
	seen := map[string]bool{}
	for range 2 {
		select {
		case handler := <-received:
			seen[handler] = true
		case <-time.After(time.Second):
			t.Fatal("shared channel was not fanned out")
		}
	}
	if !seen["first"] || !seen["second"] {
		t.Fatalf("handlers = %#v", seen)
	}

	first.Close()
	recorder.publish(channel, []byte("two"))
	select {
	case handler := <-received:
		if handler != "second" {
			t.Fatalf("closed handler %q received the message", handler)
		}
	case <-time.After(time.Second):
		t.Fatal("remaining handler did not receive the message")
	}
}

func TestShardedSubscriberRecoversAfterUnsubscribeError(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	pubSub := newShardedPubSub(context.Background(), client, nil)
	defer pubSub.Close()
	removed := "socket.io#/#removed#"
	remaining := "socket.io#/#remaining#"
	received := make(chan struct{}, 1)
	subscription := pubSub.newSubscription(func([]byte, string) { received <- struct{}{} })
	subscription.Subscribe(removed)
	subscription.Subscribe(remaining)
	if err := pubSub.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForShardedState(t, func() bool {
		return len(recorder.activePeers(removed)) == 1 && len(recorder.activePeers(remaining)) == 1
	})

	recorder.rejectNextUnsubscribe(removed)
	subscription.Unsubscribe(removed)
	waitForShardedState(t, func() bool {
		return len(recorder.activePeers(removed)) == 0 && len(recorder.activePeers(remaining)) == 1
	})
	if delivered := recorder.publish(remaining, []byte("payload")); delivered != 1 {
		t.Fatalf("remaining channel delivered to %d subscribers", delivered)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("remaining channel did not recover")
	}
}

func TestShardedSubscriptionIsIdempotentAndClosed(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	pubSub := newShardedPubSub(context.Background(), client, nil)
	defer pubSub.Close()
	channel := "socket.io#/#once#"
	subscription := pubSub.newSubscription(func([]byte, string) {})
	subscription.Subscribe(channel)
	subscription.Subscribe(channel)
	if err := pubSub.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	waitForShardedState(t, func() bool { return len(recorder.activePeers(channel)) == 1 })
	if attempts := recorder.attemptCount(channel); attempts != 1 {
		t.Fatalf("SSUBSCRIBE attempts = %d, want 1", attempts)
	}

	subscription.Close()
	waitForShardedState(t, func() bool { return len(recorder.activePeers(channel)) == 0 })
	subscription.Subscribe("socket.io#/#after-close#")
	if err := pubSub.flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	if attempts := recorder.attemptCount("socket.io#/#after-close#"); attempts != 0 {
		t.Fatalf("closed subscription attempted %d new subscriptions", attempts)
	}
}

func TestShardedSubscriptionChangesDoNotWaitForRedis(t *testing.T) {
	dialStarted := make(chan struct{})
	releaseDial := make(chan struct{})
	var started sync.Once
	var dials atomic.Int64
	client := rds.NewClient(&rds.Options{
		Addr:       "unreachable",
		MaxRetries: -1,
		Dialer: func(ctx context.Context, _, _ string) (net.Conn, error) {
			dials.Add(1)
			started.Do(func() { close(dialStarted) })
			select {
			case <-releaseDial:
				return nil, errors.New("injected dial failure")
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	pubSub := newShardedPubSub(ctx, client, nil)
	subscription := pubSub.newSubscription(func([]byte, string) {})
	subscription.Subscribe("first")
	select {
	case <-dialStarted:
	case <-time.After(time.Second):
		t.Fatal("subscription did not start dialing")
	}

	done := make(chan struct{})
	go func() {
		for i := range 64 {
			subscription.Subscribe(fmt.Sprintf("channel-%d", i))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("subscription change waited for Redis I/O")
	}

	close(releaseDial)
	flushCtx, stopFlush := context.WithTimeout(context.Background(), time.Second)
	if err := pubSub.flush(flushCtx); err != nil {
		t.Fatal(err)
	}
	stopFlush()
	if attempts := dials.Load(); attempts != 1 {
		t.Fatalf("Redis outage caused %d dials for one owner", attempts)
	}
	cancel()
	pubSub.Close()
	_ = client.Close()
}

func TestShardedSubscriberRecoversCrossSlotReconnect(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 2)
	cluster := rds.NewClusterClient(&rds.ClusterOptions{Addrs: []string{server.Addr()}})
	t.Cleanup(func() { _ = cluster.Close() })
	ctx := t.Context()
	pubSub := newShardedPubSub(ctx, cluster, nil)
	defer pubSub.Close()
	received := make(chan string, 2)
	subscription := pubSub.newSubscription(func(_ []byte, channel string) { received <- channel })
	channels := []string{"socket.io#/#room{one}#", "socket.io#/#room{two}#"}
	for _, channel := range channels {
		subscription.Subscribe(channel)
	}
	if err := pubSub.flush(ctx); err != nil {
		t.Fatal(err)
	}

	waitForShardedStateWithin(t, 750*time.Millisecond, func() bool {
		single, batched := recorder.counts()
		return single >= 4 && batched >= 1 &&
			len(recorder.activePeers(channels[0])) == 1 && len(recorder.activePeers(channels[1])) == 1
	})
	for _, channel := range channels {
		if delivered := recorder.publish(channel, []byte("payload")); delivered != 1 {
			t.Fatalf("channel %q delivered to %d subscribers", channel, delivered)
		}
	}
	seen := make(map[string]struct{}, len(channels))
	for range channels {
		select {
		case channel := <-received:
			seen[channel] = struct{}{}
		case <-time.After(time.Second):
			t.Fatal("recovered subscription did not receive a message")
		}
	}
	if len(seen) != len(channels) {
		t.Fatalf("received channels = %#v", seen)
	}
}

func TestShardedSubscriberFollowsServerSunsubscribe(t *testing.T) {
	first, firstRecorder := newShardedPubSubRecorder(t, 0)
	second, secondRecorder := newShardedPubSubRecorder(t, 0)
	var moved atomic.Bool
	cluster := rds.NewClusterClient(&rds.ClusterOptions{
		Addrs: []string{first.Addr(), second.Addr()},
		ClusterSlots: func(context.Context) ([]rds.ClusterSlot, error) {
			addr := first.Addr()
			if moved.Load() {
				addr = second.Addr()
			}
			return []rds.ClusterSlot{{Start: 0, End: 16383, Nodes: []rds.ClusterNode{{Addr: addr}}}}, nil
		},
	})
	t.Cleanup(func() { _ = cluster.Close() })
	ctx := t.Context()
	pubSub := newShardedPubSub(ctx, cluster, nil)
	defer pubSub.Close()
	channel := "socket.io#/#moved#"
	received := make(chan struct{}, 1)
	subscription := pubSub.newSubscription(func([]byte, string) { received <- struct{}{} })
	subscription.Subscribe(channel)
	if err := pubSub.flush(ctx); err != nil {
		t.Fatal(err)
	}
	waitForShardedState(t, func() bool { return len(firstRecorder.activePeers(channel)) == 1 })

	moved.Store(true)
	if !firstRecorder.pushSunsubscribe(channel) {
		t.Fatal("initial subscription is unavailable")
	}
	waitForShardedStateWithin(t, 750*time.Millisecond, func() bool {
		return len(firstRecorder.activePeers(channel)) == 0 && len(secondRecorder.activePeers(channel)) == 1
	})
	if delivered := secondRecorder.publish(channel, []byte("payload")); delivered != 1 {
		t.Fatalf("moved channel delivered to %d subscribers", delivered)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("moved subscription did not receive a message")
	}
}

func TestShardedResponseChannelCollisionDispatchesOnce(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	opts := DefaultShardedRedisAdapterOptions()
	opts.SetSubscriptionMode(redis.DynamicPrivateSubscriptionMode)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/collision")
	current := NewShardedRedisAdapter(nsp, redisClient, opts).(*shardedRedisAdapter)
	defer current.Close()

	received := make(chan struct{}, 2)
	if err := nsp.On("probe", func(...any) { received <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	current.AddAll("sid", types.NewSet(socket.Room(current.Uid())))
	if err := current.pubSub.flush(current.ctx); err != nil {
		t.Fatal(err)
	}
	waitForShardedState(t, func() bool { return len(recorder.activePeers(current.response)) == 1 })

	payload := shardedServerSideEmitPayload(t, "/collision")
	if delivered := recorder.publish(current.response, payload); delivered != 1 {
		t.Fatalf("delivered to %d subscribers, want 1", delivered)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("colliding channel message was not dispatched")
	}
	select {
	case <-received:
		t.Fatal("colliding channel message was dispatched twice")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestShardedRedisAdapterComputeChannel(t *testing.T) {
	current := MakeShardedRedisAdapter().(*shardedRedisAdapter)
	current.channel = "socket.io#/#"
	current.opts.SetSubscriptionMode(redis.DynamicSubscriptionMode)
	room := socket.Room("room")

	message := &clusteradapter.ClusterMessage{
		Type: clusteradapter.BROADCAST,
		Data: &clusteradapter.BroadcastMessage{Opts: &clusteradapter.PacketOptions{Rooms: []socket.Room{room}}},
	}
	if got := current.computeChannel(message); got != "socket.io#/#room#" {
		t.Fatalf("dynamic channel = %q", got)
	}

	requestID := "request"
	message.Data.(*clusteradapter.BroadcastMessage).RequestId = &requestID
	if got := current.computeChannel(message); got != current.channel {
		t.Fatalf("acknowledged broadcast channel = %q", got)
	}
}

func BenchmarkShardedPubSubDispatch(b *testing.B) {
	var received atomic.Uint64
	pubSub := &shardedPubSub{routes: map[string]shardedRoute{
		"channel": {first: &shardedSubscription{handler: func([]byte, string) { received.Add(1) }}},
	}}
	payload := []byte("payload")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		pubSub.dispatch("channel", payload)
	}
}
