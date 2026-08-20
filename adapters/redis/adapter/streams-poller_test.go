package adapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	clusteradapter "github.com/zishang520/socket.io/adapters/adapter/v3"
	rediswire "github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type pollerXReadCall struct {
	stream string
	id     string
	block  int64
}

type blockingPollerXReadHook struct {
	calls chan pollerXReadCall
}

func (*blockingPollerXReadHook) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *blockingPollerXReadHook) ProcessHook(next rds.ProcessHook) rds.ProcessHook {
	return func(ctx context.Context, cmd rds.Cmder) error {
		if cmd.Name() != "xread" {
			return next(ctx, cmd)
		}

		args := cmd.Args()
		call := pollerXReadCall{}
		if len(args) >= 3 {
			call.stream, _ = args[len(args)-2].(string)
			call.id, _ = args[len(args)-1].(string)
		}
		for i := 1; i+1 < len(args); i++ {
			if name, ok := args[i].(string); ok && name == "block" {
				call.block, _ = args[i+1].(int64)
				break
			}
		}
		h.calls <- call
		<-ctx.Done()
		return ctx.Err()
	}
}

func (*blockingPollerXReadHook) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

type retryPollerXReadHook struct {
	calls        chan pollerXReadCall
	releaseFirst chan struct{}
	first        bool
}

func (*retryPollerXReadHook) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *retryPollerXReadHook) ProcessHook(next rds.ProcessHook) rds.ProcessHook {
	return func(ctx context.Context, cmd rds.Cmder) error {
		if cmd.Name() != "xread" {
			return next(ctx, cmd)
		}

		args := cmd.Args()
		call := pollerXReadCall{}
		if len(args) >= 3 {
			call.stream, _ = args[len(args)-2].(string)
			call.id, _ = args[len(args)-1].(string)
		}
		for i := 1; i+1 < len(args); i++ {
			if name, ok := args[i].(string); ok && name == "block" {
				call.block, _ = args[i+1].(int64)
				break
			}
		}
		h.calls <- call
		if !h.first {
			h.first = true
			select {
			case <-h.releaseFirst:
				return rds.Nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return next(ctx, cmd)
	}
}

func (*retryPollerXReadHook) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

type initialTailPollerHook struct {
	initialStarted chan struct{}
	releaseInitial chan struct{}
	xreads         chan pollerXReadCall
	initialErr     error
	failedInitial  bool
}

func (*initialTailPollerHook) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *initialTailPollerHook) ProcessHook(next rds.ProcessHook) rds.ProcessHook {
	return func(ctx context.Context, cmd rds.Cmder) error {
		switch cmd.Name() {
		case "xrevrange":
			if !h.failedInitial {
				h.failedInitial = true
				close(h.initialStarted)
				select {
				case <-h.releaseInitial:
					if h.initialErr != nil {
						return h.initialErr
					}
					return next(ctx, cmd)
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return next(ctx, cmd)
		case "xread":
			args := cmd.Args()
			call := pollerXReadCall{}
			if len(args) >= 3 {
				call.stream, _ = args[len(args)-2].(string)
				call.id, _ = args[len(args)-1].(string)
			}
			h.xreads <- call
			<-ctx.Done()
			return ctx.Err()
		default:
			return next(ctx, cmd)
		}
	}
}

func (*initialTailPollerHook) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func newBlockingPollerClient(t *testing.T) (*rediswire.RedisClient, *blockingPollerXReadHook) {
	t.Helper()
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	hook := &blockingPollerXReadHook{calls: make(chan pollerXReadCall, 8)}
	client.AddHook(hook)
	return mustRedisClient(t, context.Background(), client), hook
}

func waitForPollerXRead(t *testing.T, hook *blockingPollerXReadHook) pollerXReadCall {
	t.Helper()
	return waitForPollerXReadCall(t, hook.calls)
}

func waitForPollerXReadCall(t *testing.T, calls <-chan pollerXReadCall) pollerXReadCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(3 * time.Second):
		t.Fatal("XREAD was not started")
		return pollerXReadCall{}
	}
}

func waitForPollerExit(t *testing.T, poller *redisStreamsPoller) {
	t.Helper()
	select {
	case <-poller.done:
	case <-time.After(time.Second):
		t.Fatal("stream poller did not exit after cancellation")
	}
}

func TestRedisStreamsPollerSharesOneReadPerStream(t *testing.T) {
	redisClient, hook := newBlockingPollerClient(t)
	server := socket.NewServer(nil, nil)
	first := NewRedisStreamsAdapter(socket.NewNamespace(server, "/first"), redisClient, nil).(*redisStreamsAdapter)
	second := NewRedisStreamsAdapter(socket.NewNamespace(server, "/second"), redisClient, nil).(*redisStreamsAdapter)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	if first.streamPoller != second.streamPoller {
		t.Fatal("namespaces assigned to the same stream did not share a poller")
	}
	call := waitForPollerXRead(t, hook)
	if call.stream != first.streamName {
		t.Fatalf("XREAD stream = %q, want %q", call.stream, first.streamName)
	}
	if call.id != "0-0" {
		t.Fatalf("initial XREAD ID = %q, want concrete empty-stream ID 0-0", call.id)
	}
	select {
	case call := <-hook.calls:
		t.Fatalf("shared stream started an extra XREAD for %q", call.stream)
	case <-time.After(20 * time.Millisecond):
	}

	poller := first.streamPoller
	first.Close()
	if poller.ctx.Err() != nil {
		t.Fatal("poller stopped while another namespace still referenced its stream")
	}
	second.Close()
	waitForPollerExit(t, poller)
}

func TestRedisStreamsPollerFreezesInitialTail(t *testing.T) {
	redisClient, hook := newBlockingPollerClient(t)
	entryID, err := redisClient.Client().XAdd(context.Background(), &rds.XAddArgs{
		Stream: "history",
		Values: map[string]any{"nsp": "/history"},
	}).Result()
	if err != nil {
		t.Fatal(err)
	}

	opts := DefaultRedisStreamsAdapterOptions()
	opts.SetStreamName("history")
	current := NewRedisStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/history"), redisClient, opts,
	).(*redisStreamsAdapter)
	t.Cleanup(current.Close)

	call := waitForPollerXRead(t, hook)
	if call.id != entryID {
		t.Fatalf("initial XREAD ID = %q, want frozen stream tail %q", call.id, entryID)
	}
	if call.id == "$" {
		t.Fatal("initial XREAD reused the moving $ cursor")
	}
}

