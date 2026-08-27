package adapter

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type pubSubShardCountHook struct {
	channel string
	count   int64
	calls   atomic.Int64
}

func (h *pubSubShardCountHook) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *pubSubShardCountHook) ProcessHook(next rds.ProcessHook) rds.ProcessHook {
	return func(ctx context.Context, cmd rds.Cmder) error {
		if cmd.Name() != "pubsub" {
			return next(ctx, cmd)
		}
		args := cmd.Args()
		if len(args) != 3 || args[1] != "shardnumsub" || args[2] != h.channel {
			return fmt.Errorf("unexpected PUBSUB arguments: %v", args)
		}
		result, ok := cmd.(*rds.MapStringIntCmd)
		if !ok {
			return fmt.Errorf("unexpected PUBSUB command type %T", cmd)
		}
		h.calls.Add(1)
		result.SetVal(map[string]int64{h.channel: h.count})
		return nil
	}
}

func (h *pubSubShardCountHook) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func mustRedisClient(t *testing.T, ctx context.Context, client rds.UniversalClient) *redis.RedisClient {
	t.Helper()
	redisClient, err := redis.NewRedisClient(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	return redisClient
}

func mustRedisClientWithSub(t *testing.T, ctx context.Context, client, subClient rds.UniversalClient) *redis.RedisClient {
	t.Helper()
	redisClient, err := redis.NewRedisClientWithSub(ctx, client, subClient)
	if err != nil {
		t.Fatal(err)
	}
	return redisClient
}

func waitForRedisPubSub(t *testing.T, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for Redis Pub/Sub state")
}

func currentRedisPubSubTransport(pubSub *redisPubSub) *rds.PubSub {
	pubSub.mu.RLock()
	defer pubSub.mu.RUnlock()
	return pubSub.pubSub
}

func TestRedisPubSubSharesConnectionAndRoutesHandlers(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	pubSub := newRedisPubSub(context.Background(), client, func(err error) {
		t.Errorf("unexpected Pub/Sub error: %v", err)
	})
	t.Cleanup(pubSub.Close)

	var exactCalls, combinedCalls atomic.Int64
	var patternChannel atomic.Value
	exact := pubSub.newSubscription(func([]byte, string) {
		exactCalls.Add(1)
	})
	combined := pubSub.newSubscription(func(_ []byte, channel string) {
		combinedCalls.Add(1)
		patternChannel.Store(channel)
	})
	exact.Subscribe("events:1")
	combined.Subscribe("events:1")
	combined.PSubscribe("events:*")

	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "events:1").Result()
		if err != nil || counts["events:1"] != 1 {
			return false
		}
		patterns, err := client.PubSubNumPat(context.Background()).Result()
		return err == nil && patterns == 1
	})

	if err := client.Publish(context.Background(), "events:1", "payload").Err(); err != nil {
		t.Fatal(err)
	}
	waitForRedisPubSub(t, func() bool {
		return exactCalls.Load() == 1 && combinedCalls.Load() == 2
	})
	if channel, _ := patternChannel.Load().(string); channel != "events:1" {
		t.Fatalf("handler channel = %q, want actual published channel", channel)
	}

	exact.Close()
	counts, err := client.PubSubNumSub(context.Background(), "events:1").Result()
	if err != nil || counts["events:1"] != 1 {
		t.Fatalf("shared channel was unsubscribed: counts=%v err=%v", counts, err)
	}

	combined.Close()
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "events:1").Result()
		if err != nil || counts["events:1"] != 0 {
			return false
		}
		patterns, err := client.PubSubNumPat(context.Background()).Result()
		return err == nil && patterns == 0
	})
}

