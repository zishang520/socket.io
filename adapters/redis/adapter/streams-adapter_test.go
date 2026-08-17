package adapter

import (
	"context"
	"encoding/base64"
	"reflect"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	rediswire "github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type xrangeRecorder struct {
	mu   sync.Mutex
	args [][]any
}

type xreadRecorder struct {
	streams chan string
}

func (*xrangeRecorder) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *xrangeRecorder) ProcessHook(next rds.ProcessHook) rds.ProcessHook {
	return func(ctx context.Context, cmd rds.Cmder) error {
		if cmd.Name() == "xrange" {
			h.mu.Lock()
			h.args = append(h.args, append([]any(nil), cmd.Args()...))
			h.mu.Unlock()
		}
		return next(ctx, cmd)
	}
}

func (*xrangeRecorder) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func (*xreadRecorder) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *xreadRecorder) ProcessHook(next rds.ProcessHook) rds.ProcessHook {
	return func(ctx context.Context, cmd rds.Cmder) error {
		if args := cmd.Args(); cmd.Name() == "xread" && len(args) >= 3 {
			if stream, ok := args[len(args)-2].(string); ok {
				select {
				case h.streams <- stream:
				default:
				}
			}
		}
		return next(ctx, cmd)
	}
}

func (*xreadRecorder) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func TestRedisStreamsAdapterBuilderAppliesDefaults(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	redisClient := mustRedisClient(t, context.Background(), client)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")

	streamAdapter := (&RedisStreamsAdapterBuilder{Redis: redisClient}).New(nsp).(*redisStreamsAdapter)
	t.Cleanup(streamAdapter.Close)

	if streamAdapter.streamName != DefaultStreamName {
		t.Fatalf("stream name = %q, want %q", streamAdapter.streamName, DefaultStreamName)
	}
	if streamAdapter.publicChannel != DefaultChannelPrefix+"#/test#" {
		t.Fatalf("public channel = %q", streamAdapter.publicChannel)
	}
	if streamAdapter.opts.MaxLen() != DefaultStreamMaxLen {
		t.Fatalf("max length = %d, want %d", streamAdapter.opts.MaxLen(), DefaultStreamMaxLen)
	}
}

func TestRedisStreamsAdapterPreservesExplicitEmptyAndZeroOptions(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	opts := DefaultRedisStreamsAdapterOptions()
	opts.SetStreamName("")
	opts.SetStreamCount(0)
	opts.SetChannelPrefix("")
	opts.SetMaxLen(0)
	opts.SetReadCount(0)
	opts.SetBlockTimeInMs(0)
	opts.SetSessionKeyPrefix("")

	streamAdapter := NewRedisStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/explicit"),
		mustRedisClient(t, ctx, client),
		opts,
	).(*redisStreamsAdapter)
	t.Cleanup(streamAdapter.Close)

	if streamAdapter.opts.StreamName() != "" || streamAdapter.opts.StreamCount() != 0 ||
		streamAdapter.opts.ChannelPrefix() != "" || streamAdapter.opts.MaxLen() != 0 ||
		streamAdapter.opts.ReadCount() != 0 || streamAdapter.opts.BlockTimeInMs() != 0 ||
		streamAdapter.opts.SessionKeyPrefix() != "" {
		t.Fatalf("explicit options were replaced: %+v", streamAdapter.opts)
	}
}

func TestRedisStreamsCleanupReleasesPollerAndAllowsClose(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	builder := &RedisStreamsAdapterBuilder{Redis: redisClient}
	streamAdapter := builder.New(socket.NewNamespace(socket.NewServer(nil, nil), "/cleanup")).(*redisStreamsAdapter)
	poller := streamAdapter.streamPoller

	called := false
	streamAdapter.Cleanup(func() {
		called = true
		streamAdapter.Close()
	})
	done := make(chan struct{})
	go func() {
		streamAdapter.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reentrant Close deadlocked")
	}
	if !called {
		t.Fatal("cleanup callback was not called")
	}
	if poller.ctx.Err() == nil {
		t.Fatal("public cleanup prevented the stream poller from being released")
	}
}

