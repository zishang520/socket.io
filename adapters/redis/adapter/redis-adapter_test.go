package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

const unavailableRedisAddress = "127.0.0.1:0"

type recordingParser struct {
	decodeCalled bool
	encodeErr    error
	packet       *Packet
}

func (p *recordingParser) Encode(any) ([]byte, error) { return nil, p.encodeErr }

func (p *recordingParser) Decode(_ []byte, value any) error {
	p.decodeCalled = true
	switch packet := value.(type) {
	case *Packet:
		*packet = *p.packet
	case **Packet:
		*packet = p.packet
	}
	return nil
}

type recordingLocalAdapter struct {
	socket.Adapter
	broadcasts        int
	broadcastsWithAck int
	adds              int
	dels              int
	disconnects       int
	broadcastStarted  chan struct{}
	broadcastRelease  chan struct{}
	invokeClientCount bool
}

type requestRecordingAdapter struct {
	socket.Adapter
	broadcastsWithAck atomic.Int64
	adds              atomic.Int64
	dels              atomic.Int64
	disconnects       atomic.Int64
	fetches           atomic.Int64
}

type fetchSocketsErrorAdapter struct {
	socket.Adapter
	err   error
	calls atomic.Int64
}

type clusterResponseAdapter struct {
	socket.Adapter
	sockets     []socket.SocketDetails
	clientCount uint64
	ack         []any
}

type processErrorHook struct {
	err        error
	panicValue any
	calls      atomic.Int64
}

type publishTestCall struct {
	request    Request
	channel    string
	skipDecode bool
	err        error
	contextErr error
	started    chan struct{}
	finished   chan struct{}
	release    <-chan struct{}
}

type publishTestHook struct {
	channel    string
	publishErr error
	calls      chan *publishTestCall
}

func (h *processErrorHook) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *processErrorHook) ProcessHook(rds.ProcessHook) rds.ProcessHook {
	return func(context.Context, rds.Cmder) error {
		h.calls.Add(1)
		if h.panicValue != nil {
			panic(h.panicValue)
		}
		return h.err
	}
}

func (h *processErrorHook) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func (h *publishTestHook) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *publishTestHook) ProcessHook(next rds.ProcessHook) rds.ProcessHook {
	return func(ctx context.Context, cmd rds.Cmder) error {
		switch cmd.Name() {
		case "pubsub":
			result, ok := cmd.(*rds.MapStringIntCmd)
			if !ok {
				return fmt.Errorf("unexpected PUBSUB command type %T", cmd)
			}
			result.SetVal(map[string]int64{h.channel: 2})
			return nil
		case "publish", "spublish":
			call := <-h.calls
			call.contextErr = ctx.Err()
			args := cmd.Args()
			if len(args) < 3 {
				call.err = fmt.Errorf("unexpected PUBLISH arguments: %v", args)
			} else {
				call.channel, _ = args[1].(string)
				if !call.skipDecode {
					switch payload := args[2].(type) {
					case []byte:
						call.err = json.Unmarshal(payload, &call.request)
					case string:
						call.err = json.Unmarshal([]byte(payload), &call.request)
					default:
						call.err = fmt.Errorf("unexpected PUBLISH payload type %T", payload)
					}
				}
			}
			close(call.started)
			if call.release != nil {
				<-call.release
			}
			if call.finished != nil {
				close(call.finished)
			}
			return h.publishErr
		default:
			return next(ctx, cmd)
		}
	}
}

func (h *publishTestHook) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func (a *recordingLocalAdapter) Broadcast(*parser.Packet, *socket.BroadcastOptions) {
	if a.broadcastStarted != nil {
		close(a.broadcastStarted)
		<-a.broadcastRelease
	}
	a.broadcasts++
}

func (a *recordingLocalAdapter) BroadcastWithAck(_ *parser.Packet, _ *socket.BroadcastOptions, clientCountCallback func(uint64), _ socket.Ack) {
	a.broadcastsWithAck++
	if a.invokeClientCount {
		clientCountCallback(0)
	}
}

func (a *recordingLocalAdapter) AddSockets(*socket.BroadcastOptions, []socket.Room) {
	a.adds++
}

func (a *recordingLocalAdapter) DelSockets(*socket.BroadcastOptions, []socket.Room) {
	a.dels++
}

func (a *recordingLocalAdapter) DisconnectSockets(*socket.BroadcastOptions, bool) {
	a.disconnects++
}

func (a *requestRecordingAdapter) BroadcastWithAck(*parser.Packet, *socket.BroadcastOptions, func(uint64), socket.Ack) {
	a.broadcastsWithAck.Add(1)
}

func (a *requestRecordingAdapter) AddSockets(*socket.BroadcastOptions, []socket.Room) {
	a.adds.Add(1)
}

func (a *requestRecordingAdapter) DelSockets(*socket.BroadcastOptions, []socket.Room) {
	a.dels.Add(1)
}

func (a *requestRecordingAdapter) DisconnectSockets(*socket.BroadcastOptions, bool) {
	a.disconnects.Add(1)
}

func (a *requestRecordingAdapter) FetchSockets(*socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(func([]socket.SocketDetails, error)) {
		a.fetches.Add(1)
	}
}

func (a *fetchSocketsErrorAdapter) FetchSockets(*socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(cb func([]socket.SocketDetails, error)) {
		a.calls.Add(1)
		cb(nil, a.err)
	}
}

func (a *clusterResponseAdapter) FetchSockets(*socket.BroadcastOptions) func(func([]socket.SocketDetails, error)) {
	return func(cb func([]socket.SocketDetails, error)) {
		cb(a.sockets, nil)
	}
}

func (a *clusterResponseAdapter) BroadcastWithAck(_ *parser.Packet, _ *socket.BroadcastOptions, clientCountCallback func(uint64), ack socket.Ack) {
	clientCountCallback(a.clientCount)
	ack(a.ack, nil)
}

func newRequestRoutingAdapter() (*redisAdapter, *requestRecordingAdapter) {
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	local := &requestRecordingAdapter{Adapter: socket.NewAdapter(nsp)}
	current := MakeRedisAdapter().(*redisAdapter)
	current.Adapter = local
	current.uid = "self"
	current.requestChannel = "socket.io-request#/test#"
	current.responseChannel = "socket.io-response#/test#"
	current.ctx = context.Background()
	return current, local
}

func dispatchClassicRequest(t *testing.T, current *redisAdapter, request *Request) {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	current.onRequest(payload, current.requestChannel)
}

func newClassicRedisTestNode(t *testing.T, addr string, local *clusterResponseAdapter) *redisAdapter {
	t.Helper()
	client := rds.NewClient(&rds.Options{Addr: addr})
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	current := NewRedisAdapter(nsp, mustRedisClient(t, t.Context(), client), nil).(*redisAdapter)
	local.Adapter = socket.NewAdapter(nsp)
	current.Adapter = local
	t.Cleanup(func() {
		current.Close()
		_ = client.Close()
	})
	return current
}

func newClassicPublishTestAdapter(t *testing.T, timeout time.Duration, publishErr error) (*redisAdapter, *publishTestHook) {
	t.Helper()
	const requestChannel = "socket.io-request#/test#"
	hook := &publishTestHook{
		channel:    requestChannel,
		publishErr: publishErr,
		calls:      make(chan *publishTestCall, 1),
	}
	client := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress, MaxRetries: -1})
	client.AddHook(hook)
	t.Cleanup(func() { _ = client.Close() })

	current := MakeRedisAdapter().(*redisAdapter)
	current.redisClient = mustRedisClient(t, t.Context(), client)
	current.ctx = t.Context()
	current.uid = "self"
	current.requestChannel = requestChannel
	current.requestsTimeout = timeout
	return current, hook
}

