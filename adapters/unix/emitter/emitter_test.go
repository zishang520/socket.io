package emitter

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zishang520/socket.io/adapters/unix/v3"
	unixadapter "github.com/zishang520/socket.io/adapters/unix/v3/adapter"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func newEmitterTestSocketPath(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp("", "sio-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return filepath.Join(directory, "socket")
}

func TestEmitterUsesClientTransportAndSharedCodec(t *testing.T) {
	basePath := newEmitterTestSocketPath(t)
	receiver, err := unix.NewUnixClient(context.Background(), basePath, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = receiver.Close() })

	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/custom")
	builder := &unixadapter.UnixAdapterBuilder{Unix: receiver}
	instance := builder.New(nsp)
	t.Cleanup(instance.Close)
	received := make(chan []byte, 1)
	if onErr := nsp.On("event", func(args ...any) {
		if len(args) == 1 {
			if payload, ok := args[0].([]byte); ok {
				received <- payload
			}
		}
	}); onErr != nil {
		t.Fatal(onErr)
	}

	payload := bytes.Repeat([]byte{0xa5}, 128<<10)
	if err := NewEmitter(receiver).Of("custom").ServerSideEmit("event", payload); err != nil {
		t.Fatal(err)
	}

	select {
	case actual := <-received:
		if !bytes.Equal(actual, payload) {
			t.Fatal("received binary payload differs from emitted payload")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the emitted event")
	}
}

func TestEmitterNamespaces(t *testing.T) {
	for _, test := range []struct {
		name string
		got  string
		want string
	}{
		{name: "emitter defaults to root", got: NewEmitter(nil).broadcastOptions.Nsp, want: defaultNamespace},
		{name: "emitter preserves explicit empty", got: NewEmitter(nil, "").broadcastOptions.Nsp, want: ""},
		{name: "operator defaults to root", got: NewBroadcastOperator(nil, nil, nil, nil, nil).broadcastOptions.Nsp, want: defaultNamespace},
		{name: "operator preserves explicit empty", got: NewBroadcastOperator(nil, &BroadcastOptions{}, nil, nil, nil).broadcastOptions.Nsp, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.got != test.want {
				t.Fatalf("namespace = %q, want %q", test.got, test.want)
			}
		})
	}
}

func TestEmitterServerSideEmitRejectsAcknowledgements(t *testing.T) {
	emitter := NewEmitter(nil)
	err := emitter.ServerSideEmit("event", func([]any, error) {})
	if err != errAcknowledgementsNotSupported {
		t.Fatalf("ServerSideEmit() error = %v, want %v", err, errAcknowledgementsNotSupported)
	}
}