func TestRedisStreamsAdapterBuilderLifecycle(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	socketServer := socket.NewServer(nil, nil)
	nsp := socket.NewNamespace(socketServer, "/test")
	builder := &RedisStreamsAdapterBuilder{Redis: redisClient}

	first := builder.New(nsp).(*redisStreamsAdapter)
	second := builder.New(nsp).(*redisStreamsAdapter)
	poller := first.streamPoller
	if second.streamPoller != poller {
		t.Fatal("adapters for the same server did not share a stream poller")
	}
	first.Close()

	if current, ok := poller.adapters.Load(nsp.Name()); !ok || current != second {
		t.Fatal("closing the replaced adapter removed the active adapter")
	}
	if poller.ctx.Err() != nil {
		t.Fatal("polling stopped while an adapter was active")
	}

	otherNsp := socket.NewNamespace(socketServer, "/other")
	other := builder.New(otherNsp).(*redisStreamsAdapter)
	if other.streamPoller != poller {
		t.Fatal("namespaces for the same server did not share a stream poller")
	}
	second.Close()
	if poller.adapters.Len() != 1 {
		t.Fatal("polling stopped before the last adapter closed")
	}
	if poller.ctx.Err() != nil {
		t.Fatal("polling stopped before the last adapter closed")
	}

	other.Close()
	if poller.ctx.Err() == nil || poller.adapters.Len() != 0 {
		t.Fatal("polling did not stop after the last adapter closed")
	}

	third := builder.New(nsp).(*redisStreamsAdapter)
	if third.streamPoller == poller {
		t.Fatal("a stopped stream poller was reused")
	}
	if third.streamPoller.ctx.Err() != nil {
		t.Fatal("polling did not restart for a new adapter")
	}
	third.Close()
}

func TestRedisStreamsPollerSharedAcrossConstructors(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	socketServer := socket.NewServer(nil, nil)

	direct := NewRedisStreamsAdapter(
		socket.NewNamespace(socketServer, "/direct"), redisClient, nil,
	).(*redisStreamsAdapter)
	first := (&RedisStreamsAdapterBuilder{Redis: redisClient}).New(
		socket.NewNamespace(socketServer, "/first"),
	).(*redisStreamsAdapter)
	second := (&RedisStreamsAdapterBuilder{Redis: redisClient}).New(
		socket.NewNamespace(socketServer, "/second"),
	).(*redisStreamsAdapter)
	t.Cleanup(direct.Close)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	if direct.streamPoller != first.streamPoller || direct.streamPoller != second.streamPoller {
		t.Fatal("direct constructor and separate builders did not share a stream poller")
	}
}

func TestRedisStreamsPollerIsolation(t *testing.T) {
	server := miniredis.RunT(t)
	firstClient := rds.NewClient(&rds.Options{Addr: server.Addr()})
	secondClient := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() {
		_ = firstClient.Close()
		_ = secondClient.Close()
	})
	firstRedis := mustRedisClient(t, context.Background(), firstClient)
	secondRedis := mustRedisClient(t, context.Background(), secondClient)
	socketServer := socket.NewServer(nil, nil)

	base := NewRedisStreamsAdapter(
		socket.NewNamespace(socketServer, "/base"), firstRedis, nil,
	).(*redisStreamsAdapter)
	otherServer := NewRedisStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/server"), firstRedis, nil,
	).(*redisStreamsAdapter)
	otherClient := NewRedisStreamsAdapter(
		socket.NewNamespace(socketServer, "/client"), secondRedis, nil,
	).(*redisStreamsAdapter)
	opts := DefaultRedisStreamsAdapterOptions()
	opts.SetReadCount(DefaultStreamReadCount + 1)
	otherConfig := NewRedisStreamsAdapter(
		socket.NewNamespace(socketServer, "/config"), firstRedis, opts,
	).(*redisStreamsAdapter)
	t.Cleanup(base.Close)
	t.Cleanup(otherServer.Close)
	t.Cleanup(otherClient.Close)
	t.Cleanup(otherConfig.Close)

	for name, adapter := range map[string]*redisStreamsAdapter{
		"server": otherServer,
		"client": otherClient,
		"config": otherConfig,
	} {
		if adapter.streamPoller == base.streamPoller {
			t.Fatalf("different %s unexpectedly shared a stream poller", name)
		}
	}
}

func TestRedisStreamsPollerReadsComputedNegativeStream(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	recorder := &xreadRecorder{streams: make(chan string, 1)}
	client.AddHook(recorder)

	opts := DefaultRedisStreamsAdapterOptions()
	opts.SetStreamCount(5)
	streamAdapter := NewRedisStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/namespace-0"),
		mustRedisClient(t, context.Background(), client),
		opts,
	).(*redisStreamsAdapter)
	t.Cleanup(streamAdapter.Close)

	if streamAdapter.streamName != "socket.io--3" {
		t.Fatalf("stream name = %q, want socket.io--3", streamAdapter.streamName)
	}
	if stream := streamAdapter.streamPoller.key.streamName; stream != streamAdapter.streamName {
		t.Fatalf("poller stream = %q, want %q", stream, streamAdapter.streamName)
	}
	select {
	case stream := <-recorder.streams:
		if stream != streamAdapter.streamName {
			t.Fatalf("XREAD stream = %q, want %q", stream, streamAdapter.streamName)
		}
	case <-time.After(time.Second):
		t.Fatal("XREAD was not started")
	}
}

