package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type preparedCodecValue struct {
	jsonCalls, msgpackCalls int
	changed                 bool
}

func (v *preparedCodecValue) MarshalJSON() ([]byte, error) {
	v.jsonCalls++
	if v.changed {
		return nil, errors.New("encoding ran after preparation")
	}
	return json.Marshal("before")
}

func (v *preparedCodecValue) MarshalMsgpack() ([]byte, error) {
	v.msgpackCalls++
	if v.changed {
		return nil, errors.New("encoding ran after preparation")
	}
	return msgpack.Marshal("before")
}

func TestPreparePublishEncodesBeforeDatabaseIO(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	pool, err := pgxpool.New(ctx, "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	client := mustNewPostgresClient(t, ctx, pool)
	t.Cleanup(client.Close)
	for _, test := range []struct {
		name          string
		threshold     int
		binary        bool
		json, msgpack int
	}{
		{"NOTIFY", 8000, false, 1, 0},
		{"size attachment", 1, false, 1, 1},
		{"binary attachment", 8000, true, 0, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			opts := DefaultPostgresAdapterOptions()
			opts.SetPayloadThreshold(test.threshold)
			current := NewPostgresAdapter(socket.NewNamespace(socket.NewServer(nil, nil), "/test"), client, opts).(*postgresAdapter)
			t.Cleanup(current.Close)
			value := &preparedCodecValue{}
			packet := []any{"event", value}
			if test.binary {
				packet = append(packet, []byte{1, 2})
			}
			publish, prepareErr := current.PreparePublish(&ClusterMessage{Type: adapter.SERVER_SIDE_EMIT,
				Data: &adapter.ServerSideEmitMessage{Packet: packet}})
			if prepareErr != nil || publish == nil {
				t.Fatalf("preparation attempted database I/O: %v", prepareErr)
			}
			if value.jsonCalls != test.json || value.msgpackCalls != test.msgpack {
				t.Fatalf("codec calls = %d/%d, want %d/%d", value.jsonCalls, value.msgpackCalls, test.json, test.msgpack)
			}
			value.changed = true
			if _, sendErr := publish(); !errors.Is(sendErr, context.Canceled) {
				t.Fatalf("send error = %v, want canceled database context", sendErr)
			}
			if value.jsonCalls != test.json || value.msgpackCalls != test.msgpack {
				t.Fatal("send encoded the payload again")
			}
		})
	}
}