func newClassicPublishQueueTestAdapter(t *testing.T, callCount int) (*redisAdapter, *recordingLocalAdapter, *publishTestHook) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	hook := &publishTestHook{
		channel: "socket.io-request#/test#",
		calls:   make(chan *publishTestCall, callCount),
	}
	client := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress, MaxRetries: -1})
	client.AddHook(hook)
	redisClient := mustRedisClient(t, ctx, client)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	local := &recordingLocalAdapter{Adapter: socket.NewAdapter(nsp)}
	current := MakeRedisAdapter().(*redisAdapter)
	current.Adapter = local
	current.redisClient = redisClient
	current.parser = utils.MsgPack()
	current.ctx = ctx
	current.cancel = cancel
	current.uid = "self"
	current.channel = "socket.io#/test#"
	current.requestChannel = hook.channel
	current.responseChannel = "socket.io-response#/test#"
	current.specificResponseChannel = "socket.io-response#/test#self#"
	t.Cleanup(func() {
		current.Close()
		_ = client.Close()
	})
	return current, local, hook
}

func startServerSideEmitWithAck(t *testing.T, current *redisAdapter, hook *publishTestHook, ack socket.Ack) (*publishTestCall, chan struct{}, <-chan error) {
	t.Helper()
	release := make(chan struct{})
	call := &publishTestCall{started: make(chan struct{}), release: release}
	hook.calls <- call
	result := make(chan error, 1)
	go func() {
		result <- current.serverSideEmitWithAck([]any{"event"}, ack)
	}()
	select {
	case <-call.started:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("PUBLISH was not reached")
	}
	if call.err != nil {
		close(release)
		t.Fatal(call.err)
	}
	return call, release, result
}

func waitServerSideEmitResult(t *testing.T, result <-chan error) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(time.Second):
		t.Fatal("serverSideEmitWithAck did not return")
		return nil
	}
}

func TestClassicBroadcastStopsOnEncodeError(t *testing.T) {
	encodeErr := errors.New("encode failed")
	for _, test := range []struct {
		name string
		run  func(*redisAdapter, *parser.Packet)
	}{
		{
			name: "broadcast",
			run: func(adapter *redisAdapter, packet *parser.Packet) {
				adapter.Broadcast(packet, nil)
			},
		},
		{
			name: "broadcast with acknowledgement",
			run: func(adapter *redisAdapter, packet *parser.Packet) {
				adapter.BroadcastWithAck(packet, nil, func(uint64) {}, func([]any, error) {})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
			local := &recordingLocalAdapter{Adapter: socket.NewAdapter(nsp)}
			goRedisClient := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress})
			t.Cleanup(func() { _ = goRedisClient.Close() })
			client := mustRedisClient(t, context.Background(), goRedisClient)
			var emitted error
			if err := client.On("error", func(args ...any) {
				emitted, _ = args[0].(error)
			}); err != nil {
				t.Fatal(err)
			}

			adapter := MakeRedisAdapter().(*redisAdapter)
			adapter.Adapter = local
			adapter.redisClient = client
			adapter.parser = &recordingParser{encodeErr: encodeErr}

			test.run(adapter, &parser.Packet{Type: parser.EVENT})

			if !errors.Is(emitted, encodeErr) {
				t.Fatalf("emitted error = %v, want %v", emitted, encodeErr)
			}
			if local.broadcasts != 0 || local.broadcastsWithAck != 0 {
				t.Fatalf("local broadcasts = %d/%d, want 0/0", local.broadcasts, local.broadcastsWithAck)
			}
			if adapter.ackRequests.Len() != 0 {
				t.Fatal("acknowledgement request was stored after encoding failed")
			}
		})
	}
}

func TestClassicPublishErrorIsEmitted(t *testing.T) {
	publishErr := errors.New("publish failed")
	goRedisClient := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress})
	goRedisClient.AddHook(&processErrorHook{err: publishErr})
	t.Cleanup(func() { _ = goRedisClient.Close() })
	client := mustRedisClient(t, context.Background(), goRedisClient)
	emitted := make(chan error, 1)
	if err := client.On("error", func(args ...any) {
		emitted <- args[0].(error)
	}); err != nil {
		t.Fatal(err)
	}

	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.Adapter = socket.NewAdapter(nsp)
	adapter.redisClient = client
	adapter.ctx = context.Background()

	_ = adapter.publish("socket.io#/test#", []byte("message"))

	select {
	case err := <-emitted:
		if !errors.Is(err, publishErr) {
			t.Fatalf("emitted error = %v, want %v", err, publishErr)
		}
	case <-time.After(time.Second):
		t.Fatal("publish error was not emitted")
	}
}

func TestClassicPublishErrorHandlerCanReenterPublisher(t *testing.T) {
	publishErr := errors.New("publish failed")
	goRedisClient := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress, MaxRetries: -1})
	goRedisClient.AddHook(&processErrorHook{err: publishErr})
	t.Cleanup(func() { _ = goRedisClient.Close() })
	client := mustRedisClient(t, context.Background(), goRedisClient)

	current := MakeRedisAdapter().(*redisAdapter)
	current.redisClient = client
	current.ctx = context.Background()
	var reentered atomic.Bool
	reentryDone := make(chan error, 1)
	if err := client.On("error", func(...any) {
		if reentered.CompareAndSwap(false, true) {
			reentryDone <- current.publish("socket.io#/test#", []byte("reentrant"))
		}
	}); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		result <- current.publish("socket.io#/test#", []byte("initial"))
	}()
	select {
	case err := <-result:
		if !errors.Is(err, publishErr) {
			t.Fatalf("initial publish error = %v, want %v", err, publishErr)
		}
	case <-time.After(time.Second):
		t.Fatal("initial publish deadlocked in the error handler")
	}
	select {
	case err := <-reentryDone:
		if !errors.Is(err, publishErr) {
			t.Fatalf("reentrant publish error = %v, want %v", err, publishErr)
		}
	case <-time.After(time.Second):
		t.Fatal("error handler could not reenter the publisher")
	}
}

func TestClassicPublishPanicReturnsToWaiter(t *testing.T) {
	goRedisClient := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress, MaxRetries: -1})
	goRedisClient.AddHook(&processErrorHook{panicValue: "publish panic"})
	t.Cleanup(func() { _ = goRedisClient.Close() })
	client := mustRedisClient(t, context.Background(), goRedisClient)

	current := MakeRedisAdapter().(*redisAdapter)
	current.redisClient = client
	current.ctx = context.Background()
	result := make(chan error, 1)
	go func() {
		result <- current.publish("socket.io#/test#", []byte("message"))
	}()

	select {
	case err := <-result:
		if err == nil {
			t.Fatal("publish panic did not return an error")
		}
	case <-time.After(time.Second):
		t.Fatal("publish panic left the waiter blocked")
	}
}

func TestClassicBroadcastsStartLocallyAndPreservePublishOrder(t *testing.T) {
	current, local, hook := newClassicPublishQueueTestAdapter(t, 2)
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseFirst) }) }
	t.Cleanup(release)
	first := &publishTestCall{
		skipDecode: true,
		started:    make(chan struct{}),
		release:    releaseFirst,
	}
	second := &publishTestCall{skipDecode: true, started: make(chan struct{})}
	hook.calls <- first
	hook.calls <- second

	ackDone := make(chan struct{})
	go func() {
		current.BroadcastWithAck(
			&parser.Packet{Type: parser.EVENT, Data: []any{"ack"}},
			nil,
			func(uint64) {},
			func([]any, error) {},
		)
		close(ackDone)
	}()
	select {
	case <-first.started:
	case <-time.After(time.Second):
		t.Fatal("acknowledged PUBLISH did not start")
	}
	select {
	case <-ackDone:
	case <-time.After(time.Second):
		t.Fatal("local acknowledgement lifecycle waited for Redis PUBLISH")
	}
	if local.broadcastsWithAck != 1 {
		t.Fatalf("local acknowledged broadcasts = %d, want 1", local.broadcastsWithAck)
	}

	broadcastDone := make(chan struct{})
	go func() {
		current.Broadcast(&parser.Packet{Type: parser.EVENT, Data: []any{"plain"}}, nil)
		close(broadcastDone)
	}()
	select {
	case <-second.started:
		t.Fatal("ordinary broadcast overtook the acknowledged publish")
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case <-broadcastDone:
	case <-time.After(time.Second):
		t.Fatal("local ordinary broadcast waited for Redis PUBLISH")
	}
	if local.broadcasts != 1 {
		t.Fatalf("local ordinary broadcasts = %d, want 1", local.broadcasts)
	}

	release()
	select {
	case <-second.started:
	case <-time.After(time.Second):
		t.Fatal("ordinary broadcast was not published after the ACK publish")
	}
	if first.channel != current.requestChannel || second.channel != current.channel {
		t.Fatalf("publish order/channels = %q, %q", first.channel, second.channel)
	}
}