func TestRedisStreamsPollerStartsOneWorkerPerStream(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	socketServer := socket.NewServer(nil, nil)

	first := NewRedisStreamsAdapter(
		socket.NewNamespace(socketServer, "/first"), redisClient, nil,
	).(*redisStreamsAdapter)
	second := NewRedisStreamsAdapter(
		socket.NewNamespace(socketServer, "/second"), redisClient, nil,
	).(*redisStreamsAdapter)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	if first.streamPoller != second.streamPoller {
		t.Fatal("adapters did not share a poller")
	}
	if stream := first.streamPoller.key.streamName; stream != DefaultStreamName {
		t.Fatalf("poller stream = %q, want %q", stream, DefaultStreamName)
	}
}

func TestRedisStreamsPollerRoutesAndStopsAfterClose(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	socketServer := socket.NewServer(nil, nil)
	builder := &RedisStreamsAdapterBuilder{Redis: redisClient}

	firstNsp := socket.NewNamespace(socketServer, "/first")
	secondNsp := socket.NewNamespace(socketServer, "/second")
	first := builder.New(firstNsp).(*redisStreamsAdapter)
	second := builder.New(secondNsp).(*redisStreamsAdapter)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	received := make(chan string, 8)
	if err := firstNsp.On("probe", func(...any) { received <- "first" }); err != nil {
		t.Fatal(err)
	}
	if err := secondNsp.On("probe", func(...any) { received <- "second" }); err != nil {
		t.Fatal(err)
	}

	publish := func(nsp string) {
		t.Helper()
		message, err := rediswire.EncodeStreamMessage(&adapter.ClusterMessage{
			Uid:  "remote",
			Nsp:  nsp,
			Type: adapter.SERVER_SIDE_EMIT,
			Data: &adapter.ServerSideEmitMessage{Packet: []any{"probe"}},
		}, true)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rediswire.XAdd(redisClient, first.streamName, message, first.opts.MaxLen()); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(time.Second)
	for {
		publish(firstNsp.Name())
		select {
		case target := <-received:
			if target != "first" {
				t.Fatalf("message routed to %s namespace, want first", target)
			}
			goto polling
		case <-time.After(10 * time.Millisecond):
			if time.Now().After(deadline) {
				t.Fatal("stream poller did not receive a message")
			}
		}
	}

polling:
	first.Close()
	publish(firstNsp.Name())
	publish(secondNsp.Name())

	select {
	case target := <-received:
		if target != "second" {
			t.Fatalf("closed namespace received a message: %s", target)
		}
	case <-time.After(time.Second):
		t.Fatal("shared stream poller stopped with an active namespace")
	}
	select {
	case target := <-received:
		t.Fatalf("unexpected extra delivery to %s namespace", target)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestRedisStreamsShardedBuilderSharesSubscriber(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	opts := DefaultRedisStreamsAdapterOptions()
	opts.SetUseShardedPubSub(true)
	builder := &RedisStreamsAdapterBuilder{Redis: redisClient, Opts: opts}
	socketServer := socket.NewServer(nil, nil)
	firstNsp := socket.NewNamespace(socketServer, "/first")
	secondNsp := socket.NewNamespace(socketServer, "/second")
	first := builder.New(firstNsp).(*redisStreamsAdapter)
	second := builder.New(secondNsp).(*redisStreamsAdapter)

	channels := []string{
		first.publicChannel,
		first.publicChannel + string(first.Uid()) + "#",
		second.publicChannel,
		second.publicChannel + string(second.Uid()) + "#",
	}
	waitForShardedState(t, func() bool {
		for _, channel := range channels {
			if len(recorder.activePeers(channel)) != 1 {
				return false
			}
		}
		return true
	})
	peer := recorder.activePeers(channels[0])[0]
	for _, channel := range channels[1:] {
		if recorder.activePeers(channel)[0] != peer {
			t.Fatal("Streams namespaces did not share the sharded subscriber")
		}
	}

	received := make(chan struct{}, 1)
	if err := secondNsp.On("probe", func(...any) { received <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	payload := shardedServerSideEmitPayload(t, "/second")
	if delivered := recorder.publish(second.publicChannel, payload); delivered != 1 {
		t.Fatalf("delivered to %d subscribers, want 1", delivered)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("shared Streams subscriber did not route the message")
	}

	first.Close()
	if peer.Closed() {
		t.Fatal("closing one Streams namespace closed the shared subscriber")
	}
	second.Close()
	waitForShardedState(t, peer.Closed)
}

func TestShardedAdaptersShareSubscriberAcrossTypes(t *testing.T) {
	server, recorder := newShardedPubSubRecorder(t, 0)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, context.Background(), client)
	socketServer := socket.NewServer(nil, nil)

	shardedNsp := socket.NewNamespace(socketServer, "/sharded")
	sharded := NewShardedRedisAdapter(shardedNsp, redisClient, nil).(*shardedRedisAdapter)
	streamOpts := DefaultRedisStreamsAdapterOptions()
	streamOpts.SetUseShardedPubSub(true)
	streamsNsp := socket.NewNamespace(socketServer, "/streams")
	streams := NewRedisStreamsAdapter(streamsNsp, redisClient, streamOpts).(*redisStreamsAdapter)
	t.Cleanup(sharded.Close)
	t.Cleanup(streams.Close)

	waitForShardedState(t, func() bool {
		return len(recorder.activePeers(sharded.channel)) == 1 &&
			len(recorder.activePeers(streams.publicChannel)) == 1
	})
	peer := recorder.activePeers(sharded.channel)[0]
	if recorder.activePeers(streams.publicChannel)[0] != peer {
		t.Fatal("sharded and Streams adapters did not share the subscriber")
	}

	received := make(chan string, 3)
	if err := shardedNsp.On("probe", func(...any) { received <- "sharded" }); err != nil {
		t.Fatal(err)
	}
	if err := streamsNsp.On("probe", func(...any) { received <- "streams" }); err != nil {
		t.Fatal(err)
	}
	if delivered := recorder.publish(sharded.channel, shardedServerSideEmitPayload(t, "/sharded")); delivered != 1 {
		t.Fatalf("delivered sharded message to %d subscribers, want 1", delivered)
	}
	if delivered := recorder.publish(streams.publicChannel, shardedServerSideEmitPayload(t, "/streams")); delivered != 1 {
		t.Fatalf("delivered Streams message to %d subscribers, want 1", delivered)
	}

	seen := map[string]int{}
	for range 2 {
		select {
		case target := <-received:
			seen[target]++
		case <-time.After(time.Second):
			t.Fatal("shared subscriber did not route both messages")
		}
	}
	if seen["sharded"] != 1 || seen["streams"] != 1 {
		t.Fatalf("messages routed to wrong namespaces: %v", seen)
	}
	select {
	case target := <-received:
		t.Fatalf("message was delivered more than once to %s", target)
	case <-time.After(20 * time.Millisecond):
	}

	sharded.Close()
	if peer.Closed() {
		t.Fatal("closing the sharded adapter closed the Streams subscriber")
	}
	streams.Close()
	waitForShardedState(t, peer.Closed)
}

func TestRestoreSessionReturnsEmptyMissedPackets(t *testing.T) {
	server := miniredis.RunT(t)
	replica := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	replicaClient := rds.NewClient(&rds.Options{Addr: replica.Addr()})
	t.Cleanup(func() {
		_ = client.Close()
		_ = replicaClient.Close()
	})
	redisClient := mustRedisClientWithSub(t, context.Background(), client, replicaClient)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	streamAdapter.ClusterAdapter.Construct(nsp)
	streamAdapter.redisClient = redisClient
	streamAdapter.streamName = "stream"
	streamAdapter.opts.SetSessionKeyPrefix(DefaultSessionKeyPrefix)

	session := &socket.SessionToPersist{
		Sid:   "sid",
		Pid:   "pid",
		Rooms: types.NewSet[socket.Room](),
	}
	payload, err := utils.MsgPack().Encode(session)
	if err != nil {
		t.Fatal(err)
	}
	err = client.Set(context.Background(), DefaultSessionKeyPrefix+"pid", base64.StdEncoding.EncodeToString(payload), 0).Err()
	if err != nil {
		t.Fatal(err)
	}
	err = client.XAdd(context.Background(), &rds.XAddArgs{
		Stream: "stream",
		ID:     "1-0",
		Values: map[string]any{"uid": "node", "nsp": "/test", "type": "1"},
	}).Err()
	if err != nil {
		t.Fatal(err)
	}

	restored, err := streamAdapter.RestoreSession("pid", "1-0")
	if err != nil {
		t.Fatal(err)
	}
	if restored.MissedPackets == nil || len(restored.MissedPackets) != 0 {
		t.Fatalf("missed packets = %#v, want non-nil empty slice", restored.MissedPackets)
	}
}

func TestRedisStreamsRecoveryClientRoutesToStreamOwner(t *testing.T) {
	const stream = "stream"
	ctx := context.Background()

	t.Run("standalone", func(t *testing.T) {
		server := miniredis.RunT(t)
		client := rds.NewClient(&rds.Options{Addr: server.Addr()})
		t.Cleanup(func() { _ = client.Close() })
		streamAdapter := &redisStreamsAdapter{
			redisClient: mustRedisClient(t, ctx, client),
			streamName:  stream,
		}

		got, err := streamAdapter.recoveryClient()
		if err != nil {
			t.Fatal(err)
		}
		if got != client {
			t.Fatal("standalone recovery did not use the write client")
		}
	})

	t.Run("cluster master", func(t *testing.T) {
		master := miniredis.RunT(t)
		replica := miniredis.RunT(t)
		cluster := rds.NewClusterClient(&rds.ClusterOptions{
			Addrs:    []string{master.Addr()},
			ReadOnly: true,
			ClusterSlots: func(context.Context) ([]rds.ClusterSlot, error) {
				return []rds.ClusterSlot{{
					Start: 0,
					End:   16383,
					Nodes: []rds.ClusterNode{{Addr: master.Addr()}, {Addr: replica.Addr()}},
				}}, nil
			},
		})
		t.Cleanup(func() { _ = cluster.Close() })
		streamAdapter := &redisStreamsAdapter{
			redisClient: mustRedisClient(t, ctx, cluster),
			streamName:  stream,
		}

		want, err := cluster.MasterForKey(ctx, stream)
		if err != nil {
			t.Fatal(err)
		}
		got, err := streamAdapter.recoveryClient()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("recovery client = %T, want cluster master %s", got, want.Options().Addr)
		}
	})

}

func TestCollectMissedPacketsUsesBoundedPages(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	recorder := &xrangeRecorder{}
	client.AddHook(recorder)
	ctx := context.Background()
	pipe := client.Pipeline()
	for range restoreSessionPageSize + 1 {
		pipe.XAdd(ctx, &rds.XAddArgs{
			Stream: DefaultStreamName,
			Values: map[string]any{"nsp": "/other"},
		})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatal(err)
	}

	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	streamAdapter.ClusterAdapter.Construct(nsp)
	streamAdapter.redisClient = mustRedisClient(t, ctx, client)
	streamAdapter.streamName = DefaultStreamName

	if err := streamAdapter.collectMissedPackets(client, &socket.Session{}, "0-0"); err != nil {
		t.Fatal(err)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.args) != 2 {
		t.Fatalf("XRANGE calls = %d, want 2", len(recorder.args))
	}
	for _, args := range recorder.args {
		if len(args) < 6 || args[len(args)-2] != "count" || args[len(args)-1] != int64(restoreSessionPageSize) {
			t.Fatalf("XRANGE args = %#v, want COUNT %d", args, restoreSessionPageSize)
		}
	}
}

func TestCollectMissedPacketsSkipsVolatileBroadcasts(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	add := func(id, event string, volatile bool) {
		message, err := rediswire.EncodeStreamMessage(&adapter.ClusterMessage{
			Uid:  "remote",
			Nsp:  "/test",
			Type: adapter.BROADCAST,
			Data: &adapter.BroadcastMessage{
				Packet: &parser.Packet{Type: parser.EVENT, Data: []any{event}},
				Opts: &adapter.PacketOptions{Flags: &socket.BroadcastFlags{
					WriteOptions: socket.WriteOptions{Volatile: volatile},
				}},
			},
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := client.XAdd(ctx, &rds.XAddArgs{
			Stream: "stream",
			ID:     id,
			Values: map[string]any(message),
		}).Err(); err != nil {
			t.Fatal(err)
		}
	}
	add("1-0", "volatile", true)
	add("2-0", "durable", false)

	streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	streamAdapter.ClusterAdapter.Construct(socket.NewNamespace(socket.NewServer(nil, nil), "/test"))
	streamAdapter.redisClient = mustRedisClient(t, ctx, client)
	streamAdapter.streamName = "stream"
	session := &socket.Session{
		SessionToPersist: &socket.SessionToPersist{Rooms: types.NewSet[socket.Room]()},
		MissedPackets:    []any{},
	}

	if err := streamAdapter.collectMissedPackets(client, session, "0-0"); err != nil {
		t.Fatal(err)
	}
	want := []any{[]any{"durable", "2-0"}}
	if !reflect.DeepEqual(session.MissedPackets, want) {
		t.Fatalf("missed packets = %#v, want %#v", session.MissedPackets, want)
	}
}

func TestRestoreSessionUsesWriteClientForStreamReads(t *testing.T) {
	writeServer := miniredis.RunT(t)
	readServer := miniredis.RunT(t)
	writeClient := rds.NewClient(&rds.Options{Addr: writeServer.Addr()})
	readClient := rds.NewClient(&rds.Options{Addr: readServer.Addr()})
	t.Cleanup(func() {
		_ = writeClient.Close()
		_ = readClient.Close()
	})

	ctx := context.Background()
	redisClient := mustRedisClientWithSub(t, ctx, writeClient, readClient)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	streamAdapter.ClusterAdapter.Construct(nsp)
	streamAdapter.redisClient = redisClient
	streamAdapter.streamName = "stream"
	streamAdapter.opts.SetSessionKeyPrefix(DefaultSessionKeyPrefix)

	session := &socket.SessionToPersist{
		Sid:   "sid",
		Pid:   "pid",
		Rooms: types.NewSet[socket.Room](),
	}
	payload, err := utils.MsgPack().Encode(session)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeClient.Set(ctx, DefaultSessionKeyPrefix+"pid", base64.StdEncoding.EncodeToString(payload), 0).Err(); err != nil {
		t.Fatal(err)
	}

	if err = writeClient.XAdd(ctx, &rds.XAddArgs{
		Stream: "stream",
		ID:     "1-0",
		Values: map[string]any{"uid": "node", "nsp": "/test", "type": "1"},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	// Simulate a replica that has received the offset but not the subsequent
	// broadcast yet. Recovery must read the authoritative write-side stream.
	if err = readClient.XAdd(ctx, &rds.XAddArgs{
		Stream: "stream",
		ID:     "1-0",
		Values: map[string]any{"uid": "node", "nsp": "/test", "type": "1"},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	wireMessage, err := rediswire.EncodeStreamMessage(&adapter.ClusterMessage{
		Uid:  "remote",
		Nsp:  "/test",
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", "payload"}},
			Opts:   new(adapter.PacketOptions),
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeClient.XAdd(ctx, &rds.XAddArgs{
		Stream: "stream",
		ID:     "2-0",
		Values: map[string]any(wireMessage),
	}).Err(); err != nil {
		t.Fatal(err)
	}

	restored, err := streamAdapter.RestoreSession("pid", "1-0")
	if err != nil {
		t.Fatal(err)
	}
	want := []any{[]any{"event", "payload", "2-0"}}
	if !reflect.DeepEqual(restored.MissedPackets, want) {
		t.Fatalf("missed packets = %#v, want %#v", restored.MissedPackets, want)
	}
}

func TestRawClusterMessage_Getters(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/chat",
		"type": "1",
		"data": `{"key":"value"}`,
	}

	t.Run("Uid", func(t *testing.T) {
		if got := rawMsg.Uid(); got != "server-1" {
			t.Errorf("Expected 'server-1', got %q", got)
		}
	})

	t.Run("Nsp", func(t *testing.T) {
		if got := rawMsg.Nsp(); got != "/chat" {
			t.Errorf("Expected '/chat', got %q", got)
		}
	})

	t.Run("Type", func(t *testing.T) {
		if got := rawMsg.Type(); got != "1" {
			t.Errorf("Expected '1', got %q", got)
		}
	})

	t.Run("Data", func(t *testing.T) {
		if got := rawMsg.Data(); got != `{"key":"value"}` {
			t.Errorf("Expected JSON data, got %q", got)
		}
	})
}

func TestRawClusterMessage_EmptyValues(t *testing.T) {
	rawMsg := RawClusterMessage{}

	if rawMsg.Uid() != "" {
		t.Error("Expected empty Uid")
	}
	if rawMsg.Nsp() != "" {
		t.Error("Expected empty Nsp")
	}
	if rawMsg.Type() != "" {
		t.Error("Expected empty Type")
	}
	if rawMsg.Data() != "" {
		t.Error("Expected empty Data")
	}
}

func TestRawClusterMessage_WrongType(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  123,     // Not a string
		"nsp":  true,    // Not a string
		"type": 1,       // Not a string
		"data": []int{}, // Not a string
	}

	// All getters should return empty string for wrong types
	if rawMsg.Uid() != "" {
		t.Error("Expected empty string for wrong Uid type")
	}
	if rawMsg.Nsp() != "" {
		t.Error("Expected empty string for wrong Nsp type")
	}
	if rawMsg.Type() != "" {
		t.Error("Expected empty string for wrong Type type")
	}
	if rawMsg.Data() != "" {
		t.Error("Expected empty string for wrong Data type")
	}
}

func TestNextOffset(t *testing.T) {
	a := &redisStreamsAdapter{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal offset",
			input:    "1234567890-0",
			expected: "1234567890-1",
		},
		{
			name:     "increment sequence",
			input:    "1234567890-99",
			expected: "1234567890-100",
		},
		{
			name:     "large timestamp",
			input:    "1749618000000-5",
			expected: "1749618000000-6",
		},
		{
			name:     "zero sequence",
			input:    "1000000000000-0",
			expected: "1000000000000-1",
		},
		{
			name:     "no dash returns original",
			input:    "invalid",
			expected: "invalid",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only dash",
			input:    "-",
			expected: "-", // sequence part is empty, parsing will fail, return original
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := a.nextOffset(tt.input)
			if result != tt.expected {
				t.Errorf("nextOffset(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShouldIncludePacket(t *testing.T) {
	a := &redisStreamsAdapter{}

	t.Run("include when no rooms specified", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{},
			Except: []socket.Room{},
		}
		if !a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected true when no rooms specified")
		}
	})

	t.Run("include when session is in target room", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		sessionRooms.Add("room2")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{"room2"},
			Except: []socket.Room{},
		}
		if !a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected true when session is in target room")
		}
	})

	t.Run("exclude when session not in target room", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{"room2", "room3"},
			Except: []socket.Room{},
		}
		if a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected false when session is not in target rooms")
		}
	})

	t.Run("exclude when session is in except list", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{},
			Except: []socket.Room{"room1"},
		}
		if a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected false when session is in except list")
		}
	})

	t.Run("exclude takes priority over include", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		sessionRooms.Add("room2")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{"room1"},
			Except: []socket.Room{"room2"},
		}
		if a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected false when session is in both target and except")
		}
	})

	t.Run("include when session is in target but not in except", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{"room1"},
			Except: []socket.Room{"room2"},
		}
		if !a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected true when session is in target but not in except")
		}
	})
}