func TestRedisPubSubRetriesFailedSubscription(t *testing.T) {
	server := miniredis.RunT(t)
	addr := server.Addr()
	server.Close()
	client := rds.NewClient(&rds.Options{
		Addr:         addr,
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() { _ = client.Close() })
	errors := make(chan error, 4)
	pubSub := newRedisPubSub(context.Background(), client, func(err error) {
		select {
		case errors <- err:
		default:
		}
	})
	t.Cleanup(pubSub.Close)
	var calls atomic.Int64
	subscription := pubSub.newSubscription(func([]byte, string) { calls.Add(1) })
	subscription.Subscribe("recover:1")
	subscription.PSubscribe("recover:*")

	select {
	case <-errors:
	case <-time.After(3 * time.Second):
		t.Fatal("initial subscription failure was not observed")
	}
	if err := server.Restart(); err != nil {
		t.Fatal(err)
	}
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "recover:1").Result()
		if err != nil || counts["recover:1"] != 1 {
			return false
		}
		patterns, err := client.PubSubNumPat(context.Background()).Result()
		return err == nil && patterns == 1
	})

	draining := true
	for draining {
		select {
		case <-errors:
		default:
			draining = false
		}
	}
	server.Close()
	subscription.Subscribe("recover:2")
	select {
	case <-errors:
	case <-time.After(3 * time.Second):
		t.Fatal("incremental subscription failure was not observed")
	}
	if err := server.Restart(); err != nil {
		t.Fatal(err)
	}
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "recover:1", "recover:2").Result()
		if err != nil || counts["recover:1"] != 1 || counts["recover:2"] != 1 {
			return false
		}
		patterns, err := client.PubSubNumPat(context.Background()).Result()
		return err == nil && patterns == 1
	})
	if err := client.Publish(context.Background(), "recover:2", "payload").Err(); err != nil {
		t.Fatal(err)
	}
	waitForRedisPubSub(t, func() bool { return calls.Load() >= 2 })
}

func TestRedisPubSubRetriesServerRejectedSubscription(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	errors := make(chan error, 1)
	pubSub := newRedisPubSub(context.Background(), client, func(err error) {
		server.SetError("")
		select {
		case errors <- err:
		default:
		}
	})
	t.Cleanup(pubSub.Close)

	received := make(chan struct{}, 1)
	subscription := pubSub.newSubscription(func([]byte, string) { received <- struct{}{} })
	subscription.Subscribe("warmup")
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "warmup").Result()
		return err == nil && counts["warmup"] == 1
	})

	server.SetError("TRYAGAIN injected subscription failure")
	subscription.Subscribe("recover:server-error")

	select {
	case <-errors:
	case <-time.After(3 * time.Second):
		t.Fatal("server-side subscription failure was not observed")
	}

	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "recover:server-error").Result()
		return err == nil && counts["recover:server-error"] == 1
	})
	if err := client.Publish(context.Background(), "recover:server-error", "payload").Err(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("recovered subscription did not receive a message")
	}
}

func TestRedisPubSubDoesNotRebuildPersistentServerError(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	var errorCount atomic.Int64
	pubSub := newRedisPubSub(context.Background(), client, func(error) {
		errorCount.Add(1)
	})
	t.Cleanup(pubSub.Close)

	received := make(chan struct{}, 1)
	subscription := pubSub.newSubscription(func([]byte, string) { received <- struct{}{} })
	subscription.Subscribe("warmup")
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "warmup").Result()
		return err == nil && counts["warmup"] == 1
	})
	initial := currentRedisPubSubTransport(pubSub)

	server.SetError("NOPERM persistent subscription failure")
	subscription.Subscribe("recover:persistent-error")
	waitForRedisPubSub(t, func() bool { return errorCount.Load() >= 1 })
	waitForRedisPubSub(t, func() bool {
		return currentRedisPubSubTransport(pubSub) != initial
	})
	replacement := currentRedisPubSubTransport(pubSub)

	// The retry loop keeps reasserting the desired subscription, but a persistent
	// Redis error must not create a new connection on every retry.
	waitForRedisPubSub(t, func() bool { return errorCount.Load() >= 3 })
	if current := currentRedisPubSubTransport(pubSub); current != replacement {
		t.Fatal("persistent server error repeatedly rebuilt the Pub/Sub connection")
	}
	server.SetError("")
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "recover:persistent-error").Result()
		return err == nil && counts["recover:persistent-error"] == 1
	})
	if err := client.Publish(context.Background(), "recover:persistent-error", "payload").Err(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(3 * time.Second):
		t.Fatal("subscription did not recover after the persistent error cleared")
	}
}

