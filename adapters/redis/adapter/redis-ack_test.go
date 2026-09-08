package adapter

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type classicJSONParser struct{}

func (classicJSONParser) Encode(value any) ([]byte, error) { return json.Marshal(value) }
func (classicJSONParser) Decode(payload []byte, value any) error {
	return json.Unmarshal(payload, value)
}

func TestClassicJSONAcknowledgementsPreserveNull(t *testing.T) {
	for _, test := range []struct {
		name         string
		requestType  redis.RequestType
		responseType redis.RequestType
		field        string
	}{
		{"server-side emit", redis.SERVER_SIDE_EMIT, redis.SERVER_SIDE_EMIT, "data"},
		{"broadcast with JSON parser", redis.BROADCAST, redis.BROADCAST_ACK, "packet"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			db := miniredis.RunT(t)
			client := rds.NewClient(&rds.Options{Addr: db.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			wrapped := mustRedisClient(t, ctx, client)
			nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/null-ack")
			if err := nsp.On("event", func(args ...any) {
				args[len(args)-1].(socket.Ack)([]any{nil}, nil)
			}); err != nil {
				t.Fatal(err)
			}
			current := MakeRedisAdapter().(*redisAdapter)
			current.Adapter = &clusterResponseAdapter{Adapter: current.Adapter, ack: []any{nil}}
			current.SetRedis(wrapped)
			if test.requestType == redis.BROADCAST {
				opts := DefaultRedisAdapterOptions()
				opts.SetParser(classicJSONParser{})
				current.SetOpts(opts)
			}
			current.Construct(nsp)
			defer current.Close()

			subscription := client.Subscribe(ctx, current.responseChannel)
			defer func() { _ = subscription.Close() }()
			if _, err := subscription.Receive(ctx); err != nil {
				t.Fatal(err)
			}
			messages := subscription.Channel()
			request := &Request{Type: test.requestType, Uid: "remote", RequestId: "request"}
			if test.requestType == redis.SERVER_SIDE_EMIT {
				request.Data = []any{"event"}
			} else {
				request.Packet = &parser.Packet{Type: parser.EVENT, Nsp: nsp.Name(), Data: []any{"event"}}
				request.Opts = adapter.EncodeOptions(nil)
			}
			payload, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			current.onRequest(payload, current.requestChannel)

			for {
				select {
				case message := <-messages:
					var response map[string]json.RawMessage
					if err := json.Unmarshal([]byte(message.Payload), &response); err != nil {
						t.Fatal(err)
					}
					var responseType redis.RequestType
					if err := json.Unmarshal(response["type"], &responseType); err != nil {
						t.Fatal(err)
					}
					if responseType != test.responseType {
						continue // Broadcast reports the client count before the acknowledgement.
					}
					if got := string(response[test.field]); got != "null" {
						t.Fatalf("%s = %s, want null in %s", test.field, got, message.Payload)
					}
					return
				case <-ctx.Done():
					t.Fatal("timed out waiting for the null acknowledgement")
				}
			}
		})
	}
}
