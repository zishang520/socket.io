package adapter

import (
	"context"
	"encoding/base64"
	"errors"
	"math"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

type xrangeTailAppender struct {
	client *rds.Client
	stream string
	id     string
	values map[string]any
	once   sync.Once
	err    error
}

type xrangeNonEmptyHook struct {
	mu    sync.Mutex
	calls int
}

type redisStreamsClusterResponseAdapter struct {
	adapter.ClusterAdapter
	local *clusterResponseAdapter
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

func (*xrangeTailAppender) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *xrangeTailAppender) ProcessHook(next rds.ProcessHook) rds.ProcessHook {
	return func(ctx context.Context, cmd rds.Cmder) error {
		if err := next(ctx, cmd); err != nil {
			return err
		}
		if cmd.Name() == "xrange" {
			h.once.Do(func() {
				h.err = h.client.XAdd(ctx, &rds.XAddArgs{Stream: h.stream, ID: h.id, Values: h.values}).Err()
			})
		}
		return h.err
	}
}

func (*xrangeTailAppender) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func (*xrangeNonEmptyHook) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *xrangeNonEmptyHook) ProcessHook(next rds.ProcessHook) rds.ProcessHook {
	return func(ctx context.Context, cmd rds.Cmder) error {
		if cmd.Name() != "xrange" {
			return next(ctx, cmd)
		}

		xrangeCmd, ok := cmd.(*rds.XMessageSliceCmd)
		if !ok {
			return next(ctx, cmd)
		}
		h.mu.Lock()
		h.calls++
		id := strconv.Itoa(h.calls) + "-0"
		h.mu.Unlock()
		xrangeCmd.SetVal([]rds.XMessage{{ID: id, Values: map[string]any{"nsp": "/other"}}})
		return nil
	}
}

func (*xrangeNonEmptyHook) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func (a *redisStreamsClusterResponseAdapter) OnMessage(message *adapter.ClusterMessage, offset adapter.Offset) {
	if message.Uid == a.Uid() || message.Nsp != a.Nsp().Name() {
		a.ClusterAdapter.OnMessage(message, offset)
		return
	}

	switch message.Type {
	case adapter.FETCH_SOCKETS:
		data, ok := message.Data.(*adapter.FetchSocketsMessage)
		if ok {
			a.PublishResponse(message.Uid, &adapter.ClusterResponse{
				Type: adapter.FETCH_SOCKETS_RESPONSE,
				Data: &adapter.FetchSocketsResponse{
					RequestId: data.RequestId,
					Sockets:   adapter.SocketDetailsToResponses(a.local.sockets),
				},
			})
			return
		}
	case adapter.BROADCAST:
		data, ok := message.Data.(*adapter.BroadcastMessage)
		if ok && data.RequestId != nil {
			a.PublishResponse(message.Uid, &adapter.ClusterResponse{
				Type: adapter.BROADCAST_CLIENT_COUNT,
				Data: &adapter.BroadcastClientCount{
					RequestId:   *data.RequestId,
					ClientCount: a.local.clientCount,
				},
			})
			var packet any
			if len(a.local.ack) != 0 {
				packet = a.local.ack[0]
			}
			a.PublishResponse(message.Uid, &adapter.ClusterResponse{
				Type: adapter.BROADCAST_ACK,
				Data: &adapter.BroadcastAck{RequestId: *data.RequestId, Packet: packet},
			})
			return
		}
	}

	a.ClusterAdapter.OnMessage(message, offset)
}

