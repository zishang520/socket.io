package adapter

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/postgres/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func mustNewPostgresClient(t *testing.T, ctx context.Context, pool *pgxpool.Pool) *postgres.PostgresClient {
	t.Helper()
	client, err := postgres.NewPostgresClient(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

type attachmentAcquireTracer struct {
	deadline chan<- time.Duration
}

func (t *attachmentAcquireTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	var remaining time.Duration
	if deadline, ok := ctx.Deadline(); ok {
		remaining = time.Until(deadline)
	}
	select {
	case t.deadline <- remaining:
	default:
	}
	return ctx
}

func (*attachmentAcquireTracer) TraceAcquireEnd(context.Context, *pgxpool.Pool, pgxpool.TraceAcquireEndData) {
}

func (*attachmentAcquireTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}

func (*attachmentAcquireTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestNotificationMessage_Marshal(t *testing.T) {
	t.Run("with attachment", func(t *testing.T) {
		msg := &NotificationMessage{
			Uid:          "server1",
			Type:         adapter.BROADCAST,
			AttachmentId: "12345",
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var restored NotificationMessage
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if restored.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", restored.Uid)
		}
		if restored.AttachmentId != "12345" {
			t.Fatalf("Expected attachmentId '12345', got %s", restored.AttachmentId)
		}
	})

	t.Run("without attachment", func(t *testing.T) {
		msg := &NotificationMessage{
			Uid:  "server1",
			Type: adapter.HEARTBEAT,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if len(data) == 0 {
			t.Fatal("Expected non-empty JSON")
		}
	})
}

func TestPostgresAdapter_MakePostgresAdapter(t *testing.T) {
	a := MakePostgresAdapter()
	if a == nil {
		t.Fatal("Expected non-nil adapter")
	}
}

func TestPostgresAdapter_SetChannel(t *testing.T) {
	a := MakePostgresAdapter()
	a.SetChannel("socket.io#/")
	pa := a.(*postgresAdapter)
	if pa.channel != "socket.io#/" {
		t.Fatalf("Expected channel 'socket.io#/', got %s", pa.channel)
	}
}

func TestPostgresAdapter_SetOptsTypedNil(t *testing.T) {
	a := MakePostgresAdapter().(*postgresAdapter)
	var opts *PostgresAdapterOptions
	a.SetOpts(opts)
}

func TestNewPostgresAdapterDefaults(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	pool, err := pgxpool.New(ctx, "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	client := mustNewPostgresClient(t, ctx, pool)
	t.Cleanup(func() {
		client.Close()
		pool.Close()
	})
	a := NewPostgresAdapter(socket.NewNamespace(socket.NewServer(nil, nil), "/test"), client, nil).(*postgresAdapter)
	t.Cleanup(a.Close)

	if a.channel != DefaultChannelPrefix+"#/test" {
		t.Fatalf("channel = %q", a.channel)
	}
	if a.opts.TableName() != DefaultTableName {
		t.Fatalf("tableName = %q", a.opts.TableName())
	}
	if a.opts.PayloadThreshold() != DefaultPayloadThreshold {
		t.Fatalf("payloadThreshold = %d", a.opts.PayloadThreshold())
	}
}

func TestPostgresAdapter_OnNotificationUsesErrorHandler(t *testing.T) {
	var received error
	opts := DefaultPostgresAdapterOptions()
	opts.SetErrorHandler(func(err error) {
		received = err
	})

	a := MakePostgresAdapter().(*postgresAdapter)
	a.SetOpts(opts)
	a.OnNotification("invalid")
	if received == nil {
		t.Fatal("expected the configured error handler to be called")
	}
}

func TestPostgresAdapterPublishErrorHandlerCanReenterPublisher(t *testing.T) {
	pool, err := pgxpool.New(context.Background(), "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client := mustNewPostgresClient(t, ctx, pool)
	t.Cleanup(client.Close)

	var current *postgresAdapter
	var reentered atomic.Bool
	reentryDone := make(chan error, 1)
	opts := DefaultPostgresAdapterOptions()
	opts.SetErrorHandler(func(error) {
		if reentered.CompareAndSwap(false, true) {
			_, err := current.PublishAndReturnOffset(&ClusterMessage{Type: adapter.HEARTBEAT})
			reentryDone <- err
		}
	})
	current = NewPostgresAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/test"),
		client,
		opts,
	).(*postgresAdapter)
	t.Cleanup(current.Close)

	result := make(chan error, 1)
	go func() {
		_, err := current.PublishAndReturnOffset(&ClusterMessage{Type: adapter.HEARTBEAT})
		result <- err
	}()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("initial publish error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("initial publish deadlocked in the error handler")
	}
	select {
	case err := <-reentryDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("reentrant publish error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("error handler could not reenter the publisher")
	}
}

func TestPostgresAdapter_AttachmentUsesNotificationContext(t *testing.T) {
	config, err := pgxpool.ParseConfig("postgres://root@127.0.0.1/socket_io_test?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	acquireDeadline := make(chan time.Duration, 1)
	config.ConnConfig.Tracer = &attachmentAcquireTracer{deadline: acquireDeadline}
	config.ConnConfig.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	clientCtx, cancelClient := context.WithCancel(context.Background())
	client := mustNewPostgresClient(t, clientCtx, pool)
	a := NewPostgresAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/test"),
		client,
		nil,
	).(*postgresAdapter)
	t.Cleanup(func() {
		cancelClient()
		a.Close()
		client.Close()
		pool.Close()
	})

	notificationCtx, cancelNotification := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := a.decodeReceivedNotification(notificationCtx, &NotificationMessage{
			Uid:          "other-node",
			Type:         adapter.BROADCAST,
			AttachmentId: "1",
		})
		result <- err
	}()

	select {
	case remaining := <-acquireDeadline:
		if remaining <= 0 || remaining > postgres.DefaultOperationTimeout {
			t.Fatalf("attachment query deadline remaining = %s, want (0, %s]", remaining, postgres.DefaultOperationTimeout)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("attachment query was not attempted")
	}

	cancelNotification()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("attachment query returned %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("attachment query did not stop with the notification context")
	}
}

func TestPostgresAdapterCleanupAfterClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool, err := pgxpool.New(ctx, "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	client := mustNewPostgresClient(t, ctx, pool)
	a := NewPostgresAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/test"),
		client,
		nil,
	).(*postgresAdapter)
	t.Cleanup(func() {
		client.Close()
		pool.Close()
	})

	a.Close()
	cleaned := make(chan struct{})
	a.Cleanup(func() {
		close(cleaned)
	})

	select {
	case <-cleaned:
	case <-time.After(time.Second):
		t.Fatal("cleanup registered after Close was not called")
	}
}

func TestPostgresAdapterStopsPublishingBeforeCleanup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	pool, err := pgxpool.New(ctx, "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	client := mustNewPostgresClient(t, ctx, pool)
	a := NewPostgresAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/test"),
		client,
		nil,
	).(*postgresAdapter)
	t.Cleanup(func() {
		client.Close()
		pool.Close()
	})

	cleanupStarted := make(chan struct{})
	releaseCleanup := make(chan struct{})
	a.Cleanup(func() {
		close(cleanupStarted)
		<-releaseCleanup
	})
	closeDone := make(chan struct{})
	go func() {
		a.Close()
		close(closeDone)
	}()
	select {
	case <-cleanupStarted:
	case <-time.After(time.Second):
		close(releaseCleanup)
		t.Fatal("cleanup did not start")
	}

	if _, err := a.PublishAndReturnOffset(&ClusterMessage{Type: adapter.HEARTBEAT}); !errors.Is(err, adapter.ErrAdapterClosed) {
		close(releaseCleanup)
		t.Fatalf("publish during cleanup error = %v, want ErrAdapterClosed", err)
	}
	close(releaseCleanup)
	select {
	case <-closeDone:
	case <-time.After(time.Second):
		t.Fatal("Close did not finish after cleanup returned")
	}
}

func TestPostgresAdapter_OnNotificationNamespace(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	pool, err := pgxpool.New(ctx, "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	client := mustNewPostgresClient(t, ctx, pool)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	a := NewPostgresAdapter(nsp, client, nil).(*postgresAdapter)
	t.Cleanup(func() {
		a.Close()
		client.Close()
		pool.Close()
	})

	received := 0
	if err := nsp.On("probe", func(...any) {
		received++
	}); err != nil {
		t.Fatal(err)
	}

	a.OnNotification(`{"uid":"node","type":9,"data":{"packet":["probe"]}}`)
	if received != 0 {
		t.Fatal("server message without namespace must be ignored")
	}

	a.OnNotification(`{"uid":"other-node","nsp":"/other","type":9,"data":{"packet":["probe"]}}`)
	if received != 0 {
		t.Fatal("message from another namespace must be ignored")
	}
	if count, err := a.ServerCount(); err != nil || count != 1 {
		t.Fatalf("message from another namespace changed the server count: %d", count)
	}

	a.OnNotification(`{"uid":"emitter","type":9,"data":{"packet":["probe"]}}`)
	if received != 1 {
		t.Fatal("emitter message without namespace must use the channel namespace")
	}
}

func decodeJSONNotification(t *testing.T, a *postgresAdapter, payload []byte) *ClusterResponse {
	t.Helper()

	var notification NotificationMessage
	if err := json.Unmarshal(payload, &notification); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	message, err := a.decodeNotification(&notification)
	if err != nil {
		t.Fatalf("decodeNotification failed: %v", err)
	}
	return message
}

func TestPostgresAdapter_JsonRoundTrip(t *testing.T) {
	a := MakePostgresAdapter()
	pa := a.(*postgresAdapter)

	t.Run("ClusterMessage JSON matches Node.js format", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Uid:  "server1",
			Nsp:  "/",
			Type: adapter.BROADCAST,
			Data: &adapter.BroadcastMessage{
				Opts: &adapter.PacketOptions{
					Rooms: []socket.Room{"room1"},
				},
			},
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		// Verify Node.js compatible field names
		var raw NotificationMessage
		if unmarshalErr := json.Unmarshal(payload, &raw); unmarshalErr != nil {
			t.Fatalf("Failed to parse: %v", unmarshalErr)
		}
		if raw.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %v", raw.Uid)
		}
		if raw.Type != adapter.BROADCAST {
			t.Fatalf("Expected type %d, got %v", adapter.BROADCAST, raw.Type)
		}

		// Verify decode roundtrip
		decoded := decodeJSONNotification(t, pa, payload)
		if decoded.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", decoded.Uid)
		}
		if decoded.Type != adapter.BROADCAST {
			t.Fatalf("Expected type BROADCAST, got %v", decoded.Type)
		}
	})

	t.Run("heartbeat without data", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Uid:  "server1",
			Nsp:  "/",
			Type: adapter.HEARTBEAT,
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		decoded := decodeJSONNotification(t, pa, payload)
		if decoded.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", decoded.Uid)
		}
		if decoded.Type != adapter.HEARTBEAT {
			t.Fatalf("Expected type HEARTBEAT, got %v", decoded.Type)
		}
		if decoded.Data != nil {
			t.Fatal("Expected nil data for heartbeat")
		}
	})

	t.Run("heartbeat with null data", func(t *testing.T) {
		payload := []byte(`{"uid":"server1","nsp":"/","type":2,"data":null}`)

		decoded := decodeJSONNotification(t, pa, payload)
		if decoded.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", decoded.Uid)
		}
		if decoded.Data != nil {
			t.Fatal("Expected nil data for null data field")
		}
	})
}

