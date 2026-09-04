package adapter

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	vk "github.com/valkey-io/valkey-go"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type recordedStreamMessage struct {
	message *adapter.ClusterMessage
	offset  adapter.Offset
}

type recordingValkeyStreamsClusterAdapter struct {
	adapter.ClusterAdapter
	messages chan recordedStreamMessage
}

func (a *recordingValkeyStreamsClusterAdapter) OnMessage(
	message *adapter.ClusterMessage,
	offset adapter.Offset,
) {
	a.messages <- recordedStreamMessage{message: message, offset: offset}
}

func TestValkeyStreamsPollerKeepsNewestRegistration(t *testing.T) {
	server := miniredis.RunT(t)
	rawClient := newValkeyRawClient(t, server.Addr())
	client := newStreamsValkeyClient(t, rawClient, nil)
	socketServer := socket.NewServer(nil, nil)
	opts := DefaultValkeyStreamsAdapterOptions()
	opts.SetBlockTimeInMs(20)

	first := NewValkeyStreamsAdapter(
		socket.NewNamespace(socketServer, "/shared"),
		client,
		opts,
	).(*valkeyStreamsAdapter)
	second := NewValkeyStreamsAdapter(
		socket.NewNamespace(socketServer, "/shared"),
		client,
		opts,
	).(*valkeyStreamsAdapter)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	if first.streamPoller != second.streamPoller {
		t.Fatal("adapters for the same server and stream did not share a poller")
	}
	poller := first.streamPoller
	if current, ok := poller.adapters.Load("/shared"); !ok || current != second {
		t.Fatalf("active registration = %#v, want second adapter", current)
	}

	first.Close()
	if current, ok := poller.adapters.Load("/shared"); !ok || current != second {
		t.Fatalf("active registration = %#v, want second adapter", current)
	}
	if poller.ctx.Err() != nil {
		t.Fatal("stale adapter stopped the shared poller")
	}
	second.Close()
	select {
	case <-poller.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("last adapter did not stop the shared poller")
	}
}