func TestClassicResponsesBypassBlockedPublishAndPreserveOrder(t *testing.T) {
	current, _, hook := newClassicPublishQueueTestAdapter(t, 3)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releasePublish := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releasePublish)
	blocked := &publishTestCall{skipDecode: true, started: make(chan struct{}), release: release}
	first := &publishTestCall{skipDecode: true, started: make(chan struct{})}
	second := &publishTestCall{skipDecode: true, started: make(chan struct{})}
	hook.calls <- blocked
	hook.calls <- first
	hook.calls <- second

	current.publishAsync(current.requestChannel, []byte("blocked"))
	select {
	case <-blocked.started:
	case <-time.After(time.Second):
		t.Fatal("blocking PUBLISH did not start")
	}

	current.publishResponseMessage(current.responseChannel, []byte("first"))
	current.publishResponseMessage(current.specificResponseChannel, []byte("second"))
	for index, call := range []*publishTestCall{first, second} {
		select {
		case <-call.started:
		case <-time.After(time.Second):
			t.Fatalf("response %d was blocked by the ordinary publisher", index+1)
		}
	}
	if first.channel != current.responseChannel || second.channel != current.specificResponseChannel {
		t.Fatalf("response order/channels = %q, %q", first.channel, second.channel)
	}
}

func TestClassicPublisherCloseReleasesAndRejects(t *testing.T) {
	current, _, hook := newClassicPublishQueueTestAdapter(t, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	releasePublish := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releasePublish)
	call := &publishTestCall{
		skipDecode: true,
		started:    make(chan struct{}),
		finished:   make(chan struct{}),
		release:    release,
	}
	hook.calls <- call

	result := make(chan error, 1)
	go func() { result <- current.publish(current.channel, []byte("blocked")) }()
	select {
	case <-call.started:
	case <-time.After(time.Second):
		t.Fatal("blocked PUBLISH did not start")
	}

	current.Close()
	select {
	case err := <-result:
		t.Fatalf("current publish returned before its task finished: %v", err)
	default:
	}
	if err := current.publish(current.channel, []byte("rejected")); !errors.Is(err, adapter.ErrAdapterClosed) {
		t.Fatalf("publish after Close error = %v", err)
	}
	releasePublish()
	select {
	case <-call.finished:
	case <-time.After(time.Second):
		t.Fatal("raw publish task did not finish")
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("current publish error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("current publish did not return after its task finished")
	}
}

func TestClosePreservesAcceptedPublishContext(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*redis.RedisClient, context.Context, context.CancelFunc, socket.Namespace) (func(bool), func())
	}{
		{
			name: "classic",
			setup: func(client *redis.RedisClient, ctx context.Context, cancel context.CancelFunc, _ socket.Namespace) (func(bool), func()) {
				current := MakeRedisAdapter().(*redisAdapter)
				current.redisClient = client
				current.ctx, current.cancel = ctx, cancel
				current.channel = "socket.io#/test#"
				current.responseChannel = "socket.io-response#/test#"
				return func(response bool) {
					if response {
						current.publishResponseMessage(current.responseChannel, []byte("response"))
					} else {
						current.publishAsync(current.channel, []byte("message"))
					}
				}, current.Close
			},
		},
		{
			name: "sharded",
			setup: func(client *redis.RedisClient, ctx context.Context, cancel context.CancelFunc, nsp socket.Namespace) (func(bool), func()) {
				current := MakeShardedRedisAdapter().(*shardedRedisAdapter)
				current.redisClient = client
				current.ctx, current.cancel = ctx, cancel
				current.channel = "socket.io#/test#"
				current.ClusterAdapter.Construct(nsp)
				return func(response bool) {
					if response {
						current.PublishResponse("requester", &adapter.ClusterResponse{
							Type: adapter.SERVER_SIDE_EMIT_RESPONSE,
							Data: &adapter.ServerSideEmitResponse{RequestId: "request", Packet: "response"},
						})
					} else {
						current.Publish(&adapter.ClusterMessage{
							Type: adapter.SERVER_SIDE_EMIT,
							Data: &adapter.ServerSideEmitMessage{Packet: []any{"message"}},
						})
					}
				}, current.Close
			},
		},
		{
			name: "streams",
			setup: func(client *redis.RedisClient, ctx context.Context, cancel context.CancelFunc, nsp socket.Namespace) (func(bool), func()) {
				current := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
				current.redisClient = client
				current.ctx, current.cancel = ctx, cancel
				current.publicChannel = "socket.io#/test#"
				current.ClusterAdapter.Construct(nsp)
				return func(response bool) {
					if response {
						current.PublishResponse("requester", &adapter.ClusterResponse{
							Type: adapter.SERVER_SIDE_EMIT_RESPONSE,
							Data: &adapter.ServerSideEmitResponse{RequestId: "request", Packet: "response"},
						})
					} else {
						current.Publish(&adapter.ClusterMessage{
							Type: adapter.SERVER_SIDE_EMIT,
							Data: &adapter.ServerSideEmitMessage{Packet: []any{"message"}},
						})
					}
				}, current.Close
			},
		},
	}

	for _, test := range tests {
		for _, response := range []bool{false, true} {
			kind := "publish"
			if response {
				kind = "response"
			}
			t.Run(test.name+"/"+kind, func(t *testing.T) {
				hook := &publishTestHook{calls: make(chan *publishTestCall, 2)}
				client := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress, MaxRetries: -1})
				client.AddHook(hook)
				redisClient := mustRedisClient(t, context.Background(), client)
				ctx, cancel := context.WithCancel(redisClient.Context())
				nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
				publish, closeAdapter := test.setup(redisClient, ctx, cancel, nsp)
				t.Cleanup(func() {
					closeAdapter()
					_ = client.Close()
				})

				release := make(chan struct{})
				var releaseOnce sync.Once
				releaseFirst := func() { releaseOnce.Do(func() { close(release) }) }
				t.Cleanup(releaseFirst)
				first := &publishTestCall{skipDecode: true, started: make(chan struct{}), release: release}
				second := &publishTestCall{skipDecode: true, started: make(chan struct{}), finished: make(chan struct{})}
				hook.calls <- first
				hook.calls <- second

				publish(response)
				select {
				case <-first.started:
				case <-time.After(time.Second):
					t.Fatal("first publish did not start")
				}
				publish(response)
				closeAdapter()
				select {
				case <-ctx.Done():
				default:
					t.Fatal("adapter context was not canceled")
				}
				releaseFirst()

				select {
				case <-second.started:
				case <-time.After(time.Second):
					t.Fatal("accepted publish did not run after Close")
				}
				if second.contextErr != nil {
					t.Fatalf("accepted publish context error = %v", second.contextErr)
				}
				select {
				case <-second.finished:
				case <-time.After(time.Second):
					t.Fatal("accepted publish did not finish")
				}
			})
		}
	}
}