func TestRedisPubSubDropsServerRejectedUnsubscription(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	errors := make(chan error, 1)
	pubSub := newRedisPubSub(context.Background(), client, func(err error) {
		server.SetError("")
		select {
		case errors <- err:
		default:
		}
	})
	t.Cleanup(pubSub.Close)

	subscription := pubSub.newSubscription(func([]byte, string) {})
	subscription.Subscribe("ghost")
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "ghost").Result()
		return err == nil && counts["ghost"] == 1
	})

	server.SetError("NOPERM injected unsubscription failure")
	subscription.Close()
	select {
	case <-errors:
	case <-time.After(3 * time.Second):
		t.Fatal("server-side unsubscription failure was not observed")
	}
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "ghost").Result()
		return err == nil && counts["ghost"] == 0
	})

	received := make(chan struct{}, 1)
	probe := pubSub.newSubscription(func([]byte, string) { received <- struct{}{} })
	probe.Subscribe("probe")
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "ghost", "probe").Result()
		return err == nil && counts["ghost"] == 0 && counts["probe"] == 1
	})
	if err := client.Publish(context.Background(), "probe", "payload").Err(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("replacement Pub/Sub connection did not receive a message")
	}
}

func TestRedisPubSubRemoveAfterFailedSubscribeDoesNotLeak(t *testing.T) {
	server := miniredis.RunT(t)
	addr := server.Addr()
	server.Close()
	client := rds.NewClient(&rds.Options{
		Addr:         addr,
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
	})
	t.Cleanup(func() { _ = client.Close() })
	errors := make(chan error, 1)
	pubSub := newRedisPubSub(context.Background(), client, func(err error) {
		select {
		case errors <- err:
		default:
		}
	})
	t.Cleanup(pubSub.Close)
	subscription := pubSub.newSubscription(func([]byte, string) {})
	subscription.Subscribe("ghost")
	select {
	case <-errors:
	case <-time.After(3 * time.Second):
		t.Fatal("subscription failure was not observed")
	}
	subscription.Close()
	if err := server.Restart(); err != nil {
		t.Fatal(err)
	}
	probe := pubSub.newSubscription(func([]byte, string) {})
	probe.Subscribe("probe")
	defer probe.Close()

	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(context.Background(), "ghost", "probe").Result()
		return err == nil && counts["ghost"] == 0 && counts["probe"] == 1
	})
}