func TestEncode(t *testing.T) {
	t.Run("encode message without data", func(t *testing.T) {
		msg := &adapter.ClusterResponse{
			Uid:  "server-1",
			Nsp:  "/",
			Type: adapter.INITIAL_HEARTBEAT,
			Data: nil,
		}

		raw, err := rediswire.EncodeStreamMessage(msg, false)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if raw.Uid() != "server-1" {
			t.Errorf("Expected uid 'server-1', got %q", raw.Uid())
		}
		if raw.Nsp() != "/" {
			t.Errorf("Expected nsp '/', got %q", raw.Nsp())
		}
		if raw.Type() != "1" {
			t.Errorf("Expected type '1', got %q", raw.Type())
		}
		if raw.Data() != "" {
			t.Errorf("Expected empty data, got %q", raw.Data())
		}
	})

	t.Run("encode JSON data", func(t *testing.T) {
		testData := &adapter.FetchSocketsMessage{
			RequestId: "req-1",
			Opts: &adapter.PacketOptions{
				Rooms: []socket.Room{"room1"},
			},
		}
		msg := &adapter.ClusterResponse{
			Uid:  "server-1",
			Nsp:  "/test",
			Type: adapter.FETCH_SOCKETS,
			Data: testData,
		}

		raw, err := rediswire.EncodeStreamMessage(msg, false)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Data should be JSON encoded
		data := raw.Data()
		if data == "" {
			t.Fatal("Expected non-empty data")
		}
		if data[0] != '{' {
			t.Error("Expected JSON format (starting with '{')")
		}
	})

	t.Run("propagate encoding errors", func(t *testing.T) {
		_, err := rediswire.EncodeStreamMessage(&adapter.ClusterMessage{
			Uid:  "server-1",
			Nsp:  "/",
			Type: adapter.MessageType(999),
			Data: func() {},
		}, false)
		if err == nil {
			t.Fatal("Expected encoding error")
		}
	})
}