func newRedisStreamsTestNode(t *testing.T, addr string, local *clusterResponseAdapter) *redisStreamsAdapter {
	t.Helper()
	client := rds.NewClient(&rds.Options{Addr: addr})
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	current := NewRedisStreamsAdapter(nsp, mustRedisClient(t, t.Context(), client), nil).(*redisStreamsAdapter)
	if local != nil {
		current.ClusterAdapter = &redisStreamsClusterResponseAdapter{
			ClusterAdapter: current.ClusterAdapter,
			local:          local,
		}
	}
	t.Cleanup(func() {
		current.Close()
		_ = client.Close()
	})
	return current
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

func TestRedisStreamsAdapterConstructAppliesDefaults(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	streamAdapter.SetRedis(mustRedisClient(t, context.Background(), client))
	streamAdapter.Construct(socket.NewNamespace(socket.NewServer(nil, nil), "/construct"))
	t.Cleanup(streamAdapter.Close)

	if streamAdapter.streamName != DefaultStreamName {
		t.Fatalf("stream name = %q, want %q", streamAdapter.streamName, DefaultStreamName)
	}
	if streamAdapter.publicChannel != DefaultChannelPrefix+"#/construct#" {
		t.Fatalf("public channel = %q", streamAdapter.publicChannel)
	}
	if streamAdapter.opts.MaxLen() != DefaultStreamMaxLen ||
		streamAdapter.opts.ReadCount() != DefaultStreamReadCount ||
		streamAdapter.opts.BlockTimeInMs() != DefaultBlockTimeInMs {
		t.Fatalf("defaults were not applied: %+v", streamAdapter.opts)
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
	opts.SetStreamCount(2)
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

	if streamAdapter.opts.StreamName() != "" || streamAdapter.opts.StreamCount() != 2 ||
		streamAdapter.opts.ChannelPrefix() != "" || streamAdapter.opts.MaxLen() != 0 ||
		streamAdapter.opts.ReadCount() != 0 || streamAdapter.opts.BlockTimeInMs() != 0 ||
		streamAdapter.opts.SessionKeyPrefix() != "" {
		t.Fatalf("explicit options were replaced: %+v", streamAdapter.opts)
	}
}

func TestRedisStreamsAdapterRoutesNonPositiveStreamCountToBaseStream(t *testing.T) {
	for _, tt := range []struct {
		name string
		raw  int
	}{
		{name: "zero", raw: 0},
		{name: "negative", raw: -2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := rds.NewClient(&rds.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			redisClient := mustRedisClient(t, context.Background(), client)
			redisErrors := make(chan error, 1)
			_ = redisClient.On("error", func(args ...any) {
				if len(args) > 0 {
					if err, ok := args[0].(error); ok {
						select {
						case redisErrors <- err:
						default:
						}
					}
				}
			})

			opts := DefaultRedisStreamsAdapterOptions()
			opts.SetStreamCount(tt.raw)
			streamAdapter := NewRedisStreamsAdapter(
				socket.NewNamespace(socket.NewServer(nil, nil), "/"+tt.name+"-stream-count"),
				redisClient,
				opts,
			).(*redisStreamsAdapter)
			t.Cleanup(streamAdapter.Close)

			if got := streamAdapter.opts.StreamCount(); got != tt.raw {
				t.Fatalf("stream count = %d, want raw value %d", got, tt.raw)
			}
			if raw := streamAdapter.opts.GetRawStreamCount(); raw == nil || raw.Get() != tt.raw {
				t.Fatalf("raw stream count = %v, want %d", raw, tt.raw)
			}
			if streamAdapter.streamName != DefaultStreamName {
				t.Fatalf("routed stream = %q, want base stream %q", streamAdapter.streamName, DefaultStreamName)
			}
			select {
			case err := <-redisErrors:
				t.Fatalf("non-positive stream count emitted an error: %v", err)
			default:
			}
		})
	}
}

func TestRedisStreamsFetchSocketsAcrossNodes(t *testing.T) {
	server := miniredis.RunT(t)
	first := newRedisStreamsTestNode(t, server.Addr(), nil)
	second := newRedisStreamsTestNode(t, server.Addr(), &clusterResponseAdapter{
		sockets: []socket.SocketDetails{adapter.NewRemoteSocket(&adapter.SocketResponse{Id: "second"})},
	})
	waitForRedisPubSub(t, func() bool {
		firstCount, firstErr := first.ServerCount()
		secondCount, secondErr := second.ServerCount()
		return firstErr == nil && secondErr == nil && firstCount == 2 && secondCount == 2
	})

	type result struct {
		sockets []socket.SocketDetails
		err     error
	}
	results := make(chan result, 1)
	var callbackCount atomic.Int64
	first.FetchSockets(nil)(func(sockets []socket.SocketDetails, err error) {
		callbackCount.Add(1)
		results <- result{sockets: sockets, err: err}
	})

	select {
	case result := <-results:
		if result.err != nil {
			t.Fatal(result.err)
		}
		ids := types.NewSet[socket.SocketId]()
		for _, details := range result.sockets {
			ids.Add(details.Id())
		}
		if len(result.sockets) != 1 || !ids.Has("second") {
			t.Fatalf("fetched sockets = %#v", ids.Keys())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for cross-node FetchSockets response")
	}
	if callbackCount.Load() != 1 {
		t.Fatalf("callback count = %d, want 1", callbackCount.Load())
	}
}

func TestRedisStreamsBroadcastWithAckAcrossNodes(t *testing.T) {
	server := miniredis.RunT(t)
	first := newRedisStreamsTestNode(t, server.Addr(), nil)
	second := newRedisStreamsTestNode(t, server.Addr(), &clusterResponseAdapter{
		clientCount: 2,
		ack:         []any{"second"},
	})
	waitForRedisPubSub(t, func() bool {
		firstCount, firstErr := first.ServerCount()
		secondCount, secondErr := second.ServerCount()
		return firstErr == nil && secondErr == nil && firstCount == 2 && secondCount == 2
	})

	timeout := int64(3_000)
	counts := make(chan uint64, 2)
	acks := make(chan []any, 1)
	first.BroadcastWithAck(
		&parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
		&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: &timeout}},
		func(count uint64) { counts <- count },
		func(args []any, _ error) { acks <- args },
	)

	seenCounts := types.NewSet[uint64]()
	for range 2 {
		select {
		case count := <-counts:
			seenCounts.Add(count)
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for cross-node client count")
		}
	}
	if !seenCounts.Has(0) || !seenCounts.Has(2) {
		t.Fatalf("client counts = %v, want [0 2]", seenCounts.Keys())
	}
	select {
	case args := <-acks:
		if len(args) != 1 || args[0] != "second" {
			t.Fatalf("acknowledgement = %#v, want [second]", args)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for cross-node acknowledgement")
	}
}

func TestRedisStreamsAdapterCloseStopsOwnedRedisOperations(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	redisClient := mustRedisClient(t, t.Context(), client)
	var errorEvents atomic.Int64
	_ = redisClient.On("error", func(...any) { errorEvents.Add(1) })

	recovery := socket.DefaultConnectionStateRecovery()
	recovery.SetMaxDisconnectionDuration(1_000)
	serverOpts := socket.DefaultServerOptions()
	serverOpts.SetConnectionStateRecovery(recovery)
	current := NewRedisStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, serverOpts), "/test"),
		redisClient,
		nil,
	).(*redisStreamsAdapter)

	restoreKey := DefaultSessionKeyPrefix + "restore"
	if err := client.Set(t.Context(), restoreKey, "session", time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	current.Close()

	_, err := current.PublishAndReturnOffset(&adapter.ClusterMessage{
		Type: adapter.SOCKETS_JOIN,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts:  adapter.EncodeOptions(nil),
			Rooms: []socket.Room{"room"},
		},
	})
	if !errors.Is(err, adapter.ErrAdapterClosed) {
		t.Fatalf("durable publish error = %v, want ErrAdapterClosed", err)
	}
	if length, err := client.XLen(t.Context(), current.streamName).Result(); err != nil || length != 0 {
		t.Fatalf("stream length/error after Close = %d/%v, want 0/nil", length, err)
	}

	current.PersistSession(&socket.SessionToPersist{Pid: "persist"})
	if exists, err := client.Exists(t.Context(), DefaultSessionKeyPrefix+"persist").Result(); err != nil || exists != 0 {
		t.Fatalf("persisted session count/error after Close = %d/%v, want 0/nil", exists, err)
	}

	if _, err := current.RestoreSession("restore", "1-0"); !errors.Is(err, context.Canceled) {
		t.Fatalf("restore error after Close = %v, want context.Canceled", err)
	}
	if exists, err := client.Exists(t.Context(), restoreKey).Result(); err != nil || exists != 1 {
		t.Fatalf("restore session count/error after Close = %d/%v, want 1/nil", exists, err)
	}
	if errorEvents.Load() != 0 {
		t.Fatalf("Redis error events after Close = %d, want 0", errorEvents.Load())
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
	streamAdapter.ctx = redisClient.Context()
	streamAdapter.streamName = "stream"
	streamAdapter.opts.SetSessionKeyPrefix(DefaultSessionKeyPrefix)

	session := &socket.SessionToPersist{
		Sid: "sid",
		Pid: "pid",
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
	if restored.Rooms == nil || restored.Rooms.Len() != 0 {
		t.Fatalf("rooms = %#v, want non-nil empty set", restored.Rooms)
	}
}

func newRedisStreamsPersistenceAdapter(t *testing.T, duration int64) (*redisStreamsAdapter, *rds.Client) {
	t.Helper()
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	recovery := socket.DefaultConnectionStateRecovery()
	recovery.SetMaxDisconnectionDuration(duration)
	serverOpts := socket.DefaultServerOptions()
	serverOpts.SetConnectionStateRecovery(recovery)

	streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	streamAdapter.ClusterAdapter.Construct(socket.NewNamespace(socket.NewServer(nil, serverOpts), "/test"))
	streamAdapter.redisClient = mustRedisClient(t, context.Background(), client)
	streamAdapter.ctx = streamAdapter.redisClient.Context()
	streamAdapter.opts.SetSessionKeyPrefix(DefaultSessionKeyPrefix)
	return streamAdapter, client
}

func TestPersistSessionSetsPositiveTTL(t *testing.T) {
	const duration = int64(1_500)
	streamAdapter, client := newRedisStreamsPersistenceAdapter(t, duration)
	streamAdapter.PersistSession(&socket.SessionToPersist{Pid: "pid"})

	ttl, err := client.PTTL(context.Background(), DefaultSessionKeyPrefix+"pid").Result()
	if err != nil {
		t.Fatal(err)
	}
	if ttl <= 0 || ttl > time.Duration(duration)*time.Millisecond {
		t.Fatalf("session TTL = %v, want within (0, %v]", ttl, time.Duration(duration)*time.Millisecond)
	}
}

func TestPersistSessionReportsEncodingError(t *testing.T) {
	streamAdapter, client := newRedisStreamsPersistenceAdapter(t, 1_000)
	errors := make(chan error, 1)
	_ = streamAdapter.redisClient.On("error", func(args ...any) {
		if len(args) > 0 {
			if err, ok := args[0].(error); ok {
				errors <- err
			}
		}
	})

	streamAdapter.PersistSession(&socket.SessionToPersist{Pid: "pid", Data: make(chan struct{})})
	select {
	case err := <-errors:
		if !strings.Contains(err.Error(), "failed to encode session") {
			t.Fatalf("encoding error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("session encoding failure did not emit an error")
	}
	if exists, err := client.Exists(context.Background(), DefaultSessionKeyPrefix+"pid").Result(); err != nil || exists != 0 {
		t.Fatalf("encoded session count/error = %d/%v, want 0/nil", exists, err)
	}
}

func TestPersistSessionRejectsInvalidDuration(t *testing.T) {
	for _, tt := range []struct {
		duration  int64
		wantError string
	}{
		{duration: 0, wantError: "must be positive"},
		{duration: -1, wantError: "must be positive"},
		{duration: math.MaxInt64, wantError: "overflows time.Duration"},
	} {
		t.Run(strconv.FormatInt(tt.duration, 10), func(t *testing.T) {
			streamAdapter, client := newRedisStreamsPersistenceAdapter(t, tt.duration)
			errors := make(chan error, 1)
			_ = streamAdapter.redisClient.On("error", func(args ...any) {
				if len(args) > 0 {
					if err, ok := args[0].(error); ok {
						errors <- err
					}
				}
			})
			streamAdapter.PersistSession(&socket.SessionToPersist{Pid: "pid"})

			exists, err := client.Exists(context.Background(), DefaultSessionKeyPrefix+"pid").Result()
			if err != nil {
				t.Fatal(err)
			}
			if exists != 0 {
				t.Fatal("session with invalid recovery duration was persisted")
			}
			select {
			case err := <-errors:
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("invalid duration error = %v, want %q", err, tt.wantError)
				}
			case <-time.After(100 * time.Millisecond):
				t.Fatal("invalid recovery duration did not emit an error")
			}
		})
	}
}

func TestRestoreSessionRejectsMalformedPersistedSession(t *testing.T) {
	tests := []struct {
		name      string
		session   any
		wantError string
	}{
		{
			name:      "missing session data",
			session:   (*socket.SessionToPersist)(nil),
			wantError: "invalid persisted session: missing session data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := rds.NewClient(&rds.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			ctx := context.Background()
			redisClient := mustRedisClient(t, ctx, client)

			payload, err := utils.MsgPack().Encode(tt.session)
			if err != nil {
				t.Fatal(err)
			}
			if err = client.Set(
				ctx,
				DefaultSessionKeyPrefix+"pid",
				base64.StdEncoding.EncodeToString(payload),
				0,
			).Err(); err != nil {
				t.Fatal(err)
			}
			if err = client.XAdd(ctx, &rds.XAddArgs{
				Stream: "stream",
				ID:     "1-0",
				Values: map[string]any{"uid": "node", "nsp": "/test", "type": "1"},
			}).Err(); err != nil {
				t.Fatal(err)
			}

			streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
			streamAdapter.ClusterAdapter.Construct(
				socket.NewNamespace(socket.NewServer(nil, nil), "/test"),
			)
			streamAdapter.redisClient = redisClient
			streamAdapter.ctx = ctx
			streamAdapter.streamName = "stream"
			streamAdapter.opts.SetSessionKeyPrefix(DefaultSessionKeyPrefix)

			if _, err = streamAdapter.RestoreSession("pid", "1-0"); err == nil || err.Error() != tt.wantError {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
		})
	}
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
	streamAdapter.ctx = ctx
	streamAdapter.streamName = DefaultStreamName

	if err := streamAdapter.collectMissedPackets(client, &socket.Session{}, "0-0"); err != nil {
		t.Fatal(err)
	}

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.args) != 3 {
		t.Fatalf("XRANGE calls = %d, want 3", len(recorder.args))
	}
	for _, args := range recorder.args {
		if len(args) < 6 || args[len(args)-2] != "count" || args[len(args)-1] != int64(restoreSessionPageSize) {
			t.Fatalf("XRANGE args = %#v, want COUNT %d", args, restoreSessionPageSize)
		}
	}
}

func TestCollectMissedPacketsReadsEntriesAppendedAfterShortPage(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	ctx := context.Background()

	if err := client.XAdd(ctx, &rds.XAddArgs{
		Stream: "stream",
		ID:     "1-0",
		Values: map[string]any{"nsp": "/other"},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	wireMessage, err := rediswire.EncodeStreamMessage(&adapter.ClusterMessage{
		Uid:  "remote",
		Nsp:  "/test",
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"late"}},
			Opts:   new(adapter.PacketOptions),
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	client.AddHook(&xrangeTailAppender{
		client: client,
		stream: "stream",
		id:     "2-0",
		values: map[string]any(wireMessage),
	})

	streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	streamAdapter.ClusterAdapter.Construct(socket.NewNamespace(socket.NewServer(nil, nil), "/test"))
	streamAdapter.redisClient = mustRedisClient(t, ctx, client)
	streamAdapter.ctx = ctx
	streamAdapter.streamName = "stream"
	session := &socket.Session{
		SessionToPersist: &socket.SessionToPersist{Rooms: types.NewSet[socket.Room]()},
		MissedPackets:    []any{},
	}

	if err := streamAdapter.collectMissedPackets(client, session, "0-0"); err != nil {
		t.Fatal(err)
	}
	want := []any{[]any{"late", "2-0"}}
	if !reflect.DeepEqual(session.MissedPackets, want) {
		t.Fatalf("missed packets = %#v, want %#v", session.MissedPackets, want)
	}
}

func TestCollectMissedPacketsReturnsErrorAtReadLimit(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	hook := &xrangeNonEmptyHook{}
	client.AddHook(hook)
	ctx := context.Background()

	streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	streamAdapter.ClusterAdapter.Construct(socket.NewNamespace(socket.NewServer(nil, nil), "/test"))
	streamAdapter.redisClient = mustRedisClient(t, ctx, client)
	streamAdapter.ctx = ctx
	streamAdapter.streamName = "stream"

	err := streamAdapter.collectMissedPackets(client, &socket.Session{}, "0-0")
	if !errors.Is(err, errRestoreSessionReadLimit) {
		t.Fatalf("error = %v, want %v", err, errRestoreSessionReadLimit)
	}
	hook.mu.Lock()
	calls := hook.calls
	hook.mu.Unlock()
	if calls != restoreSessionMaxXRangeCalls {
		t.Fatalf("XRANGE calls = %d, want %d", calls, restoreSessionMaxXRangeCalls)
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
	streamAdapter.ctx = ctx
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

func TestCollectMissedPacketsValidatesIncludedPacketData(t *testing.T) {
	tests := []struct {
		name       string
		packet     *parser.Packet
		opts       *adapter.PacketOptions
		rawData    string
		wantErr    string
		wantPacket []any
	}{
		{
			name:    "missing packet",
			opts:    new(adapter.PacketOptions),
			wantErr: "invalid broadcast message",
		},
		{
			name:    "nil data",
			packet:  &parser.Packet{Type: parser.EVENT},
			opts:    new(adapter.PacketOptions),
			wantErr: "invalid broadcast packet data",
		},
		{
			name:    "non-array data",
			packet:  &parser.Packet{Type: parser.EVENT, Data: "event"},
			opts:    new(adapter.PacketOptions),
			wantErr: "invalid broadcast packet data",
		},
		{
			name:       "empty array",
			packet:     &parser.Packet{Type: parser.EVENT, Data: []any{}},
			opts:       new(adapter.PacketOptions),
			wantPacket: []any{"1-0"},
		},
		{
			name:       "event array",
			packet:     &parser.Packet{Type: parser.EVENT, Data: []any{"event", "value"}},
			opts:       new(adapter.PacketOptions),
			wantPacket: []any{"event", "value", "1-0"},
		},
		{
			name:    "missing rooms",
			rawData: `{"packet":{"type":2,"data":["event"]},"opts":{"except":[]}}`,
			wantErr: "invalid broadcast options: rooms and except are required",
		},
		{
			name:    "missing except",
			rawData: `{"packet":{"type":2,"data":["event"]},"opts":{"rooms":[]}}`,
			wantErr: "invalid broadcast options: rooms and except are required",
		},
		{
			name:       "missing optional flags",
			rawData:    `{"packet":{"type":2,"data":["event"]},"opts":{"rooms":[],"except":[]}}`,
			wantPacket: []any{"event", "1-0"},
		},
		{
			name:    "non-event with malformed options",
			rawData: `{"packet":{"type":0},"opts":{"except":[]}}`,
		},
		{
			name:    "event with id and malformed options",
			rawData: `{"packet":{"type":2,"id":1,"data":["event"]},"opts":{"rooms":[]}}`,
		},
		{
			name:    "volatile event with malformed options",
			rawData: `{"packet":{"type":2,"data":["event"]},"opts":{"flags":{"volatile":true}}}`,
		},
		{
			name:   "excluded malformed data",
			packet: &parser.Packet{Type: parser.EVENT, Data: "event"},
			opts:   &adapter.PacketOptions{Rooms: []socket.Room{"other"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := rds.NewClient(&rds.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			ctx := context.Background()

			message, err := rediswire.EncodeStreamMessage(&adapter.ClusterMessage{
				Uid:  "remote",
				Nsp:  "/test",
				Type: adapter.BROADCAST,
				Data: &adapter.BroadcastMessage{Packet: tt.packet, Opts: tt.opts},
			}, false)
			if err != nil {
				t.Fatal(err)
			}
			if tt.rawData != "" {
				message["data"] = tt.rawData
			}
			if err = client.XAdd(ctx, &rds.XAddArgs{
				Stream: "stream",
				ID:     "1-0",
				Values: map[string]any(message),
			}).Err(); err != nil {
				t.Fatal(err)
			}

			streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
			streamAdapter.ClusterAdapter.Construct(socket.NewNamespace(socket.NewServer(nil, nil), "/test"))
			streamAdapter.redisClient = mustRedisClient(t, ctx, client)
			streamAdapter.ctx = ctx
			streamAdapter.streamName = "stream"
			session := &socket.Session{
				SessionToPersist: &socket.SessionToPersist{Rooms: types.NewSet(socket.Room("room"))},
				MissedPackets:    []any{},
			}

			err = streamAdapter.collectMissedPackets(client, session, "0-0")
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("error = %v, want %q", err, tt.wantErr)
				}
				if len(session.MissedPackets) != 0 {
					t.Fatalf("missed packets = %#v, want none", session.MissedPackets)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tt.wantPacket == nil {
				if len(session.MissedPackets) != 0 {
					t.Fatalf("missed packets = %#v, want none", session.MissedPackets)
				}
				return
			}
			want := []any{tt.wantPacket}
			if !reflect.DeepEqual(session.MissedPackets, want) {
				t.Fatalf("missed packets = %#v, want %#v", session.MissedPackets, want)
			}
		})
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
	streamAdapter.ctx = ctx
	streamAdapter.streamName = "stream"
	streamAdapter.opts.SetSessionKeyPrefix(DefaultSessionKeyPrefix)

	session := &socket.SessionToPersist{
		Sid:   "sid",
		Pid:   "pid",
		Rooms: types.NewSet(socket.Room("sid")),
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
	payload, err := adapter.EncodeClusterMessageMsgpack(message)
	if err != nil {
		t.Fatalf("Failed to encode message: %v", err)
	}

	decoded, err := adapter.DecodeClusterMessage(payload)
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

func TestComputeStreamName(t *testing.T) {
	t.Run("single stream", func(t *testing.T) {
		opts := DefaultRedisStreamsAdapterOptions()
		opts.SetStreamName("socket.io")
		opts.SetStreamCount(1)

		result := rediswire.StreamNameForNamespace(opts.StreamName(), "/chat", opts.StreamCount())
		if result != "socket.io" {
			t.Errorf("Expected 'socket.io', got %q", result)
		}
	})

	t.Run("multiple streams", func(t *testing.T) {
		opts := DefaultRedisStreamsAdapterOptions()
		opts.SetStreamName("socket.io")
		opts.SetStreamCount(4)

		result := rediswire.StreamNameForNamespace(opts.StreamName(), "/chat", opts.StreamCount())
		expected := "socket.io-3"
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})

	t.Run("negative hash", func(t *testing.T) {
		opts := DefaultRedisStreamsAdapterOptions()
		opts.SetStreamName("socket.io")
		opts.SetStreamCount(5)

		if result := rediswire.StreamNameForNamespace(opts.StreamName(), "/namespace-0", opts.StreamCount()); result != "socket.io--3" {
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