func TestClassicBroadcastWithAckUsesDefaultTimeout(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	redisClient := mustRedisClient(t, context.Background(), client)
	if err := redisClient.On("error", func(...any) {}); err != nil {
		t.Fatal(err)
	}
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	redisAdapter := NewRedisAdapter(nsp, redisClient, nil).(*redisAdapter)
	t.Cleanup(func() {
		redisAdapter.Close()
		_ = client.Close()
	})

	redisAdapter.BroadcastWithAck(
		&parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
		nil,
		func(uint64) {},
		func([]any, error) {},
	)
	time.Sleep(20 * time.Millisecond)

	if redisAdapter.ackRequests.Len() != 1 {
		t.Fatal("acknowledgement request expired before the default timeout")
	}
}

// Regression test for the cross-node acknowledgement path reported in
// https://github.com/zishang520/socket.io/issues/22.
func TestClassicBroadcastWithAckAcrossNodes(t *testing.T) {
	server := miniredis.RunT(t)
	first := newClassicRedisTestNode(t, server.Addr(), &clusterResponseAdapter{
		clientCount: 1,
		ack:         []any{"first"},
	})
	second := newClassicRedisTestNode(t, server.Addr(), &clusterResponseAdapter{
		clientCount: 2,
		ack:         []any{"second"},
	})
	waitForRedisPubSub(t, func() bool {
		firstCount, firstErr := first.ServerCount()
		secondCount, secondErr := second.ServerCount()
		return firstErr == nil && secondErr == nil && firstCount == 2 && secondCount == 2
	})

	counts := make(chan uint64, 2)
	acks := make(chan []any, 2)
	first.BroadcastWithAck(
		&parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
		nil,
		func(count uint64) { counts <- count },
		func(args []any, _ error) { acks <- args },
	)

	seenCounts := types.NewSet[uint64]()
	seenAcks := types.NewSet[string]()
	for range 2 {
		select {
		case count := <-counts:
			seenCounts.Add(count)
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for cross-node client count")
		}
		select {
		case args := <-acks:
			if len(args) != 1 {
				t.Fatalf("acknowledgement = %#v, want one value", args)
			}
			value, ok := args[0].(string)
			if !ok {
				t.Fatalf("acknowledgement value type = %T, want string", args[0])
			}
			seenAcks.Add(value)
		case <-time.After(3 * time.Second):
			t.Fatal("timed out waiting for cross-node acknowledgement")
		}
	}
	if !seenCounts.Has(1) || !seenCounts.Has(2) || !seenAcks.Has("first") || !seenAcks.Has("second") {
		t.Fatalf("client counts/acknowledgements = %v/%v", seenCounts.Keys(), seenAcks.Keys())
	}
}

func TestRedisAdapterTypedNilOptionsAndParserUseDefaults(t *testing.T) {
	current := MakeRedisAdapter().(*redisAdapter)
	var typedNilOptions *RedisAdapterOptions
	current.SetOpts(typedNilOptions)

	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	options := DefaultRedisAdapterOptions()
	options.SetParser((*recordingParser)(nil))
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	current = NewRedisAdapter(nsp, mustRedisClient(t, t.Context(), client), options).(*redisAdapter)
	t.Cleanup(current.Close)

	if utils.IsNil(current.Parser()) {
		t.Fatal("typed nil parser was not replaced with the default")
	}
}

func TestClassicBroadcastAckTimeoutNormalization(t *testing.T) {
	tests := []struct {
		name        string
		timeout     float64
		stillActive time.Duration
	}{
		{name: "zero"},
		{name: "negative", timeout: -1},
		{name: "above JavaScript maximum", timeout: 1 << 31},
		{name: "normal", timeout: 100, stillActive: 20 * time.Millisecond},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := miniredis.RunT(t)
			client := rds.NewClient(&rds.Options{Addr: server.Addr()})
			redisClient := mustRedisClient(t, t.Context(), client)
			if err := redisClient.On("error", func(...any) {}); err != nil {
				t.Fatal(err)
			}
			current := NewRedisAdapter(
				socket.NewNamespace(socket.NewServer(nil, nil), "/test"),
				redisClient,
				nil,
			).(*redisAdapter)
			t.Cleanup(func() {
				current.Close()
				_ = client.Close()
			})

			current.BroadcastWithAck(
				&parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
				&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Timeout: &test.timeout}},
				func(uint64) {},
				func([]any, error) {},
			)

			if test.stillActive > 0 {
				time.Sleep(test.stillActive)
				if current.ackRequests.Len() != 1 {
					t.Fatal("normal acknowledgement timeout expired too early")
				}
			}
			deadline := time.Now().Add(time.Second)
			for current.ackRequests.Len() != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			if current.ackRequests.Len() != 0 {
				t.Fatal("acknowledgement request did not expire")
			}
		})
	}
}

func TestClassicAckRequestFollowsAdapterContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.ctx = ctx
	adapter.registerAckRequest("request", &AckRequest{}, time.Minute)

	cancel()
	deadline := time.Now().Add(time.Second)
	for adapter.ackRequests.Len() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if adapter.ackRequests.Len() != 0 {
		t.Fatal("context cancellation did not remove the acknowledgement request")
	}
}

func TestClassicAckRequestFollowsAdapterClose(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	current := NewRedisAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/test"),
		mustRedisClient(t, t.Context(), client),
		nil,
	).(*redisAdapter)
	current.registerAckRequest("request", &AckRequest{}, time.Hour)
	if current.ackRequests.Len() != 1 {
		t.Fatal("acknowledgement request was not registered")
	}

	current.Close()
	deadline := time.Now().Add(time.Second)
	for current.ackRequests.Len() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if current.ackRequests.Len() != 0 {
		t.Fatal("Close did not remove the acknowledgement request")
	}
}

func TestClassicServerCountErrorsAreReturned(t *testing.T) {
	countErr := errors.New("count failed")
	client := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress})
	client.AddHook(&processErrorHook{err: countErr})

	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.Adapter = socket.NewAdapter(nsp)
	adapter.redisClient = mustRedisClient(t, context.Background(), client)
	adapter.ctx = context.Background()
	adapter.requestChannel = "socket.io-request#/#"

	var allRoomsErr error
	adapter.AllRooms()(func(_ *types.Set[socket.Room], err error) {
		allRoomsErr = err
	})
	if !errors.Is(allRoomsErr, countErr) {
		t.Fatalf("AllRooms() error = %v, want %v", allRoomsErr, countErr)
	}

	var fetchSocketsErr error
	adapter.FetchSockets(nil)(func(_ []socket.SocketDetails, err error) {
		fetchSocketsErr = err
	})
	if !errors.Is(fetchSocketsErr, countErr) {
		t.Fatalf("FetchSockets() error = %v, want %v", fetchSocketsErr, countErr)
	}

	ackCalled := false
	err := adapter.ServerSideEmit([]any{"event", func([]any, error) {
		ackCalled = true
	}})
	if !errors.Is(err, countErr) {
		t.Fatalf("ServerSideEmit() error = %v, want %v", err, countErr)
	}
	if ackCalled {
		t.Fatal("acknowledgement was called after server count failed")
	}
	if adapter.requests.Len() != 0 {
		t.Fatal("request was stored after server count failed")
	}
}

func TestClassicRequestPublishErrorsAreReturnedImmediately(t *testing.T) {
	publishErr := errors.New("publish failed")
	const timeout = 20 * time.Millisecond
	tests := []struct {
		name string
		run  func(*redisAdapter, func(error))
	}{
		{
			name: "all rooms",
			run: func(current *redisAdapter, done func(error)) {
				current.AllRooms()(func(_ *types.Set[socket.Room], err error) { done(err) })
			},
		},
		{
			name: "fetch sockets",
			run: func(current *redisAdapter, done func(error)) {
				current.FetchSockets(nil)(func(_ []socket.SocketDetails, err error) { done(err) })
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current, hook := newClassicPublishTestAdapter(t, timeout, publishErr)
			nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
			current.Adapter = socket.NewAdapter(nsp)
			_ = current.redisClient.On("error", func(...any) {})
			call := &publishTestCall{started: make(chan struct{})}
			hook.calls <- call

			var callbackCount atomic.Int64
			var gotErr error
			test.run(current, func(err error) {
				callbackCount.Add(1)
				gotErr = err
			})

			if call.err != nil {
				t.Fatal(call.err)
			}
			if !errors.Is(gotErr, publishErr) {
				t.Fatalf("callback error = %v, want %v", gotErr, publishErr)
			}
			if current.requests.Len() != 0 {
				t.Fatal("request was retained after publish failed")
			}

			time.Sleep(timeout + 10*time.Millisecond)
			if got := callbackCount.Load(); got != 1 {
				t.Fatalf("callback count = %d, want 1", got)
			}
		})
	}
}