func TestDefaultStreamName(t *testing.T) {
	if DefaultStreamName != "socket.io" {
		t.Errorf("Expected 'socket.io', got %q", DefaultStreamName)
	}
}

func TestDefaultSessionKeyPrefix(t *testing.T) {
	if DefaultSessionKeyPrefix != "sio:session:" {
		t.Errorf("Expected 'sio:session:', got %q", DefaultSessionKeyPrefix)
	}
}

func TestDefaultStreamReadCount(t *testing.T) {
	if DefaultStreamReadCount != 100 {
		t.Errorf("Expected 100, got %d", DefaultStreamReadCount)
	}
}

func TestOffsetRegex(t *testing.T) {
	validOffsets := []string{
		"0-0",
		"1234567890123-0",
		"1749618000000-999",
		"0-1",
	}

	invalidOffsets := []string{
		"",
		"invalid",
		"1234567890123",
		"-1",
		"1234567890123-",
		"abc-123",
		"123-abc",
		"$",
		"*",
	}

	for _, offset := range validOffsets {
		t.Run("valid: "+offset, func(t *testing.T) {
			if !offsetRegex.MatchString(offset) {
				t.Errorf("Expected %q to be valid", offset)
			}
		})
	}

	for _, offset := range invalidOffsets {
		t.Run("invalid: "+offset, func(t *testing.T) {
			if offsetRegex.MatchString(offset) {
				t.Errorf("Expected %q to be invalid", offset)
			}
		})
	}
}