func TestRedisStreamsPollerFreezesHealthyTailBeforeConstructReturns(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	hook := &initialTailPollerHook{
		initialStarted: make(chan struct{}),
		releaseInitial: make(chan struct{}),
		xreads:         make(chan pollerXReadCall, 1),
	}
	defer func() {
		select {
		case <-hook.releaseInitial:
		default:
			close(hook.releaseInitial)
		}
	}()
	client.AddHook(hook)
	redisClient := mustRedisClient(t, context.Background(), client)
	socketServer := socket.NewServer(nil, nil)
	firstConstructed := make(chan *redisStreamsAdapter, 1)
	go func() {
		firstConstructed <- NewRedisStreamsAdapter(
			socket.NewNamespace(socketServer, "/healthy-start"), redisClient, nil,
		).(*redisStreamsAdapter)
	}()

	select {
	case <-hook.initialStarted:
	case <-time.After(time.Second):
		t.Fatal("initial tail lookup was not started")
	}
	secondConstructed := make(chan *redisStreamsAdapter, 1)
	go func() {
		secondConstructed <- NewRedisStreamsAdapter(
			socket.NewNamespace(socketServer, "/healthy-start-2"), redisClient, nil,
		).(*redisStreamsAdapter)
	}()
	select {
	case current := <-firstConstructed:
		current.Close()
		t.Fatal("poller creator returned before the healthy tail lookup completed")
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case current := <-secondConstructed:
		current.Close()
		t.Fatal("shared poller acquisition returned before the healthy tail lookup completed")
	case <-time.After(20 * time.Millisecond):
	}
	close(hook.releaseInitial)

	var first, second *redisStreamsAdapter
	select {
	case first = <-firstConstructed:
	case <-time.After(time.Second):
		t.Fatal("poller creator did not return after the healthy tail lookup completed")
	}
	select {
	case second = <-secondConstructed:
	case <-time.After(time.Second):
		t.Fatal("shared poller acquisition did not return after the healthy tail lookup completed")
	}
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)
	if first.streamPoller != second.streamPoller {
		t.Fatal("concurrent adapters did not share the initialized poller")
	}
	call := waitForPollerXReadCall(t, hook.xreads)
	if call.id != "0-0" {
		t.Fatalf("healthy initial XREAD ID = %q, want frozen empty-stream ID 0-0", call.id)
	}
}

