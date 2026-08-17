package adapter

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	rediswire "github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type pollerXReadCall struct {
	stream string
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
	select {
	case call := <-hook.calls:
		return call
	case <-time.After(time.Second):
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

func TestRedisStreamsPollerBlockIsFinite(t *testing.T) {
	for _, test := range []struct {
		name string
		ms   int64
		want time.Duration
	}{
		{name: "negative", ms: -1, want: maxRedisStreamsPollerBlock},
		{name: "zero", ms: 0, want: maxRedisStreamsPollerBlock},
		{name: "positive", ms: 25, want: 25 * time.Millisecond},
		{name: "default", ms: DefaultBlockTimeInMs, want: maxRedisStreamsPollerBlock},
		{name: "above maximum", ms: DefaultBlockTimeInMs + 1, want: maxRedisStreamsPollerBlock},
		{name: "overflow", ms: int64(1<<63 - 1), want: maxRedisStreamsPollerBlock},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := redisStreamsPollerBlock(test.ms); got != test.want {
				t.Fatalf("effective block = %s, want %s", got, test.want)
			}
		})
	}
}

func TestRedisStreamsPollerCancelStopsInfiniteRawBlock(t *testing.T) {
	redisClient, hook := newBlockingPollerClient(t)
	opts := DefaultRedisStreamsAdapterOptions()
	opts.SetBlockTimeInMs(0)
	current := NewRedisStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/cancel"), redisClient, opts,
	).(*redisStreamsAdapter)
	t.Cleanup(current.Close)

	call := waitForPollerXRead(t, hook)
	if call.block <= 0 || time.Duration(call.block)*time.Millisecond > maxRedisStreamsPollerBlock {
		t.Fatalf("XREAD BLOCK = %dms, want a finite positive value up to %s", call.block, maxRedisStreamsPollerBlock)
	}
	if current.opts.BlockTimeInMs() != 0 {
		t.Fatalf("public raw block option was changed to %d", current.opts.BlockTimeInMs())
	}

	poller := current.streamPoller
	current.Close()
	waitForPollerExit(t, poller)
}