func TestDecode_JSONData(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "7", // FETCH_SOCKETS
		"data": `{"requestId":"req-1"}`,
	}

	result, err := rediswire.DecodeStreamMessage(rawMsg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Uid != "server-1" {
		t.Errorf("Expected uid 'server-1', got %q", result.Uid)
	}
	if result.Nsp != "/" {
		t.Errorf("Expected nsp '/', got %q", result.Nsp)
	}
	if result.Type != adapter.FETCH_SOCKETS {
		t.Errorf("Expected FETCH_SOCKETS type, got %v", result.Type)
	}
}

func TestDecode_Base64MsgpackData(t *testing.T) {
	// Create base64-encoded MessagePack data
	testData := &adapter.FetchSocketsMessage{
		RequestId: "base64-req",
	}
	encoded, _ := msgpack.Marshal(testData)
	base64Data := base64.StdEncoding.EncodeToString(encoded)

	rawMsg := RawClusterMessage{
		"uid":  "server-2",
		"nsp":  "/chat",
		"type": "7", // FETCH_SOCKETS
		"data": base64Data,
	}

	result, err := rediswire.DecodeStreamMessage(rawMsg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Uid != "server-2" {
		t.Errorf("Expected uid 'server-2', got %q", result.Uid)
	}
	if result.Type != adapter.FETCH_SOCKETS {
		t.Errorf("Expected FETCH_SOCKETS type, got %v", result.Type)
	}
}

func TestDecode_InvalidType(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "invalid",
	}

	_, err := rediswire.DecodeStreamMessage(rawMsg)
	if err == nil {
		t.Error("Expected error for invalid type")
	}
}