func TestRedisPubSubBatchesSubscriptionChanges(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	transport := client.Subscribe(ctx, "warmup")
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(ctx, "warmup").Result()
		return err == nil && counts["warmup"] == 1
	})
	if err := transport.Unsubscribe(ctx, "warmup"); err != nil {
		t.Fatal(err)
	}
	waitForRedisPubSub(t, func() bool {
		counts, err := client.PubSubNumSub(ctx, "warmup").Result()
		return err == nil && counts["warmup"] == 0
	})

	pubSub := &redisPubSub{
		ctx:      ctx,
		cancel:   cancel,
		pubSub:   transport,
		channels: newRedisPubSubRoutes(),
		patterns: newRedisPubSubRoutes(),
	}
	t.Cleanup(pubSub.Close)

	const channelCount = 64
	subscription := &redisSubscription{}
	channels := make([]string, channelCount)
	for i := range channels {
		channel := fmt.Sprintf("batch:%d", i)
		channels[i] = channel
		pubSub.channels.handlers[channel] = redisPubSubRoute{first: subscription}
		pubSub.channels.dirty[channel] = struct{}{}
	}

	before := server.CommandCount()
	if pubSub.reconcile(&pubSub.channels, false) {
		t.Fatal("batched subscription failed")
	}
	waitForRedisPubSub(t, func() bool { return server.CommandCount() > before })
	if commands := server.CommandCount() - before; commands != 1 {
		t.Fatalf("subscribe commands = %d, want 1", commands)
	}
	if len(pubSub.channels.active) != channelCount {
		t.Fatalf("active channels = %d, want %d", len(pubSub.channels.active), channelCount)
	}

	for _, channel := range channels {
		delete(pubSub.channels.handlers, channel)
		pubSub.channels.dirty[channel] = struct{}{}
	}
	before = server.CommandCount()
	if pubSub.reconcile(&pubSub.channels, false) {
		t.Fatal("batched unsubscription failed")
	}
	waitForRedisPubSub(t, func() bool { return server.CommandCount() > before })
	if commands := server.CommandCount() - before; commands != 1 {
		t.Fatalf("unsubscribe commands = %d, want 1", commands)
	}
	if len(pubSub.channels.active) != 0 {
		t.Fatalf("active channels after unsubscribe = %d, want 0", len(pubSub.channels.active))
	}
}

func TestRedisPubSubReportsBatchFailureOnce(t *testing.T) {
	server := miniredis.RunT(t)
	addr := server.Addr()
	server.Close()
	client := rds.NewClient(&rds.Options{
		Addr:         addr,
		MaxRetries:   -1,
		DialTimeout:  20 * time.Millisecond,
		ReadTimeout:  20 * time.Millisecond,
		WriteTimeout: 20 * time.Millisecond,
	})
	t.Cleanup(func() { _ = client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	var errors atomic.Int64
	pubSub := &redisPubSub{
		ctx:      ctx,
		cancel:   cancel,
		pubSub:   client.Subscribe(ctx),
		channels: newRedisPubSubRoutes(),
		patterns: newRedisPubSubRoutes(),
		onError:  func(error) { errors.Add(1) },
	}
	t.Cleanup(pubSub.Close)

	subscription := &redisSubscription{}
	for i := range 64 {
		channel := fmt.Sprintf("failed-batch:%d", i)
		pubSub.channels.handlers[channel] = redisPubSubRoute{first: subscription}
		pubSub.channels.dirty[channel] = struct{}{}
	}
	if !pubSub.reconcile(&pubSub.channels, false) {
		t.Fatal("failed batch was reported as successful")
	}
	if got := errors.Load(); got != 1 {
		t.Fatalf("reported errors = %d, want one error for the batch", got)
	}
	if got := len(pubSub.channels.dirty); got != 64 {
		t.Fatalf("retry channels = %d, want 64", got)
	}
}

func TestRedisAdapterBuildersSharePubSubPerServer(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	io := socket.NewServer(nil, nil)
	builder := &RedisAdapterBuilder{Redis: redisClient}

	const namespaceCount = 16
	adapters := make([]*redisAdapter, 0, namespaceCount)
	for i := range namespaceCount {
		current := builder.New(socket.NewNamespace(io, "/nsp"+string(rune('a'+i)))).(*redisAdapter)
		adapters = append(adapters, current)
		if current.pubSub != adapters[0].pubSub {
			t.Fatal("namespaces on the same server did not share Pub/Sub")
		}
	}
	if got := redisClient.ListenerCount("error"); got != 1 {
		t.Fatalf("error listeners = %d, want one wrapper fallback", got)
	}

	shared := adapters[0].pubSub
	for _, current := range adapters[:len(adapters)-1] {
		current.Close()
		current.Close()
		shared.mu.RLock()
		closed := shared.closed
		shared.mu.RUnlock()
		if closed {
			t.Fatal("shared Pub/Sub closed before its last adapter")
		}
	}
	adapters[len(adapters)-1].Close()
	shared.mu.RLock()
	closed := shared.closed
	shared.mu.RUnlock()
	if !closed {
		t.Fatal("shared Pub/Sub remained open after its last adapter")
	}
	if got := redisClient.ListenerCount("error"); got != 1 {
		t.Fatalf("error listeners after close = %d, want one wrapper fallback", got)
	}
}

func TestRedisAdaptersSubscribeOnConstruct(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	io := socket.NewServer(nil, nil)

	classic := NewRedisAdapter(socket.NewNamespace(io, "/ready-classic"), redisClient, nil).(*redisAdapter)
	defer classic.Close()
	waitForRedisPubSub(t, func() bool {
		count, err := classic.ServerCount()
		if err != nil || count != 1 {
			return false
		}
		count, err = client.PubSubNumPat(context.Background()).Result()
		return err == nil && count == 1
	})

	streams := NewRedisStreamsAdapter(socket.NewNamespace(io, "/ready-streams"), redisClient, nil).(*redisStreamsAdapter)
	defer streams.Close()
	waitForRedisPubSub(t, func() bool {
		count, err := streams.ServerCount()
		return err == nil && count == 1
	})
}

func TestRedisAdapterServerCountUsesSubClient(t *testing.T) {
	writeServer := miniredis.RunT(t)
	subServer := miniredis.RunT(t)
	writeClient := rds.NewClient(&rds.Options{Addr: writeServer.Addr()})
	subClient := rds.NewClient(&rds.Options{Addr: subServer.Addr()})
	t.Cleanup(func() {
		_ = writeClient.Close()
		_ = subClient.Close()
	})

	redisClient := mustRedisClientWithSub(t, context.Background(), writeClient, subClient)
	current := NewRedisAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/count-classic"),
		redisClient,
		nil,
	).(*redisAdapter)
	t.Cleanup(current.Close)

	waitForRedisPubSub(t, func() bool {
		counts, err := subClient.PubSubNumSub(context.Background(), current.requestChannel).Result()
		return err == nil && counts[current.requestChannel] == 1
	})
	if counts, err := writeClient.PubSubNumSub(context.Background(), current.requestChannel).Result(); err != nil {
		t.Fatal(err)
	} else if counts[current.requestChannel] != 0 {
		t.Fatalf("write client subscriber count = %d, want 0", counts[current.requestChannel])
	}

	count, err := current.ServerCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("server count = %d, want 1 from SubClient", count)
	}
}

