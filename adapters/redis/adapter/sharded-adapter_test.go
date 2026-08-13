package adapter

import (
	"context"
	"fmt"
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
)

type shardedPubSubRecorder struct {
	mu           sync.Mutex
	subscribers  map[string]*miniredisserver.Peer
	single       int
	batched      int
	disconnectAt int
	disconnected bool
	rejectUnsub  bool
}

func (r *shardedPubSubRecorder) subscribe(channels []string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(channels) != 1 {
		r.batched++
		return false
	}

	r.single++
	if r.single == r.disconnectAt && !r.disconnected {
		r.disconnected = true
		return true
	}
	return false
}

func (r *shardedPubSubRecorder) activate(channel string, peer *miniredisserver.Peer) {
	r.mu.Lock()
	r.subscribers[channel] = peer
	r.mu.Unlock()
}

func (r *shardedPubSubRecorder) counts() (single, batched int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.single, r.batched
}

func (r *shardedPubSubRecorder) ready(channels ...string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, channel := range channels {
		peer := r.subscribers[channel]
		if peer == nil || peer.Closed() {
			return false
		}
	}
	return true
}

func (r *shardedPubSubRecorder) rejectNextUnsubscribe() {
	r.mu.Lock()
	r.rejectUnsub = true
	r.mu.Unlock()
}

func (r *shardedPubSubRecorder) unsubscribe() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.rejectUnsub {
		return false
	}
	r.rejectUnsub = false
	return true
}

func (r *shardedPubSubRecorder) deactivate(channel string, peer *miniredisserver.Peer) {
	r.mu.Lock()
	if r.subscribers[channel] == peer {
		delete(r.subscribers, channel)
	}
	r.mu.Unlock()
}

func (r *shardedPubSubRecorder) publish(channel string, payload []byte) bool {
	r.mu.Lock()
	peer := r.subscribers[channel]
	r.mu.Unlock()
	if peer == nil || peer.Closed() {
		return false
	}

	peer.Block(func(writer *miniredisserver.Writer) {
		writer.WritePushLen(3)
		writer.WriteBulk("smessage")
		writer.WriteBulk(channel)
		writer.WriteBulk(string(payload))
	})
	peer.Flush()
	return true
}