func TestPostgresAdapter_MsgpackRoundTrip(t *testing.T) {
	a := MakePostgresAdapter()
	pa := a.(*postgresAdapter)

	t.Run("encode and decode msgpack", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Uid:  "server1",
			Nsp:  "/",
			Type: adapter.SOCKETS_JOIN,
			Data: &adapter.SocketsJoinLeaveMessage{
				Opts: &adapter.PacketOptions{
					Rooms: []socket.Room{"room1"},
				},
				Rooms: []socket.Room{"target-room"},
			},
		}

		wireMessage := *msg
		wireMessage.Data, _ = postgres.MarshalAdapterData(msg.Data)
		encoded, err := utils.MsgPack().Encode(&wireMessage)
		if err != nil {
			t.Fatalf("msgpack encode failed: %v", err)
		}

		// Decode with decodeMsgpack
		decoded, err := pa.decodeMsgpack(encoded)
		if err != nil {
			t.Fatalf("decodeMsgpack failed: %v", err)
		}

		if decoded.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", decoded.Uid)
		}
		if decoded.Type != adapter.SOCKETS_JOIN {
			t.Fatalf("Expected type SOCKETS_JOIN, got %v", decoded.Type)
		}
	})
}

func TestPostgresAdapter_DecodeNodeResponses(t *testing.T) {
	pa := MakePostgresAdapter().(*postgresAdapter)

	for _, test := range []struct {
		name        string
		messageType adapter.MessageType
		payload     string
		packet      any
	}{
		{
			name:        "server-side emit response",
			messageType: adapter.SERVER_SIDE_EMIT_RESPONSE,
			payload:     `{"uid":"node","nsp":"/","type":10,"data":{"requestId":"request","packet":"response"}}`,
			packet:      "response",
		},
		{
			name:        "broadcast acknowledgement",
			messageType: adapter.BROADCAST_ACK,
			payload:     `{"uid":"node","nsp":"/","type":12,"data":{"requestId":"request","packet":null}}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := decodeJSONNotification(t, pa, []byte(test.payload))
			if message.Type != test.messageType {
				t.Fatalf("expected type %d, got %d", test.messageType, message.Type)
			}

			var packet any
			switch data := message.Data.(type) {
			case *adapter.ServerSideEmitResponse:
				packet = data.Packet
			case *adapter.BroadcastAck:
				packet = data.Packet
			}
			if packet != test.packet {
				t.Fatalf("packet = %#v, want %#v", packet, test.packet)
			}
		})
	}
}

func TestPostgresAdapter_DecodeNodeMsgpackFixtures(t *testing.T) {
	pa := MakePostgresAdapter().(*postgresAdapter)

	t.Run("null scalar acknowledgement", func(t *testing.T) {
		payload, err := hex.DecodeString("84a3756964a46e6f6465a36e7370a12fa4747970650aa46461746182a9726571756573744964a172a67061636b6574c0")
		if err != nil {
			t.Fatal(err)
		}
		message, err := pa.decodeMsgpack(payload)
		if err != nil {
			t.Fatalf("decodeMsgpack failed: %v", err)
		}
		packet := message.Data.(*adapter.ServerSideEmitResponse).Packet
		if packet != nil {
			t.Fatalf("expected nil packet, got %#v", packet)
		}
	})

	t.Run("binary broadcast and millisecond timeout", func(t *testing.T) {
		payload, err := hex.DecodeString("84a3756964a46e6f6465a36e7370a12fa47479706503a46461746182a67061636b657483a47479706502a36e7370a12fa46461746192a56576656e74c403010203a46f70747383a5726f6f6d7390a665786365707490a5666c61677381a774696d656f7574cd02ee")
		if err != nil {
			t.Fatal(err)
		}
		message, err := pa.decodeMsgpack(payload)
		if err != nil {
			t.Fatalf("decodeMsgpack failed: %v", err)
		}
		data := message.Data.(*adapter.BroadcastMessage)
		packetData := data.Packet.Data.([]any)
		binary, ok := packetData[1].([]byte)
		if !ok || !bytes.Equal(binary, []byte{1, 2, 3}) {
			t.Fatalf("unexpected binary payload: %#v", packetData[1])
		}
		if data.Opts.Flags.Timeout == nil || *data.Opts.Flags.Timeout != 750 {
			t.Fatalf("unexpected timeout: %v", data.Opts.Flags.Timeout)
		}
	})
}

func TestPostgresAdapter_MarshalBinary(t *testing.T) {
	t.Run("nil data", func(t *testing.T) {
		msg := &adapter.ClusterMessage{Type: adapter.BROADCAST}
		if _, binary := postgres.MarshalAdapterData(msg.Data); binary {
			t.Error("Expected false for nil data")
		}
	})

	t.Run("heartbeat type", func(t *testing.T) {
		msg := &adapter.ClusterMessage{Type: adapter.HEARTBEAT, Data: "test"}
		if _, binary := postgres.MarshalAdapterData(msg.Data); binary {
			t.Error("Expected false for heartbeat type")
		}
	})

	t.Run("nested binary packet", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.BROADCAST,
			Data: &adapter.BroadcastMessage{
				Packet: &parser.Packet{Data: []any{"event", []byte{1, 2, 3}}},
			},
		}
		if _, binary := postgres.MarshalAdapterData(msg.Data); !binary {
			t.Error("Expected true for nested binary packet")
		}
	})
}

func TestPostgresAdapterBuilder(t *testing.T) {
	t.Run("creates builder", func(t *testing.T) {
		builder := &PostgresAdapterBuilder{}
		if builder.Postgres != nil {
			t.Fatal("Expected nil Postgres initially")
		}
	})
}