func TestRedisStreamsPollerRetriesInitialTailLookupInBackground(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	hook := &initialTailPollerHook{
		initialStarted: make(chan struct{}),
		releaseInitial: make(chan struct{}),
		xreads:         make(chan pollerXReadCall, 1),
		initialErr:     errors.New("temporary tail lookup failure"),
	}
	defer func() {
		select {
		case <-hook.releaseInitial:
		default:
			close(hook.releaseInitial)
		}
	}()
	client.AddHook(hook)
	redisClient := mustRedisClient(t, context.Background(), client)
	constructed := make(chan *redisStreamsAdapter, 1)
	go func() {
		constructed <- NewRedisStreamsAdapter(
			socket.NewNamespace(socket.NewServer(nil, nil), "/initial-retry"), redisClient, nil,
		).(*redisStreamsAdapter)
	}()

	select {
	case <-hook.initialStarted:
	case <-time.After(time.Second):
		t.Fatal("initial tail lookup was not started")
	}
	var current *redisStreamsAdapter
	select {
	case current = <-constructed:
		current.Close()
		t.Fatal("adapter construction returned before the first tail lookup completed")
	case <-time.After(20 * time.Millisecond):
	}

	entryID, err := client.XAdd(context.Background(), &rds.XAddArgs{
		Stream: DefaultStreamName,
		Values: map[string]any{"nsp": "/initial-retry"},
	}).Result()
	if err != nil {
		t.Fatal(err)
	}
	close(hook.releaseInitial)
	select {
	case current = <-constructed:
	case <-time.After(time.Second):
		t.Fatal("adapter construction waited for background tail retries")
	}
	t.Cleanup(current.Close)

	call := waitForPollerXReadCall(t, hook.xreads)
	if call.id != entryID {
		t.Fatalf("XREAD ID after initial tail error = %q, want frozen retry tail %q", call.id, entryID)
	}
}