func TestRedisStreamsAdapterServerCountUsesSubClient(t *testing.T) {
	writeServer := miniredis.RunT(t)
	subServer := miniredis.RunT(t)
	writeClient := rds.NewClient(&rds.Options{Addr: writeServer.Addr()})
	subClient := rds.NewClient(&rds.Options{Addr: subServer.Addr()})
	t.Cleanup(func() {
		_ = writeClient.Close()
		_ = subClient.Close()
	})

	redisClient := mustRedisClientWithSub(t, context.Background(), writeClient, subClient)
	current := NewRedisStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/count-streams"),
		redisClient,
		nil,
	).(*redisStreamsAdapter)
	t.Cleanup(current.Close)

	waitForRedisPubSub(t, func() bool {
		counts, err := subClient.PubSubNumSub(context.Background(), current.publicChannel).Result()
		return err == nil && counts[current.publicChannel] == 1
	})
	if counts, err := writeClient.PubSubNumSub(context.Background(), current.publicChannel).Result(); err != nil {
		t.Fatal(err)
	} else if counts[current.publicChannel] != 0 {
		t.Fatalf("write client subscriber count = %d, want 0", counts[current.publicChannel])
	}

	count, err := current.ServerCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("server count = %d, want 1 from SubClient", count)
	}
}