func TestValkeyStreamsPollerFreezesPrimaryTailBeforeReadingSubClient(t *testing.T) {
	primaryServer := miniredis.RunT(t)
	subServer := miniredis.RunT(t)
	primary := newValkeyRawClient(t, primaryServer.Addr())
	sub := newValkeyRawClient(t, subServer.Addr())
	client := newStreamsValkeyClient(t, primary, sub)

	fields := map[string]string{
		"uid":  "remote",
		"nsp":  "/tail",
		"type": strconv.Itoa(int(adapter.HEARTBEAT)),
	}
	xaddAt(t, sub, "stream", "1-0", fields)
	xaddAt(t, primary, "stream", "2-0", fields)

	streamAdapter := MakeValkeyStreamsAdapter().(*valkeyStreamsAdapter)
	recorder := &recordingValkeyStreamsClusterAdapter{
		ClusterAdapter: streamAdapter.ClusterAdapter,
		messages:       make(chan recordedStreamMessage, 2),
	}
	streamAdapter.ClusterAdapter = recorder
	streamAdapter.SetValkey(client)
	opts := DefaultValkeyStreamsAdapterOptions()
	opts.SetStreamName("stream")
	opts.SetBlockTimeInMs(20)
	streamAdapter.SetOpts(opts)
	streamAdapter.Construct(socket.NewNamespace(socket.NewServer(nil, nil), "/tail"))
	t.Cleanup(streamAdapter.Close)

	xaddAt(t, sub, "stream", "3-0", fields)
	select {
	case got := <-recorder.messages:
		if got.offset != "3-0" || got.message.Nsp != "/tail" {
			t.Fatalf("delivered message = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("entry appended after construction was not delivered")
	}
}

func TestValkeyStreamsPollerRetriesInitialTailInBackground(t *testing.T) {
	primaryServer := miniredis.RunT(t)
	primary, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{primaryServer.Addr()},
		DisableCache: true,
		DisableRetry: true,
		AlwaysRESP2:  true,
		Dialer:       net.Dialer{Timeout: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(primary.Close)

	subServer := miniredis.RunT(t)
	sub := newValkeyRawClient(t, subServer.Addr())
	fields := map[string]string{
		"uid":  "remote",
		"nsp":  "/retry",
		"type": strconv.Itoa(int(adapter.HEARTBEAT)),
	}
	xaddAt(t, primary, "stream", "2-0", fields)
	xaddAt(t, sub, "stream", "1-0", fields)
	primaryServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client, err := valkey.NewValkeyClientWithSub(ctx, primary, sub)
	if err != nil {
		t.Fatal(err)
	}

	streamAdapter := MakeValkeyStreamsAdapter().(*valkeyStreamsAdapter)
	recorder := &recordingValkeyStreamsClusterAdapter{
		ClusterAdapter: streamAdapter.ClusterAdapter,
		messages:       make(chan recordedStreamMessage, 2),
	}
	streamAdapter.ClusterAdapter = recorder
	streamAdapter.SetValkey(client)
	opts := DefaultValkeyStreamsAdapterOptions()
	opts.SetStreamName("stream")
	opts.SetBlockTimeInMs(20)
	streamAdapter.SetOpts(opts)
	socketServer := socket.NewServer(nil, nil)
	var reentrant ValkeyStreamsAdapter
	reentrantCreated := make(chan struct{})
	t.Cleanup(func() {
		if reentrant != nil {
			reentrant.Close()
		}
	})
	if err := client.On("error", func(...any) {
		reentrant = NewValkeyStreamsAdapter(
			socket.NewNamespace(socketServer, "/reentrant"),
			client,
			opts,
		)
		close(reentrantCreated)
	}); err != nil {
		t.Fatal(err)
	}

	constructed := make(chan struct{})
	go func() {
		streamAdapter.Construct(socket.NewNamespace(socketServer, "/retry"))
		close(constructed)
	}()
	select {
	case <-reentrantCreated:
	case <-time.After(time.Second):
		cancel()
		<-constructed
		streamAdapter.Close()
		t.Fatal("initial-tail error listener was not called")
	}
	<-constructed
	t.Cleanup(streamAdapter.Close)

	if err := primaryServer.Restart(); err != nil {
		t.Fatal(err)
	}
	xaddAt(t, sub, "stream", "3-0", fields)

	select {
	case got := <-recorder.messages:
		if got.offset != "3-0" {
			t.Fatalf("delivered offset = %q, want 3-0", got.offset)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("poller did not recover after the primary became available")
	}
}

func TestValkeyStreamsPollerReentrantCloseOnInitialTailError(t *testing.T) {
	primaryServer := miniredis.RunT(t)
	primary, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{primaryServer.Addr()},
		DisableCache: true,
		DisableRetry: true,
		AlwaysRESP2:  true,
		Dialer:       net.Dialer{Timeout: 50 * time.Millisecond},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(primary.Close)
	primaryServer.Close()

	subServer := miniredis.RunT(t)
	client := newStreamsValkeyClient(t, primary, newValkeyRawClient(t, subServer.Addr()))
	current := MakeValkeyStreamsAdapter().(*valkeyStreamsAdapter)
	current.SetValkey(client)
	opts := DefaultValkeyStreamsAdapterOptions()
	opts.SetBlockTimeInMs(20)
	current.SetOpts(opts)
	closed := make(chan struct{}, 1)
	if err := client.On("error", func(...any) {
		current.Close()
		closed <- struct{}{}
	}); err != nil {
		t.Fatal(err)
	}

	current.Construct(socket.NewNamespace(socket.NewServer(nil, nil), "/reentrant-close"))
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("initial-tail error listener was not called")
	}
	poller := current.streamPoller
	if poller == nil {
		t.Fatal("initial-tail error was reported before the poller was assigned")
	}
	if current.ctx.Err() == nil || poller.ctx.Err() == nil {
		t.Fatal("reentrant Close did not stop the adapter and its stream poller")
	}

	valkeyStreamsPollers.mu.Lock()
	registered := valkeyStreamsPollers.groups[poller.key] == poller
	valkeyStreamsPollers.mu.Unlock()
	if registered {
		t.Fatal("reentrant Close left the stream poller registered")
	}
}