func TestDecode_NoData(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "0", // INITIAL_HEARTBEAT
	}

	result, err := rediswire.DecodeStreamMessage(rawMsg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Data != nil {
		t.Errorf("Expected nil data, got %v", result.Data)
	}
}

func TestDecode_InvalidBase64(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "4",
		"data": "not-valid-base64!!!",
	}

	_, err := rediswire.DecodeStreamMessage(rawMsg)
	if err == nil {
		t.Error("Expected error for invalid base64")
	}
}

func TestDecodePubSubMessage(t *testing.T) {
	message := &adapter.ClusterMessage{
		Uid:  "server-1",
		Nsp:  "/chat",
		Type: adapter.FETCH_SOCKETS,
		Data: &adapter.FetchSocketsMessage{
			RequestId: "request-1",
			Opts:      &adapter.PacketOptions{},
		},
	}
	payload, err := rediswire.EncodeClusterMessageMsgpack(message)
	if err != nil {
		t.Fatalf("Failed to encode message: %v", err)
	}

	decoded, err := rediswire.UnmarshalClusterMessage(payload)
	if err != nil {
		t.Fatalf("Failed to decode message: %v", err)
	}
	data, ok := decoded.Data.(*adapter.FetchSocketsMessage)
	if !ok {
		t.Fatalf("Expected *FetchSocketsMessage, got %T", decoded.Data)
	}
	if data.RequestId != "request-1" {
		t.Fatalf("Expected request-1, got %q", data.RequestId)
	}
}

