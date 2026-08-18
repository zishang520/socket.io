package adapter

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	vk "github.com/valkey-io/valkey-go"
	baseadapter "github.com/zishang520/socket.io/adapters/adapter/v3"
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type recordingParser struct {
	decodeCalled bool
	packet       *Packet
}

func (p *recordingParser) Encode(any) ([]byte, error) { return nil, nil }

func (p *recordingParser) Decode(_ []byte, value any) error {
	p.decodeCalled = true
	if packet, ok := value.(**Packet); ok {
		*packet = p.packet
	}
	return nil
}

func TestValkeyAdapterOnMessageAcceptsNamespaceChannel(t *testing.T) {
	parser := &recordingParser{packet: &Packet{Uid: baseadapter.ServerId("sender")}}
	adapter := MakeValkeyAdapter().(*valkeyAdapter)
	adapter.channel = "socket.io#/#"
	adapter.uid = "sender"
	adapter.parser = parser

	adapter.onMessage(adapter.channel+"*", adapter.channel, []byte("payload"))

	if !parser.decodeCalled {
		t.Fatal("expected namespace channel message to be decoded")
	}
}

func canceledValkeyClient(t *testing.T) (*valkey.ValkeyClient, context.Context) {
	t.Helper()

	server := miniredis.RunT(t)
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{server.Addr()},
		DisableCache: true,
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
