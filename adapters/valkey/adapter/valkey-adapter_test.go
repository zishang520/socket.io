package adapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	vk "github.com/valkey-io/valkey-go"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type recordingSocketAdapter struct {
	socket.Adapter
	broadcasts chan *parser.Packet
}

func (a *recordingSocketAdapter) Broadcast(packet *parser.Packet, _ *socket.BroadcastOptions) {
	a.broadcasts <- packet
}

func (a *recordingSocketAdapter) BroadcastWithAck(
	_ *parser.Packet,
	_ *socket.BroadcastOptions,
	clientCount func(uint64),
	ack socket.Ack,
) {
	clientCount(1)
	ack([]any{"ack"}, nil)
}

func newRecordingClassicAdapter(
	t *testing.T,
	client *valkey.ValkeyClient,
	nspName string,
) (*valkeyAdapter, *recordingSocketAdapter) {
	t.Helper()
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), nspName)
	recorder := &recordingSocketAdapter{
		Adapter:    socket.MakeAdapter(),
		broadcasts: make(chan *parser.Packet, 1),
	}
	a := MakeValkeyAdapter().(*valkeyAdapter)
	a.Adapter = recorder
	a.SetValkey(client)
	a.Construct(nsp)
	t.Cleanup(a.Close)
	return a, recorder
}

func canceledValkeyClient(t *testing.T) (*valkey.ValkeyClient, context.Context) {
	t.Helper()

	server := miniredis.RunT(t)
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{server.Addr()},
		DisableCache: true,
		AlwaysRESP2:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	valkeyClient, err := valkey.NewValkeyClient(ctx, client)
	if err != nil {
		t.Fatal(err)
	}
	return valkeyClient, ctx
}

func TestServerCountReturnsValkeyErrors(t *testing.T) {
	client, ctx := canceledValkeyClient(t)

	adapters := []struct {
		name        string
		serverCount func() (int64, error)
	}{
		{
			name: "classic",
			serverCount: (&valkeyAdapter{
				valkeyClient:   client,
				ctx:            ctx,
				requestChannel: "request",
			}).ServerCount,
		},
		{
			name: "sharded",
			serverCount: (&shardedValkeyAdapter{
				valkeyClient: client,
				ctx:          ctx,
				channel:      "channel",
			}).ServerCount,
		},
		{
			name: "streams",
			serverCount: (&valkeyStreamsAdapter{
				valkeyClient:  client,
				ctx:           ctx,
				opts:          DefaultValkeyStreamsAdapterOptions(),
				publicChannel: "channel",
			}).ServerCount,
		},
	}

	for _, tt := range adapters {
		t.Run(tt.name, func(t *testing.T) {
			count, err := tt.serverCount()
			if err == nil {
				t.Fatal("expected ServerCount error")
			}
			if count != 0 {
				t.Fatalf("ServerCount = %d, want 0", count)
			}
		})
	}
}

func TestClassicServerCountErrorsAreReturned(t *testing.T) {
	client, ctx := canceledValkeyClient(t)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	adapter := MakeValkeyAdapter().(*valkeyAdapter)
	adapter.Adapter = socket.NewAdapter(nsp)
	adapter.valkeyClient = client
	adapter.ctx = ctx
	adapter.requestChannel = "request"

	var allRoomsErr error
	adapter.AllRooms()(func(_ *types.Set[socket.Room], err error) {
		allRoomsErr = err
	})
	if !errors.Is(allRoomsErr, context.Canceled) {
		t.Fatalf("AllRooms() error = %v, want %v", allRoomsErr, context.Canceled)
	}

	var fetchSocketsErr error
	adapter.FetchSockets(nil)(func(_ []socket.SocketDetails, err error) {
		fetchSocketsErr = err
	})
	if !errors.Is(fetchSocketsErr, context.Canceled) {
		t.Fatalf("FetchSockets() error = %v, want %v", fetchSocketsErr, context.Canceled)
	}

	ackCalled := false
	err := adapter.ServerSideEmit([]any{"event", func([]any, error) {
		ackCalled = true
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ServerSideEmit() error = %v, want %v", err, context.Canceled)
	}
	if ackCalled {
		t.Fatal("acknowledgement was called after server count failed")
	}
	if adapter.requests.Len() != 0 {
		t.Fatal("request was stored after server count failed")
	}
}

func TestValkeyAdapterBroadcastsLocallyAndRemotely(t *testing.T) {
	server := miniredis.RunT(t)
	source, sourceRecorder := newRecordingClassicAdapter(
		t, newValkeyAdapterTestClient(t, server.Addr()), "/test",
	)
	_, remoteRecorder := newRecordingClassicAdapter(
		t, newValkeyAdapterTestClient(t, server.Addr()), "/test",
	)
	source.Broadcast(&parser.Packet{Type: parser.EVENT, Data: []any{"event"}}, nil)

	for name, recorder := range map[string]*recordingSocketAdapter{
		"local":  sourceRecorder,
		"remote": remoteRecorder,
	} {
		select {
		case packet := <-recorder.broadcasts:
			if packet.Nsp != "/test" {
				t.Fatalf("%s packet namespace = %q", name, packet.Nsp)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s broadcast was not delivered", name)
		}
	}
}

func TestValkeyAdapterRegistersAckBeforePublish(t *testing.T) {
	server := miniredis.RunT(t)
	source, _ := newRecordingClassicAdapter(
		t, newValkeyAdapterTestClient(t, server.Addr()), "/ack",
	)
	newRecordingClassicAdapter(
		t, newValkeyAdapterTestClient(t, server.Addr()), "/ack",
	)
	counts := make(chan uint64, 2)
	acks := make(chan []any, 2)
	source.BroadcastWithAck(
		&parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
		nil,
		func(count uint64) { counts <- count },
		func(args []any, _ error) { acks <- args },
	)

	for range 2 {
		select {
		case count := <-counts:
			if count != 1 {
				t.Fatalf("client count = %d", count)
			}
		case <-time.After(time.Second):
			t.Fatal("client count acknowledgement was lost")
		}
		select {
		case args := <-acks:
			if len(args) != 1 || args[0] != "ack" {
				t.Fatalf("acknowledgement = %#v", args)
			}
		case <-time.After(time.Second):
			t.Fatal("broadcast acknowledgement was lost")
		}
	}
}
