package adapter

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	baseadapter "github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/unix/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func newTestNamespace(name string) socket.Namespace {
	return socket.NewNamespace(socket.NewServer(nil, nil), name)
}

func newTestSocketPath(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("", "sio-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return filepath.Join(directory, "socket")
}

func newTestUnixClient(t *testing.T, socketPath string) *unix.UnixClient {
	t.Helper()
	client, err := unix.NewUnixClient(context.Background(), socketPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func encodeTestMessage(t *testing.T, message *baseadapter.ClusterMessage) []byte {
	t.Helper()
	payload, err := baseadapter.EncodeClusterMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func unixBuilderState(builder *UnixAdapterBuilder) (listening, retrying bool) {
	builder.mu.Lock()
	defer builder.mu.Unlock()
	return builder.listening, builder.retrying
}

func TestUnixAdapterBuilderDispatchesExactNamespace(t *testing.T) {
	client := newTestUnixClient(t, newTestSocketPath(t))
	builder := &UnixAdapterBuilder{Unix: client}
	firstNsp := newTestNamespace("/first")
	secondNsp := newTestNamespace("/second")
	first := builder.New(firstNsp).(*unixAdapter)
	second := builder.New(secondNsp).(*unixAdapter)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	var firstCalls, secondCalls atomic.Int64
	if err := firstNsp.On("event", func(...any) { firstCalls.Add(1) }); err != nil {
		t.Fatal(err)
	}
	if err := secondNsp.On("event", func(...any) { secondCalls.Add(1) }); err != nil {
		t.Fatal(err)
	}

	builder.dispatchMessage(encodeTestMessage(t, &baseadapter.ClusterMessage{
		Uid:  "peer",
		Nsp:  "/first",
		Type: baseadapter.SERVER_SIDE_EMIT,
		Data: &baseadapter.ServerSideEmitMessage{Packet: []any{"event"}},
	}))
	if firstCalls.Load() != 1 || secondCalls.Load() != 0 {
		t.Fatalf("namespace calls = (%d, %d), want (1, 0)", firstCalls.Load(), secondCalls.Load())
	}

	for _, message := range []*baseadapter.ClusterMessage{
		{Uid: first.Uid(), Nsp: "/first", Type: baseadapter.SERVER_SIDE_EMIT, Data: &baseadapter.ServerSideEmitMessage{Packet: []any{"event"}}},
		{Uid: "peer", Nsp: "", Type: baseadapter.SERVER_SIDE_EMIT, Data: &baseadapter.ServerSideEmitMessage{Packet: []any{"event"}}},
		{Uid: "peer", Nsp: "/missing", Type: baseadapter.SERVER_SIDE_EMIT, Data: &baseadapter.ServerSideEmitMessage{Packet: []any{"event"}}},
	} {
		builder.dispatchMessage(encodeTestMessage(t, message))
	}
	if firstCalls.Load() != 1 || secondCalls.Load() != 0 {
		t.Fatalf("ignored messages changed calls to (%d, %d)", firstCalls.Load(), secondCalls.Load())
	}
}

func TestUnixAdapterIgnoresStaleMessageAfterClose(t *testing.T) {
	client := newTestUnixClient(t, newTestSocketPath(t))
	builder := &UnixAdapterBuilder{Unix: client}
	nsp := newTestNamespace("/test")
	instance := builder.New(nsp).(*unixAdapter)

	stale, ok := builder.namespaceToAdapters.Load("/test")
	if !ok {
		t.Fatal("namespace adapter was not registered")
	}
	var calls atomic.Int64
	if err := nsp.On("event", func(...any) { calls.Add(1) }); err != nil {
		t.Fatal(err)
	}

	instance.Close()
	stale.OnMessage(&baseadapter.ClusterMessage{
		Uid:  "peer",
		Nsp:  "/test",
		Type: baseadapter.SERVER_SIDE_EMIT,
		Data: &baseadapter.ServerSideEmitMessage{Packet: []any{"event"}},
	}, "")
	if calls.Load() != 0 {
		t.Fatalf("closed adapter handled %d stale messages, want 0", calls.Load())
	}
}

func TestUnixAdapterBuilderRejectsMalformedMessageBeforeHeartbeat(t *testing.T) {
	client := newTestUnixClient(t, newTestSocketPath(t))
	builder := &UnixAdapterBuilder{Unix: client}
	instance := builder.New(newTestNamespace("/test")).(*unixAdapter)
	t.Cleanup(instance.Close)

	var errorsReported atomic.Int64
	if err := client.On("error", func(...any) { errorsReported.Add(1) }); err != nil {
		t.Fatal(err)
	}

	builder.dispatchMessage([]byte(`{"uid":"peer","nsp":"/test","type":999}`))
	builder.dispatchMessage([]byte(`{invalid}`))
	if errorsReported.Load() != 2 {
		t.Fatalf("reported errors = %d, want 2", errorsReported.Load())
	}
	if count, err := instance.ServerCount(); err != nil || count != 1 {
		t.Fatalf("ServerCount() = (%d, %v), want (1, nil)", count, err)
	}
}

func TestUnixAdapterBuilderRetriesFailedListen(t *testing.T) {
	root := filepath.Dir(newTestSocketPath(t))
	socketPath := filepath.Join(root, "missing", "socket.io.sock")
	client := newTestUnixClient(t, socketPath)
	builder := &UnixAdapterBuilder{Unix: client}

	first := builder.New(newTestNamespace("/first")).(*unixAdapter)
	t.Cleanup(first.Close)
	if listening, _ := unixBuilderState(builder); listening {
		t.Fatal("builder marked a failed listener as active")
	}

	if err := os.Mkdir(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatal(err)
	}
	second := builder.New(newTestNamespace("/second")).(*unixAdapter)
	t.Cleanup(second.Close)
	if listening, _ := unixBuilderState(builder); !listening {
		t.Fatal("builder did not retry Listen after the initial failure")
	}
}

func TestUnixAdapterBuilderRetriesFailedListenWithoutAnotherNamespace(t *testing.T) {
	root := filepath.Dir(newTestSocketPath(t))
	socketPath := filepath.Join(root, "missing", "socket.io.sock")
	client := newTestUnixClient(t, socketPath)
	builder := &UnixAdapterBuilder{Unix: client}
	errorsReported := make(chan error, 1)
	if err := client.On("error", func(args ...any) {
		if err, ok := args[0].(error); ok {
			select {
			case errorsReported <- err:
			default:
			}
		}
	}); err != nil {
		t.Fatal(err)
	}
	nsp := newTestNamespace("/test")
	received := make(chan struct{}, 1)
	if err := nsp.On("event", func(...any) { received <- struct{}{} }); err != nil {
		t.Fatal(err)
	}

	instance := builder.New(nsp).(*unixAdapter)
	t.Cleanup(instance.Close)
	select {
	case err := <-errorsReported:
		if err == nil {
			t.Fatal("initial Listen failure emitted a nil error")
		}
	case <-time.After(time.Second):
		t.Fatal("initial Listen failure did not emit an error")
	}
	if listening, retrying := unixBuilderState(builder); listening || !retrying {
		t.Fatalf("builder state after failed Listen = (%t, %t), want (false, true)", listening, retrying)
	}

	if err := os.Mkdir(filepath.Dir(socketPath), 0o755); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		listening, retrying := unixBuilderState(builder)
		if listening && !retrying {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("builder did not recover from failed Listen: listening=%t retrying=%t", listening, retrying)
		}
		time.Sleep(10 * time.Millisecond)
	}

	sender := newTestUnixClient(t, socketPath)
	listenerPath := socketPath + "." + string(instance.Uid())
	if err := sender.Send(listenerPath, encodeTestMessage(t, &baseadapter.ClusterMessage{
		Uid:  "peer",
		Nsp:  "/test",
		Type: baseadapter.SERVER_SIDE_EMIT,
		Data: &baseadapter.ServerSideEmitMessage{Packet: []any{"event"}},
	})); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("recovered listener did not dispatch the message")
	}
}

func TestUnixAdapterBuilderContextCancellationClosesAdapters(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client, err := unix.NewUnixClient(ctx, newTestSocketPath(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	builder := &UnixAdapterBuilder{Unix: client}
	first := builder.New(newTestNamespace("/first")).(*unixAdapter)
	second := builder.New(newTestNamespace("/second")).(*unixAdapter)
	first.Init()
	second.Init()
	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for !first.isClosed.Load() || !second.isClosed.Load() || builder.namespaceToAdapters.Len() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf(
				"context cancellation did not close adapters: closed=(%t, %t), namespaces=%d",
				first.isClosed.Load(), second.isClosed.Load(), builder.namespaceToAdapters.Len(),
			)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestUnixAdapterBuilderPreCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, err := unix.NewUnixClient(ctx, newTestSocketPath(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })

	builder := &UnixAdapterBuilder{Unix: client}
	instance := builder.New(newTestNamespace("/test")).(*unixAdapter)
	instance.Init()

	deadline := time.Now().Add(2 * time.Second)
	for !instance.isClosed.Load() || builder.namespaceToAdapters.Len() != 0 {
		if time.Now().After(deadline) {
			t.Fatalf(
				"pre-canceled context left an active adapter: closed=%t, namespaces=%d",
				instance.isClosed.Load(), builder.namespaceToAdapters.Len(),
			)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if listening, retrying := unixBuilderState(builder); listening || retrying {
		t.Fatalf("pre-canceled builder state = (%t, %t), want (false, false)", listening, retrying)
	}
}

func TestUnixAdapterCloseKeepsSharedListenerAndReplacement(t *testing.T) {
	basePath := newTestSocketPath(t)
	client := newTestUnixClient(t, basePath)
	builder := &UnixAdapterBuilder{Unix: client}
	first := builder.New(newTestNamespace("/same")).(*unixAdapter)
	first.Init()
	second := builder.New(newTestNamespace("/same")).(*unixAdapter)
	listenerPath := basePath + "." + string(first.Uid())
	if !first.isClosed.Load() {
		t.Fatal("replaced adapter remained active")
	}

	first.Close()
	first.Close()
	current, ok := builder.namespaceToAdapters.Load("/same")
	if !ok || current != second {
		t.Fatal("closing a replaced adapter removed the active namespace adapter")
	}
	second.Close()
	if _, ok := builder.namespaceToAdapters.Load("/same"); ok {
		t.Fatal("closing the active adapter did not unregister it")
	}

	sender := newTestUnixClient(t, basePath)
	if err := sender.Send(listenerPath, encodeTestMessage(t, &baseadapter.ClusterMessage{
		Uid: "peer", Nsp: "/same", Type: baseadapter.HEARTBEAT,
	})); err != nil {
		t.Fatalf("shared listener was unavailable after adapter Close: %v", err)
	}
}

func TestUnixAdapterCloseRunsCleanupOnce(t *testing.T) {
	client := newTestUnixClient(t, newTestSocketPath(t))
	instance := NewUnixAdapter(newTestNamespace("/test"), client, nil).(*unixAdapter)
	var calls atomic.Int64
	instance.Cleanup(func() { calls.Add(1) })

	instance.Close()
	instance.Close()
	if calls.Load() != 1 {
		t.Fatalf("cleanup calls = %d, want 1", calls.Load())
	}

	instance.Cleanup(func() { calls.Add(1) })
	if calls.Load() != 2 {
		t.Fatalf("cleanup registered after Close was called %d times, want total 2", calls.Load())
	}
}

func TestUnixAdapterErrorHandlerCanReenterPublisher(t *testing.T) {
	socketPath := newTestSocketPath(t)
	deadPeerPath := socketPath + ".dead"
	listener, err := net.Listen("unix", deadPeerPath)
	if err != nil {
		t.Fatal(err)
	}
	unixListener, ok := listener.(*net.UnixListener)
	if !ok {
		_ = listener.Close()
		t.Fatalf("listener type = %T, want *net.UnixListener", listener)
	}
	unixListener.SetUnlinkOnClose(false)
	if err := unixListener.Close(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(deadPeerPath) })

	client := newTestUnixClient(t, socketPath)
	current := NewUnixAdapter(newTestNamespace("/test"), client, nil).(*unixAdapter)
	t.Cleanup(current.Close)
	var reentered atomic.Bool
	reentryDone := make(chan error, 1)
	if err := client.On("error", func(...any) {
		if reentered.CompareAndSwap(false, true) {
			_, err := current.PublishAndReturnOffset(&baseadapter.ClusterMessage{Type: baseadapter.HEARTBEAT})
			reentryDone <- err
		}
	}); err != nil {
		t.Fatal(err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := current.PublishAndReturnOffset(&baseadapter.ClusterMessage{Type: baseadapter.HEARTBEAT})
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("initial publish error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("initial publish deadlocked in the error handler")
	}
	select {
	case err := <-reentryDone:
		if err != nil {
			t.Fatalf("reentrant publish error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("error handler could not reenter the publisher")
	}
}