func TestRedisStreamsPollerKeepsInitialIDAfterTimeout(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	hook := &retryPollerXReadHook{
		calls:        make(chan pollerXReadCall, 4),
		releaseFirst: make(chan struct{}),
	}
	client.AddHook(hook)
	redisClient := mustRedisClient(t, context.Background(), client)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/retry")
	received := make(chan struct{}, 1)
	if err := nsp.On("probe", func(...any) { received <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	current := NewRedisStreamsAdapter(nsp, redisClient, nil).(*redisStreamsAdapter)
	t.Cleanup(current.Close)

	first := waitForPollerXReadCall(t, hook.calls)
	if first.id != "0-0" {
		t.Fatalf("first XREAD ID = %q, want 0-0", first.id)
	}
	message, err := rediswire.EncodeStreamMessage(&clusteradapter.ClusterMessage{
		Uid:  "remote",
		Nsp:  nsp.Name(),
		Type: clusteradapter.SERVER_SIDE_EMIT,
		Data: &clusteradapter.ServerSideEmitMessage{Packet: []any{"probe"}},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = rediswire.XAdd(redisClient, current.streamName, message, current.opts.MaxLen()); err != nil {
		t.Fatal(err)
	}
	close(hook.releaseFirst)

	second := waitForPollerXReadCall(t, hook.calls)
	if second.id != first.id {
		t.Fatalf("XREAD retry ID = %q, want preserved ID %q", second.id, first.id)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("message written during XREAD timeout was skipped")
	}
}

func TestRedisStreamsPollerIsolatesActualStreams(t *testing.T) {
	redisClient, hook := newBlockingPollerClient(t)
	server := socket.NewServer(nil, nil)
	firstOpts := DefaultRedisStreamsAdapterOptions()
	firstOpts.SetStreamName("stream:first")
	secondOpts := DefaultRedisStreamsAdapterOptions()
	secondOpts.SetStreamName("stream:second")
	first := NewRedisStreamsAdapter(socket.NewNamespace(server, "/first"), redisClient, firstOpts).(*redisStreamsAdapter)
	second := NewRedisStreamsAdapter(socket.NewNamespace(server, "/second"), redisClient, secondOpts).(*redisStreamsAdapter)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	if first.streamPoller == second.streamPoller {
		t.Fatal("different actual streams unexpectedly shared a poller")
	}
	streams := map[string]bool{}
	streams[waitForPollerXRead(t, hook).stream] = true
	streams[waitForPollerXRead(t, hook).stream] = true
	if !streams[first.streamName] || !streams[second.streamName] {
		t.Fatalf("XREAD streams = %v, want %q and %q", streams, first.streamName, second.streamName)
	}

	first.Close()
	waitForPollerExit(t, first.streamPoller)
	if second.streamPoller.ctx.Err() != nil {
		t.Fatal("closing one stream stopped another stream's poller")
	}
	second.Close()
	waitForPollerExit(t, second.streamPoller)
}

func TestRedisStreamsPollerBlockPreservesValidValues(t *testing.T) {
	for _, test := range []struct {
		name    string
		ms      int64
		want    time.Duration
		wantErr bool
	}{
		{name: "negative", ms: -1, wantErr: true},
		{name: "zero", ms: 0, wantErr: true},
		{name: "positive", ms: 25, want: 25 * time.Millisecond},
		{name: "default", ms: DefaultBlockTimeInMs, want: time.Duration(DefaultBlockTimeInMs) * time.Millisecond},
		{name: "above maximum", ms: DefaultBlockTimeInMs + 1, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := redisStreamsPollerBlock(test.ms)
			if (err != nil) != test.wantErr {
				t.Fatalf("effective block error = %v, wantErr %t", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("effective block = %s, want %s", got, test.want)
			}
		})
	}
}

func TestRedisStreamsPollerReportsZeroBlockTime(t *testing.T) {
	redisClient, hook := newBlockingPollerClient(t)
	reported := make(chan error, 1)
	if err := redisClient.On("error", func(args ...any) {
		if len(args) == 1 {
			if err, ok := args[0].(error); ok {
				reported <- err
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	opts := DefaultRedisStreamsAdapterOptions()
	opts.SetBlockTimeInMs(0)
	current := NewRedisStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/cancel"), redisClient, opts,
	).(*redisStreamsAdapter)
	t.Cleanup(current.Close)

	call := waitForPollerXRead(t, hook)
	if call.block != DefaultBlockTimeInMs {
		t.Fatalf("fallback XREAD BLOCK = %dms, want %dms", call.block, DefaultBlockTimeInMs)
	}
	if current.opts.BlockTimeInMs() != 0 {
		t.Fatalf("public raw block option was changed to %d", current.opts.BlockTimeInMs())
	}

	select {
	case err := <-reported:
		if err == nil || err.Error() == "" {
			t.Fatalf("invalid block error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("zero blockTimeInMs did not emit an error")
	}
}

func TestRedisStreamsPollerReportsBlockAboveFiveSeconds(t *testing.T) {
	redisClient, hook := newBlockingPollerClient(t)
	reported := make(chan error, 1)
	if err := redisClient.On("error", func(args ...any) {
		if len(args) == 1 {
			if err, ok := args[0].(error); ok {
				reported <- err
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	opts := DefaultRedisStreamsAdapterOptions()
	opts.SetBlockTimeInMs(DefaultBlockTimeInMs + 1)
	current := NewRedisStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/long-block"), redisClient, opts,
	).(*redisStreamsAdapter)
	t.Cleanup(current.Close)

	call := waitForPollerXRead(t, hook)
	if call.block != DefaultBlockTimeInMs {
		t.Fatalf("fallback XREAD BLOCK = %dms, want %dms", call.block, DefaultBlockTimeInMs)
	}
	select {
	case err := <-reported:
		if err == nil || err.Error() == "" {
			t.Fatalf("invalid block error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blockTimeInMs above 5000 did not emit an error")
	}
}

func TestRedisStreamsPollerReportsNegativeBlockTime(t *testing.T) {
	redisClient, hook := newBlockingPollerClient(t)
	reported := make(chan error, 1)
	if err := redisClient.On("error", func(args ...any) {
		if len(args) == 1 {
			if err, ok := args[0].(error); ok {
				reported <- err
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	opts := DefaultRedisStreamsAdapterOptions()
	opts.SetBlockTimeInMs(-1)
	current := NewRedisStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/invalid-block"), redisClient, opts,
	).(*redisStreamsAdapter)
	t.Cleanup(current.Close)

	call := waitForPollerXRead(t, hook)
	if call.block != DefaultBlockTimeInMs {
		t.Fatalf("fallback XREAD BLOCK = %dms, want %dms", call.block, DefaultBlockTimeInMs)
	}
	select {
	case err := <-reported:
		if err == nil || err.Error() == "" {
			t.Fatalf("invalid block error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("negative blockTimeInMs did not emit an error")
	}
}

func TestRedisStreamsPollerRestoresPreviousAdapterOnReverseClose(t *testing.T) {
	redisClient, _ := newBlockingPollerClient(t)
	server := socket.NewServer(nil, nil)
	nsp := socket.NewNamespace(server, "/same")
	first := NewRedisStreamsAdapter(nsp, redisClient, nil).(*redisStreamsAdapter)
	second := NewRedisStreamsAdapter(nsp, redisClient, nil).(*redisStreamsAdapter)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	poller := first.streamPoller
	if second.streamPoller != poller {
		t.Fatal("same namespace adapters did not share a poller")
	}
	second.Close()
	if current, ok := poller.adapters.Load(nsp.Name()); !ok || current != first {
		t.Fatal("closing the newest adapter did not restore the previous registration")
	}
	if poller.ctx.Err() != nil {
		t.Fatal("poller stopped while the previous adapter remained active")
	}

	first.Close()
	waitForPollerExit(t, poller)
}