func TestHashCode(t *testing.T) {
	tests := []struct {
		input    string
		expected int32
	}{
		{"/", 47},
		{"/namespace-0", -1732195153},
		{"/😀", 1818066},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := hashCode(tt.input)
			if result != tt.expected {
				t.Errorf("hashCode(%q) = %d, want %d", tt.input, result, tt.expected)
			}
			// Ensure deterministic
			if hashCode(tt.input) != result {
				t.Error("hashCode is not deterministic")
			}
		})
	}
}

func TestComputeStreamName(t *testing.T) {
	t.Run("single stream", func(t *testing.T) {
		opts := DefaultRedisStreamsAdapterOptions()
		opts.SetStreamName("socket.io")
		opts.SetStreamCount(1)

		result := computeStreamName("/chat", opts)
		if result != "socket.io" {
			t.Errorf("Expected 'socket.io', got %q", result)
		}
	})

	t.Run("multiple streams", func(t *testing.T) {
		opts := DefaultRedisStreamsAdapterOptions()
		opts.SetStreamName("socket.io")
		opts.SetStreamCount(4)

		result := computeStreamName("/chat", opts)
		expected := "socket.io-" + strconv.FormatInt(int64(hashCode("/chat"))%4, 10)
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})

	t.Run("negative hash", func(t *testing.T) {
		opts := DefaultRedisStreamsAdapterOptions()
		opts.SetStreamName("socket.io")
		opts.SetStreamCount(5)

		if result := computeStreamName("/namespace-0", opts); result != "socket.io--3" {
			t.Errorf("Expected 'socket.io--3', got %q", result)
		}
	})
}

func TestIsEphemeral(t *testing.T) {
	t.Run("broadcast without requestId is not ephemeral", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.BROADCAST,
			Data: &adapter.BroadcastMessage{},
		}
		if isEphemeral(msg) {
			t.Error("Expected false for broadcast without requestId")
		}
	})

	t.Run("broadcast with requestId is ephemeral", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.BROADCAST,
			Data: &adapter.BroadcastMessage{RequestId: new("req-1")},
		}
		if !isEphemeral(msg) {
			t.Error("Expected true for broadcast with requestId")
		}
	})

	t.Run("SERVER_SIDE_EMIT is ephemeral", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.SERVER_SIDE_EMIT,
		}
		if !isEphemeral(msg) {
			t.Error("Expected true for SERVER_SIDE_EMIT")
		}
	})

	t.Run("FETCH_SOCKETS is ephemeral", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.FETCH_SOCKETS,
		}
		if !isEphemeral(msg) {
			t.Error("Expected true for FETCH_SOCKETS")
		}
	})

	t.Run("SOCKETS_JOIN is not ephemeral", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.SOCKETS_JOIN,
		}
		if isEphemeral(msg) {
			t.Error("Expected false for SOCKETS_JOIN")
		}
	})
}