func TestClassicFetchSocketsReturnsLocalErrorWithoutRedis(t *testing.T) {
	localErr := errors.New("local fetch failed")
	hook := &processErrorHook{err: errors.New("unexpected Redis command")}
	client := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress, MaxRetries: -1})
	client.AddHook(hook)
	t.Cleanup(func() { _ = client.Close() })

	local := &fetchSocketsErrorAdapter{Adapter: socket.MakeAdapter(), err: localErr}
	current := MakeRedisAdapter().(*redisAdapter)
	current.Adapter = local
	current.redisClient = mustRedisClient(t, t.Context(), client)
	current.ctx = t.Context()
	current.requestChannel = "socket.io-request#/test#"

	var callbackCount atomic.Int64
	var sockets []socket.SocketDetails
	var gotErr error
	current.FetchSockets(nil)(func(result []socket.SocketDetails, err error) {
		callbackCount.Add(1)
		sockets = result
		gotErr = err
	})

	if gotErr != localErr {
		t.Fatalf("FetchSockets() error = %v, want original %v", gotErr, localErr)
	}
	if sockets != nil {
		t.Fatalf("FetchSockets() sockets = %v, want nil", sockets)
	}
	if got := callbackCount.Load(); got != 1 {
		t.Fatalf("callback count = %d, want 1", got)
	}
	if got := local.calls.Load(); got != 1 {
		t.Fatalf("local FetchSockets call count = %d, want 1", got)
	}
	if got := hook.calls.Load(); got != 0 {
		t.Fatalf("Redis command count = %d, want 0", got)
	}
	if current.requests.Len() != 0 {
		t.Fatal("local FetchSockets error registered a pending request")
	}
}