func newShardedTestAdapter(t *testing.T, disconnectAt int) (*shardedRedisAdapter, *shardedPubSubRecorder) {
	t.Helper()

	server := miniredis.RunT(t)
	recorder := &shardedPubSubRecorder{
		subscribers:  make(map[string]*miniredisserver.Peer),
		disconnectAt: disconnectAt,
	}
	if err := server.Server().Register("SSUBSCRIBE", func(peer *miniredisserver.Peer, _ string, channels []string) {
		disconnect := recorder.subscribe(channels)
		if len(channels) != 1 {
			peer.WriteError("CROSSSLOT Keys in request don't hash to the same slot")
			return
		}

		peer.Block(func(writer *miniredisserver.Writer) {
			writer.WritePushLen(3)
			writer.WriteBulk("ssubscribe")
			writer.WriteBulk(channels[0])
			writer.WriteInt(1)
		})
		peer.Flush()
		recorder.activate(channels[0], peer)
		if disconnect {
			peer.Close()
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := server.Server().Register("SUNSUBSCRIBE", func(peer *miniredisserver.Peer, _ string, channels []string) {
		if recorder.unsubscribe() {
			peer.WriteError("CROSSSLOT Keys in request don't hash to the same slot")
			return
		}
		for _, channel := range channels {
			peer.Block(func(writer *miniredisserver.Writer) {
				writer.WritePushLen(3)
				writer.WriteBulk("sunsubscribe")
				writer.WriteBulk(channel)
				writer.WriteInt(0)
			})
			recorder.deactivate(channel, peer)
		}
	}); err != nil {
		t.Fatal(err)
	}

	cluster := rds.NewClusterClient(&rds.ClusterOptions{Addrs: []string{server.Addr()}})
	ctx, cancel := context.WithCancel(context.Background())
	shardedAdapter := MakeShardedRedisAdapter().(*shardedRedisAdapter)
	shardedAdapter.redisClient = redis.NewRedisClient(ctx, cluster)
	shardedAdapter.ctx = ctx
	shardedAdapter.cancel = cancel
	shardedAdapter.ClusterAdapter.Construct(socket.NewNamespace(socket.NewServer(nil, nil), "/test"))
	t.Cleanup(func() {
		shardedAdapter.Close()
		_ = cluster.Close()
	})
	return shardedAdapter, recorder
}

func shardedServerSideEmitPayload(t *testing.T, event, value string) []byte {
	t.Helper()
	payload, err := redis.EncodeClusterMessage(&clusteradapter.ClusterMessage{
		Uid:  "remote",
		Nsp:  "/test",
		Type: clusteradapter.SERVER_SIDE_EMIT,
		Data: &clusteradapter.ServerSideEmitMessage{Packet: []any{event, value}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func dynamicSubscriptionState(s *shardedRedisAdapter) (nodes, channels, references int) {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	for _, entry := range s.nodePubSubs {
		references += entry.refCount
	}
	return len(s.nodePubSubs), len(s.chanToAddr), references
}

func waitForShardedState(t *testing.T, check func() bool) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for !check() {
		select {
		case <-ticker.C:
		case <-timer.C:
			t.Fatal("timed out waiting for sharded subscription state")
		}
	}
}

func TestShardedDynamicSubscriptionsRecoverAfterCrossSlotReconnect(t *testing.T) {
	shardedAdapter, recorder := newShardedTestAdapter(t, 2)
	channels := []string{"socket.io#/#room{one}#", "socket.io#/#room{two}#"}
	var errorCount atomic.Int64
	var emittedError atomic.Value
	if err := shardedAdapter.redisClient.On("error", func(args ...any) {
		errorCount.Add(1)
		if len(args) != 0 {
			emittedError.Store(args[0])
		}
	}); err != nil {
		t.Fatal(err)
	}

	received := make(chan any, len(channels)*2)
	if err := shardedAdapter.Nsp().On("probe", func(args ...any) {
		received <- args[0]
	}); err != nil {
		t.Fatal(err)
	}

	for _, channel := range channels {
		shardedAdapter.subscribeNode(channel)
	}

	waitForShardedState(t, func() bool {
		shardedAdapter.dynamicMu.Lock()
		recovered := !shardedAdapter.recovering && len(shardedAdapter.nodePubSubs) == 1 && len(shardedAdapter.chanToAddr) == 2
		shardedAdapter.dynamicMu.Unlock()
		single, batched := recorder.counts()
		return recovered && single >= 4 && batched >= 1 && recorder.ready(channels...)
	})

	for i, channel := range channels {
		value := fmt.Sprintf("value-%d", i)
		payload := shardedServerSideEmitPayload(t, "probe", value)
		if !recorder.publish(channel, payload) {
			t.Fatalf("recovered subscription %q is unavailable", channel)
		}
	}

	values := make(map[any]struct{}, len(channels))
	for range channels {
		select {
		case value := <-received:
			values[value] = struct{}{}
		case <-time.After(time.Second):
			t.Fatal("recovered subscription did not receive a message")
		}
	}
	if len(values) != len(channels) {
		t.Fatalf("received values = %#v, want one value from each channel", values)
	}
	select {
	case value := <-received:
		t.Fatalf("received duplicate value %#v", value)
	case <-time.After(20 * time.Millisecond):
	}
	if count := errorCount.Load(); count != 0 {
		t.Fatalf("recoverable connection error was emitted %d time(s): %v", count, emittedError.Load())
	}
}

func TestShardedDynamicSubscriptionsRecoverAfterUnsubscribeError(t *testing.T) {
	shardedAdapter, recorder := newShardedTestAdapter(t, 0)
	channels := []string{"socket.io#/#room{one}#", "socket.io#/#room{two}#"}
	for _, channel := range channels {
		shardedAdapter.subscribeNode(channel)
	}

	var errorCount atomic.Int64
	if err := shardedAdapter.redisClient.On("error", func(...any) {
		errorCount.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	received := make(chan any, 1)
	if err := shardedAdapter.Nsp().On("unsubscribe-probe", func(args ...any) {
		received <- args[0]
	}); err != nil {
		t.Fatal(err)
	}

	recorder.rejectNextUnsubscribe()
	shardedAdapter.unsubscribeNode(channels[0])
	waitForShardedState(t, func() bool {
		nodes, channelCount, references := dynamicSubscriptionState(shardedAdapter)
		single, _ := recorder.counts()
		return nodes == 1 && channelCount == 1 && references == 1 && single >= 3 && recorder.ready(channels[1])
	})

	if recorder.publish(channels[0], shardedServerSideEmitPayload(t, "unsubscribe-probe", "removed")) {
		t.Fatal("removed channel remained subscribed")
	}
	if !recorder.publish(channels[1], shardedServerSideEmitPayload(t, "unsubscribe-probe", "remaining")) {
		t.Fatal("remaining channel was not restored")
	}
	select {
	case value := <-received:
		if value != "remaining" {
			t.Fatalf("received value = %#v, want remaining", value)
		}
	case <-time.After(time.Second):
		t.Fatal("remaining channel did not receive a message")
	}
	if count := errorCount.Load(); count != 0 {
		t.Fatalf("recoverable unsubscribe error was emitted %d time(s)", count)
	}
}

func TestShardedDynamicSubscriptionsReuseAndReleaseNode(t *testing.T) {
	shardedAdapter, _ := newShardedTestAdapter(t, 0)
	channels := []string{"socket.io#/#room{one}#", "socket.io#/#room{two}#"}
	for _, channel := range channels {
		shardedAdapter.subscribeNode(channel)
	}

	if nodes, channelCount, references := dynamicSubscriptionState(shardedAdapter); nodes != 1 || channelCount != 2 || references != 2 {
		t.Fatalf("subscription state = (%d nodes, %d channels, %d references), want (1, 2, 2)", nodes, channelCount, references)
	}

	shardedAdapter.unsubscribeNode(channels[0])
	if nodes, channelCount, references := dynamicSubscriptionState(shardedAdapter); nodes != 1 || channelCount != 1 || references != 1 {
		t.Fatalf("subscription state = (%d nodes, %d channels, %d references), want (1, 1, 1)", nodes, channelCount, references)
	}

	shardedAdapter.unsubscribeNode(channels[1])
	if nodes, channelCount, references := dynamicSubscriptionState(shardedAdapter); nodes != 0 || channelCount != 0 || references != 0 {
		t.Fatalf("subscription state = (%d nodes, %d channels, %d references), want empty", nodes, channelCount, references)
	}
}

func TestShardedDynamicSubscribeFailureLeavesNoState(t *testing.T) {
	ring := rds.NewRing(&rds.RingOptions{
		Addrs:              map[string]string{"node": "127.0.0.1:0"},
		MaxRetries:         -1,
		DialTimeout:        time.Millisecond,
		DialerRetries:      1,
		DialerRetryTimeout: time.Millisecond,
	})
	t.Cleanup(func() { _ = ring.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancel)

	shardedAdapter := MakeShardedRedisAdapter().(*shardedRedisAdapter)
	shardedAdapter.redisClient = redis.NewRedisClient(ctx, ring)
	shardedAdapter.ctx = ctx
	shardedAdapter.cancel = cancel
	var errorCount atomic.Int64
	if err := shardedAdapter.redisClient.On("error", func(...any) {
		errorCount.Add(1)
	}); err != nil {
		t.Fatal(err)
	}

	shardedAdapter.subscribeNode("socket.io#/#room#")
	shardedAdapter.dynamicMu.Lock()
	nodes := len(shardedAdapter.nodePubSubs)
	channels := len(shardedAdapter.chanToAddr)
	desired := len(shardedAdapter.dynamicChannels)
	shardedAdapter.dynamicMu.Unlock()
	if nodes != 0 || channels != 0 || desired != 0 {
		t.Fatalf("failed subscription retained state: nodes=%d channels=%d desired=%d", nodes, channels, desired)
	}
	if count := errorCount.Load(); count != 1 {
		t.Fatalf("error count = %d, want 1", count)
	}
}

func TestShardedDynamicSubscriptionIsIdempotent(t *testing.T) {
	shardedAdapter, _ := newShardedTestAdapter(t, 0)
	const channel = "socket.io#/#room{one}#"

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			shardedAdapter.subscribeNode(channel)
		})
	}
	wg.Wait()

	if nodes, channels, references := dynamicSubscriptionState(shardedAdapter); nodes != 1 || channels != 1 || references != 1 {
		t.Fatalf("subscription state = (%d nodes, %d channels, %d references), want (1, 1, 1)", nodes, channels, references)
	}
}

func TestShardedClosePreventsNewDynamicSubscriptions(t *testing.T) {
	shardedAdapter, _ := newShardedTestAdapter(t, 0)
	shardedAdapter.subscribeNode("socket.io#/#room{one}#")
	shardedAdapter.Close()
	shardedAdapter.subscribeNode("socket.io#/#room{two}#")
	shardedAdapter.Close()

	if nodes, channels, references := dynamicSubscriptionState(shardedAdapter); nodes != 0 || channels != 0 || references != 0 {
		t.Fatal("Close allowed a new dynamic subscription")
	}
}

func TestShardedRedisAdapterComputeChannel(t *testing.T) {
	const channel = "socket.io#/#"

	tests := []struct {
		name      string
		mode      redis.SubscriptionMode
		rooms     []socket.Room
		requestId *string
		want      string
	}{
		{name: "static", mode: redis.StaticSubscriptionMode, rooms: []socket.Room{"room"}, want: channel},
		{name: "dynamic public room", mode: redis.DynamicSubscriptionMode, rooms: []socket.Room{"room"}, want: channel + "room#"},
		{name: "dynamic socket room", mode: redis.DynamicSubscriptionMode, rooms: []socket.Room{"12345678901234567890"}, want: channel},
		{name: "dynamic private socket room", mode: redis.DynamicPrivateSubscriptionMode, rooms: []socket.Room{"12345678901234567890"}, want: channel + "12345678901234567890#"},
		{name: "multiple rooms", mode: redis.DynamicPrivateSubscriptionMode, rooms: []socket.Room{"room1", "room2"}, want: channel},
		{name: "broadcast with ack", mode: redis.DynamicPrivateSubscriptionMode, rooms: []socket.Room{"room"}, requestId: new("request"), want: channel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := MakeShardedRedisAdapter().(*shardedRedisAdapter)
			s.channel = channel
			s.opts.SetSubscriptionMode(tt.mode)

			message := &clusteradapter.ClusterMessage{
				Type: clusteradapter.BROADCAST,
				Data: &clusteradapter.BroadcastMessage{
					Opts:      &clusteradapter.PacketOptions{Rooms: tt.rooms},
					RequestId: tt.requestId,
				},
			}
			if got := s.computeChannel(message); got != tt.want {
				t.Fatalf("computeChannel() = %q, want %q", got, tt.want)
			}
		})
	}
}
