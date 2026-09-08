package adapter

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	vk "github.com/valkey-io/valkey-go"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type classicJSONParser struct{}

func (classicJSONParser) Encode(value any) ([]byte, error) { return json.Marshal(value) }
func (classicJSONParser) Decode(payload []byte, value any) error {
	return json.Unmarshal(payload, value)
}

type nullAckAdapter struct{ socket.Adapter }

func (*nullAckAdapter) BroadcastWithAck(_ *parser.Packet, _ *socket.BroadcastOptions, clientCount func(uint64), ack socket.Ack) {
	clientCount(1)
	ack([]any{nil}, nil)
}

func TestClassicJSONAcknowledgementsPreserveNull(t *testing.T) {
	for _, test := range []struct {
		name         string
		requestType  valkey.RequestType
		responseType valkey.RequestType
		field        string
	}{
		{"server-side emit", valkey.SERVER_SIDE_EMIT, valkey.SERVER_SIDE_EMIT, "data"},
		{"broadcast with JSON parser", valkey.BROADCAST, valkey.BROADCAST_ACK, "packet"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			db := miniredis.RunT(t)
			client, err := vk.NewClient(vk.ClientOption{
				InitAddress: []string{db.Addr()}, DisableCache: true, AlwaysRESP2: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.Close)
			wrapped, err := valkey.NewValkeyClient(ctx, client)
			if err != nil {
				t.Fatal(err)
			}
			nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/null-ack")
			if err = nsp.On("event", func(args ...any) {
				args[len(args)-1].(socket.Ack)([]any{nil}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			current := MakeValkeyAdapter().(*valkeyAdapter)
			current.Adapter = &nullAckAdapter{Adapter: current.Adapter}
			current.SetValkey(wrapped)
			if test.requestType == valkey.BROADCAST {
				opts := DefaultValkeyAdapterOptions()
				opts.SetParser(classicJSONParser{})
				current.SetOpts(opts)
			}
			current.Construct(nsp)
			defer current.Close()

			subscription := wrapped.Subscribe(ctx, current.responseChannel)
			defer func() { _ = subscription.Close() }()
			request := &Request{Type: test.requestType, Uid: "remote", RequestId: "request"}
			if test.requestType == valkey.SERVER_SIDE_EMIT {
				request.Data = []any{"event"}
			} else {
				request.Packet = &parser.Packet{Type: parser.EVENT, Nsp: nsp.Name(), Data: []any{"event"}}
				request.Opts = adapter.EncodeOptions(nil)
			}
			payload, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			if err := wrapped.Publish(ctx, current.requestChannel, payload); err != nil {
				t.Fatal(err)
			}

			for {
				message, err := subscription.ReceiveMessage(ctx)
				if err != nil {
					t.Fatal(err)
				}
				var response map[string]json.RawMessage
				if err := json.Unmarshal([]byte(message.Message), &response); err != nil {
					t.Fatal(err)
				}
				var responseType valkey.RequestType
				if err := json.Unmarshal(response["type"], &responseType); err != nil {
					t.Fatal(err)
				}
				if responseType != test.responseType {
					continue // Broadcast reports the client count before the acknowledgement.
				}
				if got := string(response[test.field]); got != "null" {
					t.Fatalf("%s = %s, want null in %s", test.field, got, message.Message)
				}
				return
			}
		})
	}
}