func TestPubSubNumSubIncludesClusterReplicas(t *testing.T) {
	master := miniredis.RunT(t)
	replica := miniredis.RunT(t)
	replicaClient := rds.NewClient(&rds.Options{Addr: replica.Addr()})
	t.Cleanup(func() { _ = replicaClient.Close() })

	const channel = "socket.io-request#/#"
	subscription := replicaClient.Subscribe(context.Background(), channel)
	t.Cleanup(func() { _ = subscription.Close() })
	waitForRedisPubSub(t, func() bool {
		counts, err := replicaClient.PubSubNumSub(context.Background(), channel).Result()
		return err == nil && counts[channel] == 1
	})

	cluster := rds.NewClusterClient(&rds.ClusterOptions{
		Addrs: []string{master.Addr()},
		ClusterSlots: func(context.Context) ([]rds.ClusterSlot, error) {
			return []rds.ClusterSlot{{
				Start: 0,
				End:   16383,
				Nodes: []rds.ClusterNode{{Addr: master.Addr()}, {Addr: replica.Addr()}},
			}}, nil
		},
	})
	t.Cleanup(func() { _ = cluster.Close() })

	count, err := pubSubNumSub(context.Background(), cluster, false, channel)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("cluster subscriber count = %d, want replica subscription", count)
	}
}

func TestPubSubNumSubRoutesShardedCountToOwner(t *testing.T) {
	first := miniredis.RunT(t)
	second := miniredis.RunT(t)
	const channel = "socket.io#/#"
	var hooks sync.Map
	cluster := rds.NewClusterClient(&rds.ClusterOptions{
		Addrs: []string{first.Addr(), second.Addr()},
		NewClient: func(options *rds.Options) *rds.Client {
			client := rds.NewClient(options)
			hook := &pubSubShardCountHook{channel: channel, count: 2}
			client.AddHook(hook)
			hooks.Store(client, hook)
			return client
		},
		ClusterSlots: func(context.Context) ([]rds.ClusterSlot, error) {
			return []rds.ClusterSlot{
				{Start: 0, End: 8191, Nodes: []rds.ClusterNode{{Addr: first.Addr()}}},
				{Start: 8192, End: 16383, Nodes: []rds.ClusterNode{{Addr: second.Addr()}}},
			}, nil
		},
	})
	t.Cleanup(func() { _ = cluster.Close() })
	owner, err := cluster.MasterForKey(t.Context(), channel)
	if err != nil {
		t.Fatal(err)
	}
	value, ok := hooks.Load(owner)
	if !ok {
		t.Fatal("channel owner was not created through ClusterOptions.NewClient")
	}
	ownerHook := value.(*pubSubShardCountHook)

	count, err := pubSubNumSub(t.Context(), cluster, true, channel)
	if err != nil {
		t.Fatal(err)
	}
	var otherCalls int64
	hooks.Range(func(key, value any) bool {
		if key != owner {
			otherCalls += value.(*pubSubShardCountHook).calls.Load()
		}
		return true
	})
	if count != 2 || ownerHook.calls.Load() != 1 || otherCalls != 0 {
		t.Fatalf("sharded count/owner calls/other calls = %d/%d/%d, want 2/1/0",
			count, ownerHook.calls.Load(), otherCalls)
	}
}

func TestClassicAndStreamsAdaptersSharePubSub(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	io := socket.NewServer(nil, nil)

	classic := NewRedisAdapter(socket.NewNamespace(io, "/classic"), redisClient, nil).(*redisAdapter)
	streams := NewRedisStreamsAdapter(socket.NewNamespace(io, "/streams"), redisClient, nil).(*redisStreamsAdapter)
	if classic.pubSub != streams.pubSub {
		t.Fatal("classic and Streams adapters did not share normal Pub/Sub")
	}

	shared := classic.pubSub
	classic.Close()
	shared.mu.RLock()
	closed := shared.closed
	shared.mu.RUnlock()
	if closed {
		t.Fatal("Streams adapter lost Pub/Sub when classic adapter closed")
	}
	streams.Close()
	shared.mu.RLock()
	closed = shared.closed
	shared.mu.RUnlock()
	if !closed {
		t.Fatal("shared Pub/Sub remained open after both adapters closed")
	}
}