// Regression test for https://github.com/zishang520/socket.io/issues/103.
func TestClassicFetchSocketsAcrossNodes(t *testing.T) {
	server := miniredis.RunT(t)
	first := newClassicRedisTestNode(t, server.Addr(), &clusterResponseAdapter{
		sockets: []socket.SocketDetails{adapter.NewRemoteSocket(&adapter.SocketResponse{Id: "first"})},
	})
	second := newClassicRedisTestNode(t, server.Addr(), &clusterResponseAdapter{
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
	first.FetchSockets(nil)(func(sockets []socket.SocketDetails, err error) {
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
		if len(result.sockets) != 2 || !ids.Has("first") || !ids.Has("second") {
			t.Fatalf("fetched sockets = %#v", ids.Keys())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for cross-node FetchSockets response")
	}
}

func TestClassicServerSideEmitPublishFailureAbortsPendingRequest(t *testing.T) {
	publishErr := errors.New("publish failed")
	const timeout = 50 * time.Millisecond
	current, hook := newClassicPublishTestAdapter(t, timeout, publishErr)
	var ackCount atomic.Int64

	call, release, result := startServerSideEmitWithAck(t, current, hook, func([]any, error) {
		ackCount.Add(1)
	})
	if _, ok := current.requests.Load(call.request.RequestId); !ok {
		close(release)
		t.Fatal("request was not pending while PUBLISH was blocked")
	}
	close(release)
	if err := waitServerSideEmitResult(t, result); !errors.Is(err, publishErr) {
		t.Fatalf("serverSideEmitWithAck() error = %v, want %v", err, publishErr)
	}
	if current.requests.Len() != 0 {
		t.Fatal("publish failure retained pending request state")
	}

	time.Sleep(timeout + 20*time.Millisecond)
	if got := ackCount.Load(); got != 0 {
		t.Fatalf("acknowledgement count after publish failure = %d, want 0", got)
	}
	if current.requests.Len() != 0 {
		t.Fatal("publish failure leaked a pending request")
	}
}

func TestClassicServerSideEmitCompletionWinsPublishError(t *testing.T) {
	publishErr := errors.New("publish failed")
	current, hook := newClassicPublishTestAdapter(t, time.Second, publishErr)
	var ackCount atomic.Int64
	ackCalled := make(chan struct{}, 1)

	call, release, result := startServerSideEmitWithAck(t, current, hook, func([]any, error) {
		ackCount.Add(1)
		ackCalled <- struct{}{}
	})
	if _, ok := current.requests.Load(call.request.RequestId); !ok {
		close(release)
		t.Fatal("request was not pending while PUBLISH was blocked")
	}
	payload, err := json.Marshal(&Response{
		Type:      redis.SERVER_SIDE_EMIT,
		RequestId: call.request.RequestId,
		Data:      []any{"response"},
	})
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	current.onResponse(payload)
	select {
	case <-ackCalled:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("response did not complete the request")
	}
	close(release)

	if err := waitServerSideEmitResult(t, result); err != nil {
		t.Fatalf("serverSideEmitWithAck() error = %v, want nil after completion", err)
	}
	if got := ackCount.Load(); got != 1 {
		t.Fatalf("acknowledgement count = %d, want 1", got)
	}
	if current.requests.Len() != 0 {
		t.Fatal("completed request retained pending state")
	}
}

func TestClassicResponsePreservesRequiredValues(t *testing.T) {
	t.Run("empty socket IDs", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request", Sockets: []socket.SocketId{}})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","sockets":[]}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("empty socket details", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request", Sockets: []adapter.SocketResponse{}})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","sockets":[]}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("zero client count", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request", ClientCount: new(uint64(0))})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","clientCount":0}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("empty rooms", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request", Rooms: []socket.Room{}})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","rooms":[]}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("nil acknowledgement", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request"})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request"}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("Node.js Buffer acknowledgement", func(t *testing.T) {
		data, err := json.Marshal(&Response{RequestId: "request", Data: []byte{1, 2}})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","data":{"type":"Buffer","data":[1,2]}}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("socket fields", func(t *testing.T) {
		data, err := json.Marshal(&Response{
			RequestId: "request",
			Sockets: []adapter.SocketResponse{{
				Id:    "socket",
				Rooms: []socket.Room{},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","sockets":[{"id":"socket","handshake":null,"rooms":[],"data":null}]}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

}

func TestRedisAdapterEmptySocketResponsesAreCounted(t *testing.T) {
	for _, test := range []struct {
		name string
		kind redis.RequestType
	}{
		{name: "socket IDs", kind: redis.SOCKETS},
		{name: "socket details", kind: redis.REMOTE_FETCH},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := MakeRedisAdapter().(*redisAdapter)
			request := &RedisRequest{
				Type:      test.kind,
				NumSub:    1,
				Sockets:   types.NewSet[socket.SocketId](),
				Responses: types.NewSlice[any](),
			}
			resolved := false
			request.Resolve = func(values *types.Slice[any]) {
				resolved = values != nil && values.Len() == 0
			}
			adapter.requests.Store("request", request)

			adapter.onResponse([]byte(`{"requestId":"request","sockets":[]}`))

			if !resolved || request.MsgCount.Load() != 1 {
				t.Fatal("empty sockets response was not counted and resolved")
			}
		})
	}
}

func TestRedisAdapterAllRoomsMergesBeforeCompleting(t *testing.T) {
	const responseCount = 512

	adapter := MakeRedisAdapter().(*redisAdapter)
	request := &RedisRequest{
		Type:   redis.ALL_ROOMS,
		NumSub: responseCount,
		Rooms:  types.NewSet[socket.Room](),
	}
	resolved := make(chan int, 1)
	request.Resolve = func(*types.Slice[any]) {
		resolved <- request.Rooms.Len()
	}
	adapter.requests.Store("request", request)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range responseCount {
		wg.Go(func() {
			<-start
			adapter.processResponse(request, &Response{
				RequestId: "request",
				Rooms:     []socket.Room{socket.Room(fmt.Sprintf("room-%d", i))},
			})
		})
	}
	close(start)

	select {
	case roomCount := <-resolved:
		if roomCount != responseCount {
			t.Fatalf("resolved with %d rooms, want %d", roomCount, responseCount)
		}
	case <-time.After(time.Second):
		t.Fatal("ALL_ROOMS request did not resolve")
	}
	wg.Wait()
}

func TestRequestOptionFlags(t *testing.T) {
	opts := &socket.BroadcastOptions{
		Rooms:  types.NewSet[socket.Room]("room"),
		Except: types.NewSet[socket.Room]("except"),
		Flags:  &socket.BroadcastFlags{WriteOptions: socket.WriteOptions{Volatile: true}},
	}

	encoded := adapter.EncodeOptions(opts)
	for _, test := range []struct {
		name          string
		messageType   redis.RequestType
		preserveFlags bool
	}{
		{name: "selection", messageType: redis.REMOTE_FETCH},
		{name: "broadcast", messageType: redis.BROADCAST, preserveFlags: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			payload, err := json.Marshal(&Request{Type: test.messageType, Opts: encoded})
			if err != nil {
				t.Fatal(err)
			}
			var wire Request
			if err := json.Unmarshal(payload, &wire); err != nil {
				t.Fatal(err)
			}
			flags := adapter.DecodeOptions(wire.Opts).Flags
			if test.preserveFlags != (flags != nil && flags.Volatile) {
				t.Fatalf("flags = %#v, preserve = %t", flags, test.preserveFlags)
			}
		})
	}
}

func TestRedisAdapterOnResponseSeparatesSocketPayloads(t *testing.T) {
	t.Run("socket IDs", func(t *testing.T) {
		adapter := MakeRedisAdapter().(*redisAdapter)
		request := &RedisRequest{
			Type:    redis.SOCKETS,
			NumSub:  3,
			Sockets: types.NewSet[socket.SocketId]("local"),
		}
		request.MsgCount.Store(1)
		resolveCount := 0
		resolved := types.NewSet[socket.SocketId]()
		request.Resolve = func(values *types.Slice[any]) {
			resolveCount++
			for _, value := range values.All() {
				resolved.Add(value.(socket.SocketId))
			}
		}
		adapter.requests.Store("request", request)

		adapter.onResponse([]byte(`{"requestId":"request","sockets":["one","shared"]}`))
		if resolveCount != 0 {
			t.Fatal("SOCKETS request resolved before all responses arrived")
		}
		if _, ok := adapter.requests.Load("request"); !ok {
			t.Fatal("incomplete SOCKETS request was deleted")
		}

		adapter.onResponse([]byte(`{"requestId":"request","sockets":["two","shared"]}`))
		if resolveCount != 1 {
			t.Fatalf("resolve count = %d, want 1", resolveCount)
		}
		for _, socketId := range []socket.SocketId{"local", "one", "two", "shared"} {
			if !resolved.Has(socketId) {
				t.Fatalf("resolved sockets do not contain %q", socketId)
			}
		}
		if resolved.Len() != 4 {
			t.Fatalf("resolved socket count = %d, want 4", resolved.Len())
		}
		if _, ok := adapter.requests.Load("request"); ok {
			t.Fatal("completed SOCKETS request was not deleted")
		}

		adapter.onResponse([]byte(`{"requestId":"request","sockets":["late"]}`))
		if resolveCount != 1 {
			t.Fatalf("late response changed resolve count to %d", resolveCount)
		}
	})

	t.Run("empty socket IDs", func(t *testing.T) {
		adapter := MakeRedisAdapter().(*redisAdapter)
		request := &RedisRequest{
			Type:    redis.SOCKETS,
			NumSub:  1,
			Sockets: types.NewSet[socket.SocketId](),
		}
		resolved := false
		request.Resolve = func(values *types.Slice[any]) {
			resolved = true
			if values == nil || values.Len() != 0 {
				t.Fatalf("resolved sockets = %#v, want empty", values)
			}
		}
		adapter.requests.Store("request", request)

		adapter.onResponse([]byte(`{"requestId":"request","sockets":[]}`))

		if !resolved {
			t.Fatal("empty SOCKETS response did not resolve the request")
		}
		if _, ok := adapter.requests.Load("request"); ok {
			t.Fatal("completed empty SOCKETS request was not deleted")
		}
	})

	// Regression test for https://github.com/zishang520/socket.io/issues/103.
	t.Run("socket details", func(t *testing.T) {
		adapter := MakeRedisAdapter().(*redisAdapter)
		request := &RedisRequest{
			Type:      redis.REMOTE_FETCH,
			NumSub:    1,
			Responses: types.NewSlice[any](),
		}
		var resolved []any
		request.Resolve = func(values *types.Slice[any]) {
			resolved = values.All()
		}
		adapter.requests.Store("request", request)

		adapter.onResponse([]byte(`{"requestId":"request","sockets":[{"id":"one","handshake":null,"rooms":[],"data":null}]}`))

		if len(resolved) != 1 {
			t.Fatalf("resolved sockets = %#v", resolved)
		}
		client := resolved[0].(socket.SocketDetails)
		if client.Id() != "one" || client.Rooms() == nil || client.Rooms().Len() != 0 {
			t.Fatalf("resolved socket = %#v", client)
		}
		if _, ok := adapter.requests.Load("request"); ok {
			t.Fatal("completed REMOTE_FETCH request was not deleted")
		}
	})

	t.Run("empty socket details", func(t *testing.T) {
		adapter := MakeRedisAdapter().(*redisAdapter)
		request := &RedisRequest{
			Type:      redis.REMOTE_FETCH,
			NumSub:    1,
			Responses: types.NewSlice[any](),
		}
		resolved := false
		request.Resolve = func(values *types.Slice[any]) {
			resolved = values != nil && values.Len() == 0
		}
		adapter.requests.Store("request", request)

		adapter.onResponse([]byte(`{"requestId":"request","sockets":[]}`))

		if !resolved {
			t.Fatal("empty REMOTE_FETCH response did not resolve the request")
		}
		if _, ok := adapter.requests.Load("request"); ok {
			t.Fatal("completed empty REMOTE_FETCH request was not deleted")
		}
	})
}

func TestRedisAdapterOnMessageAcceptsNamespaceChannel(t *testing.T) {
	parser := &recordingParser{packet: &Packet{Uid: adapter.ServerId("sender")}}
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.channel = "socket.io#/#"
	adapter.uid = "sender"
	adapter.parser = parser

	adapter.onMessage([]byte("payload"), adapter.channel)

	if !parser.decodeCalled {
		t.Fatal("expected namespace channel message to be decoded")
	}
}

func TestClassicBroadcastRequiresValidOptions(t *testing.T) {
	for _, test := range []struct {
		name string
		opts *adapter.PacketOptions
		want int
	}{
		{name: "valid", opts: adapter.EncodeOptions(nil), want: 1},
		{name: "missing options"},
	} {
		t.Run(test.name, func(t *testing.T) {
			nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
			local := &recordingLocalAdapter{Adapter: socket.NewAdapter(nsp)}
			current := MakeRedisAdapter().(*redisAdapter)
			current.Adapter = local
			current.channel = "socket.io#/test#"
			current.uid = "self"
			current.parser = &recordingParser{packet: &Packet{
				Uid:    "remote",
				Packet: &parser.Packet{Type: parser.EVENT, Nsp: "/test"},
				Opts:   test.opts,
			}}

			current.onMessage([]byte("payload"), current.channel)

			if local.broadcasts != test.want {
				t.Fatalf("Broadcast() calls = %d, want %d", local.broadcasts, test.want)
			}
		})
	}
}

func TestRedisAdapterOnResponseWrapsAckPacket(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	var response []any
	adapter.ackRequests.Store("request", &AckRequest{
		Ack: func(args []any, _ error) {
			response = args
		},
	})
	payload, err := json.Marshal(&Response{
		Type:      redis.BROADCAST_ACK,
		RequestId: "request",
		Packet:    []any{"first", "second"},
	})
	if err != nil {
		t.Fatal(err)
	}

	adapter.onResponse(payload)

	want := []any{[]any{"first", "second"}}
	if !reflect.DeepEqual(response, want) {
		t.Fatalf("acknowledgement = %#v, want %#v", response, want)
	}
}

func TestRedisAdapterMalformedAggregateResponsesAreNotCounted(t *testing.T) {
	for _, test := range []struct {
		name    string
		kind    redis.RequestType
		payload string
	}{
		{name: "socket IDs missing", kind: redis.SOCKETS, payload: `{"requestId":"request"}`},
		{name: "socket IDs null", kind: redis.SOCKETS, payload: `{"requestId":"request","sockets":null}`},
		{name: "socket IDs wrong type", kind: redis.SOCKETS, payload: `{"requestId":"request","sockets":{}}`},
		{name: "socket details missing", kind: redis.REMOTE_FETCH, payload: `{"requestId":"request"}`},
		{name: "socket details null", kind: redis.REMOTE_FETCH, payload: `{"requestId":"request","sockets":null}`},
		{name: "socket details wrong type", kind: redis.REMOTE_FETCH, payload: `{"requestId":"request","sockets":"invalid"}`},
		{name: "rooms missing", kind: redis.ALL_ROOMS, payload: `{"requestId":"request"}`},
		{name: "rooms null", kind: redis.ALL_ROOMS, payload: `{"requestId":"request","rooms":null}`},
		{name: "rooms wrong type", kind: redis.ALL_ROOMS, payload: `{"requestId":"request","rooms":{}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := MakeRedisAdapter().(*redisAdapter)
			request := &RedisRequest{
				Type:      test.kind,
				NumSub:    1,
				Sockets:   types.NewSet[socket.SocketId](),
				Responses: types.NewSlice[any](),
				Rooms:     types.NewSet[socket.Room](),
			}
			resolved := false
			request.Resolve = func(*types.Slice[any]) { resolved = true }
			adapter.requests.Store("request", request)

			adapter.onResponse([]byte(test.payload))

			if resolved {
				t.Fatal("malformed response resolved the request")
			}
			if request.MsgCount.Load() != 0 {
				t.Fatalf("message count = %d, want 0", request.MsgCount.Load())
			}
			if _, ok := adapter.requests.Load("request"); !ok {
				t.Fatal("request was deleted after a malformed response")
			}
		})
	}
}

func TestRedisAdapterRejectsMissingBroadcastClientCount(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	called := false
	adapter.ackRequests.Store("request", &AckRequest{
		ClientCountCallback: func(uint64) { called = true },
	})

	adapter.onResponse([]byte(`{"type":8,"requestId":"request"}`))

	if called {
		t.Fatal("missing clientCount payload invoked the callback")
	}
}

func TestClassicRejectsMalformedBroadcastAckRequests(t *testing.T) {
	validRequest := func() *Request {
		return &Request{
			Uid:       "remote",
			RequestId: "request",
			Type:      redis.BROADCAST,
			Packet:    &parser.Packet{Type: parser.EVENT, Nsp: "/test", Data: []any{"event"}},
			Opts:      adapter.EncodeOptions(nil),
		}
	}

	tests := []struct {
		name   string
		mutate func(*Request)
	}{
		{name: "missing sender UID", mutate: func(request *Request) { request.Uid = "" }},
		{name: "missing request ID", mutate: func(request *Request) { request.RequestId = "" }},
		{name: "missing packet", mutate: func(request *Request) { request.Packet = nil }},
		{name: "missing options", mutate: func(request *Request) { request.Opts = nil }},
		{name: "missing rooms", mutate: func(request *Request) { request.Opts.Rooms = nil }},
		{name: "missing except rooms", mutate: func(request *Request) { request.Opts.Except = nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current, local := newRequestRoutingAdapter()
			request := validRequest()
			test.mutate(request)

			current.handleBroadcastRequest(request)

			if got := local.broadcastsWithAck.Load(); got != 0 {
				t.Fatalf("local BroadcastWithAck() calls = %d, want 0", got)
			}
		})
	}
}

func TestClassicSelectionRequestsRequireValidOptions(t *testing.T) {
	operations := []struct {
		name  string
		kind  redis.RequestType
		calls func(*requestRecordingAdapter) int64
	}{
		{name: "join", kind: redis.REMOTE_JOIN, calls: func(local *requestRecordingAdapter) int64 { return local.adds.Load() }},
		{name: "leave", kind: redis.REMOTE_LEAVE, calls: func(local *requestRecordingAdapter) int64 { return local.dels.Load() }},
		{name: "disconnect", kind: redis.REMOTE_DISCONNECT, calls: func(local *requestRecordingAdapter) int64 { return local.disconnects.Load() }},
		{name: "fetch", kind: redis.REMOTE_FETCH, calls: func(local *requestRecordingAdapter) int64 { return local.fetches.Load() }},
	}
	options := []struct {
		name string
		wire string
		want int64
	}{
		{name: "valid", wire: `,"opts":{"rooms":[],"except":[]}`, want: 1},
		{name: "missing options"},
	}

	for _, operation := range operations {
		for _, options := range options {
			t.Run(operation.name+"/"+options.name, func(t *testing.T) {
				current, local := newRequestRoutingAdapter()
				payload := fmt.Sprintf(
					`{"uid":"remote","requestId":"request","type":%d,"rooms":["room"],"close":false%s}`,
					operation.kind,
					options.wire,
				)

				current.onRequest([]byte(payload), current.requestChannel)

				if got := operation.calls(local); got != options.want {
					t.Fatalf("local operation calls = %d, want %d", got, options.want)
				}
			})
		}
	}
}

func TestClassicBulkSocketOperationsRunLocallyOnce(t *testing.T) {
	server := miniredis.RunT(t)
	goRedisClient := rds.NewClient(&rds.Options{Addr: server.Addr()})
	client := mustRedisClient(t, context.Background(), goRedisClient)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	adapter := NewRedisAdapter(nsp, client, nil).(*redisAdapter)
	local := &recordingLocalAdapter{Adapter: socket.NewAdapter(nsp)}
	adapter.Adapter = local
	t.Cleanup(func() {
		adapter.Close()
		_ = goRedisClient.Close()
	})

	adapter.AddSockets(nil, []socket.Room{"room"})
	adapter.DelSockets(nil, []socket.Room{"room"})
	adapter.DisconnectSockets(nil, false)

	// Allow the Redis self-loop to be delivered. It must be ignored because the
	// local operation is now performed explicitly by the publishing adapter.
	time.Sleep(30 * time.Millisecond)
	if local.adds != 1 || local.dels != 1 || local.disconnects != 1 {
		t.Fatalf("local operations = add:%d del:%d disconnect:%d, want 1 each", local.adds, local.dels, local.disconnects)
	}
}

func TestClassicRequestRoutingUsesSenderUID(t *testing.T) {
	t.Run("expired self broadcast is ignored", func(t *testing.T) {
		current, local := newRequestRoutingAdapter()
		current.ctx = t.Context()
		current.registerAckRequest("request", &AckRequest{}, 0)

		deadline := time.Now().Add(time.Second)
		for current.ackRequests.Len() != 0 && time.Now().Before(deadline) {
			time.Sleep(time.Millisecond)
		}
		if current.ackRequests.Len() != 0 {
			t.Fatal("zero-timeout acknowledgement request was not deleted")
		}

		dispatchClassicRequest(t, current, &Request{
			Uid:       current.uid,
			Type:      redis.BROADCAST,
			RequestId: "request",
			Packet:    &parser.Packet{Type: parser.EVENT, Nsp: "/test", Data: []any{"event"}},
			Opts:      adapter.EncodeOptions(nil),
		})
		if got := local.broadcastsWithAck.Load(); got != 0 {
			t.Fatalf("self broadcast count = %d, want 0", got)
		}
	})

	t.Run("remote broadcast request ID collision is handled", func(t *testing.T) {
		current, local := newRequestRoutingAdapter()
		current.ackRequests.Store("request", &AckRequest{})

		dispatchClassicRequest(t, current, &Request{
			Uid:       "remote",
			Type:      redis.BROADCAST,
			RequestId: "request",
			Packet:    &parser.Packet{Type: parser.EVENT, Nsp: "/test", Data: []any{"event"}},
			Opts:      adapter.EncodeOptions(nil),
		})
		if got := local.broadcastsWithAck.Load(); got != 1 {
			t.Fatalf("remote broadcast count = %d, want 1", got)
		}
	})

	t.Run("self fetch is ignored without pending state", func(t *testing.T) {
		current, local := newRequestRoutingAdapter()

		dispatchClassicRequest(t, current, &Request{
			Uid:       current.uid,
			Type:      redis.REMOTE_FETCH,
			RequestId: "request",
			Opts:      adapter.EncodeOptions(nil),
		})
		if got := local.fetches.Load(); got != 0 {
			t.Fatalf("self fetch count = %d, want 0", got)
		}
	})

	t.Run("remote fetch request ID collision is handled", func(t *testing.T) {
		current, local := newRequestRoutingAdapter()
		current.requests.Store("request", &RedisRequest{})

		dispatchClassicRequest(t, current, &Request{
			Uid:       "remote",
			Type:      redis.REMOTE_FETCH,
			RequestId: "request",
			Opts:      adapter.EncodeOptions(nil),
		})
		if got := local.fetches.Load(); got != 1 {
			t.Fatalf("remote fetch count = %d, want 1", got)
		}
	})

	t.Run("missing sender UID is not considered self", func(t *testing.T) {
		current, local := newRequestRoutingAdapter()

		dispatchClassicRequest(t, current, &Request{
			Type:      redis.REMOTE_FETCH,
			RequestId: "request",
			Opts:      adapter.EncodeOptions(nil),
		})
		if got := local.fetches.Load(); got != 1 {
			t.Fatalf("request without UID fetch count = %d, want 1", got)
		}
	})
}

func TestClassicRegisterRequestTimeoutDeletesBeforeCallback(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	request := &RedisRequest{}
	deletedBeforeCallback := make(chan bool, 1)
	var callbackCount atomic.Int64

	adapter.registerRequest("request", request, 10*time.Millisecond, func() {
		callbackCount.Add(1)
		_, exists := adapter.requests.Load("request")
		deletedBeforeCallback <- !exists
	})

	select {
	case deleted := <-deletedBeforeCallback:
		if !deleted {
			t.Fatal("timeout callback ran before the pending request was deleted")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout callback was not called")
	}

	time.Sleep(20 * time.Millisecond)
	if callbackCount.Load() != 1 {
		t.Fatalf("timeout callback count = %d, want 1", callbackCount.Load())
	}
	if adapter.requests.Len() != 0 {
		t.Fatal("timed-out request remained pending")
	}
}

func TestClassicFinishRequestStopsTimeout(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	resolved := make(chan struct{}, 1)
	var resolveCount atomic.Int64
	var timeoutCalled atomic.Bool
	request := &RedisRequest{
		Type: redis.REMOTE_JOIN,
		Resolve: func(*types.Slice[any]) {
			resolveCount.Add(1)
			resolved <- struct{}{}
		},
	}

	adapter.registerRequest("request", request, 100*time.Millisecond, func() {
		timeoutCalled.Store(true)
	})
	adapter.processResponse(request, &Response{RequestId: "request"})

	select {
	case <-resolved:
	case <-time.After(time.Second):
		t.Fatal("completed request was not resolved")
	}

	time.Sleep(150 * time.Millisecond)
	if resolveCount.Load() != 1 {
		t.Fatalf("resolve callback count = %d, want 1", resolveCount.Load())
	}
	if timeoutCalled.Load() {
		t.Fatal("completed request timeout callback was called")
	}
	if adapter.requests.Len() != 0 {
		t.Fatal("completed request retained pending state")
	}
}

func TestClassicFinishRequestResolvesOnce(t *testing.T) {
	adapter := MakeRedisAdapter().(*redisAdapter)
	var resolveCount atomic.Int64
	var timeoutCalled atomic.Bool
	request := &RedisRequest{
		Type: redis.REMOTE_JOIN,
		Resolve: func(*types.Slice[any]) {
			resolveCount.Add(1)
		},
	}
	adapter.registerRequest("request", request, time.Second, func() {
		timeoutCalled.Store(true)
	})

	const responseCount = 64
	var wg sync.WaitGroup
	for range responseCount {
		wg.Go(func() {
			adapter.processResponse(request, &Response{RequestId: "request"})
		})
	}
	wg.Wait()

	if resolveCount.Load() != 1 {
		t.Fatalf("resolve callback count = %d, want 1", resolveCount.Load())
	}
	if timeoutCalled.Load() {
		t.Fatal("completed request timeout callback was called")
	}
	if adapter.requests.Len() != 0 {
		t.Fatal("completed request retained pending state")
	}
}

func TestClassicCloseDoesNotWaitForInFlightMessage(t *testing.T) {
	goRedisClient := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress})
	t.Cleanup(func() { _ = goRedisClient.Close() })
	client := mustRedisClient(t, context.Background(), goRedisClient)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	local := &recordingLocalAdapter{
		Adapter:          socket.NewAdapter(nsp),
		broadcastStarted: make(chan struct{}),
		broadcastRelease: make(chan struct{}),
	}
	current := MakeRedisAdapter().(*redisAdapter)
	current.Adapter = local
	current.redisClient = client
	current.channel = "socket.io#/test#"
	current.uid = "self"
	current.parser = &recordingParser{packet: &Packet{
		Uid:    "sender",
		Packet: &parser.Packet{Type: parser.EVENT, Nsp: "/test"},
		Opts:   adapter.EncodeOptions(nil),
	}}
	messageDone := make(chan struct{})
	go func() {
		current.onMessage([]byte("payload"), current.channel)
		close(messageDone)
	}()
	<-local.broadcastStarted
	closed := make(chan struct{})
	go func() {
		current.Close()
		close(closed)
	}()

	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close waited for an in-flight local adapter call")
	}
	close(local.broadcastRelease)
	// The already-entered local call is allowed to finish independently.
	select {
	case <-messageDone:
	case <-time.After(time.Second):
		t.Fatal("in-flight broadcast did not finish after it was released")
	}
	if local.broadcasts != 1 {
		t.Fatalf("broadcast count = %d, want 1", local.broadcasts)
	}
}

func TestClassicCloseFromSynchronousCallbackDoesNotDeadlock(t *testing.T) {
	goRedisClient := rds.NewClient(&rds.Options{Addr: unavailableRedisAddress})
	t.Cleanup(func() { _ = goRedisClient.Close() })
	client := mustRedisClient(t, context.Background(), goRedisClient)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	local := &recordingLocalAdapter{Adapter: socket.NewAdapter(nsp), invokeClientCount: true}
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.Adapter = local
	adapter.redisClient = client
	adapter.ctx, adapter.cancel = context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		adapter.BroadcastWithAck(
			&parser.Packet{Type: parser.EVENT},
			&socket.BroadcastOptions{Flags: &socket.BroadcastFlags{Local: true}},
			func(uint64) { adapter.Close() },
			func([]any, error) {},
		)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Close deadlocked inside a synchronous adapter callback")
	}
}
