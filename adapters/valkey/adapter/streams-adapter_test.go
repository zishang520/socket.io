package adapter

import (
	"context"
	"encoding/base64"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	vk "github.com/valkey-io/valkey-go"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func newValkeyRawClient(t *testing.T, address string) vk.Client {
	t.Helper()
	client, err := vk.NewClient(vk.ClientOption{
		InitAddress:  []string{address},
		DisableCache: true,
		AlwaysRESP2:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	return client
}

func newStreamsValkeyClient(t *testing.T, primary vk.Client, sub vk.Client) *valkey.ValkeyClient {
	t.Helper()
	client, err := valkey.NewValkeyClientWithSub(context.Background(), primary, sub)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func xaddAt(t *testing.T, client vk.Client, stream, id string, fields map[string]string) {
	t.Helper()
	command := client.B().Xadd().Key(stream).Id(id).FieldValue()
	for _, field := range [...]string{"uid", "nsp", "type", "data"} {
		if value, ok := fields[field]; ok {
			command = command.FieldValue(field, value)
		}
	}
	if err := client.Do(
		context.Background(),
		command.Build(),
	).Error(); err != nil {
		t.Fatal(err)
	}
}

func TestValkeyStreamsAdapterBuilderAppliesDefaults(t *testing.T) {
	server := miniredis.RunT(t)
	rawClient := newValkeyRawClient(t, server.Addr())
	client := newStreamsValkeyClient(t, rawClient, nil)
	input := DefaultValkeyStreamsAdapterOptions()
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")

	streamAdapter := (&ValkeyStreamsAdapterBuilder{Valkey: client, Opts: input}).New(nsp).(*valkeyStreamsAdapter)
	t.Cleanup(streamAdapter.Close)

	if streamAdapter.streamName != DefaultStreamName {
		t.Fatalf("stream name = %q, want %q", streamAdapter.streamName, DefaultStreamName)
	}
	if streamAdapter.publicChannel != DefaultChannelPrefix+"#/test#" {
		t.Fatalf("public channel = %q", streamAdapter.publicChannel)
	}
	if streamAdapter.opts.MaxLen() != DefaultStreamMaxLen ||
		streamAdapter.opts.ReadCount() != DefaultStreamReadCount ||
		streamAdapter.opts.BlockTimeInMs() != DefaultBlockTimeInMs {
		t.Fatalf("defaults were not applied: %+v", streamAdapter.opts)
	}
	if input.GetRawStreamName() != nil || input.GetRawMaxLen() != nil {
		t.Fatal("builder mutated its input options")
	}
}

func TestValkeyStreamsServerCountWithIndependentClients(t *testing.T) {
	server := miniredis.RunT(t)
	newAdapter := func() *valkeyStreamsAdapter {
		client := newStreamsValkeyClient(t, newValkeyRawClient(t, server.Addr()), nil)
		return NewValkeyStreamsAdapter(
			socket.NewNamespace(socket.NewServer(nil, nil), "/shared"),
			client,
			nil,
		).(*valkeyStreamsAdapter)
	}
	first, second := newAdapter(), newAdapter()
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	count, err := first.ServerCount()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("server count = %d, want 2", count)
	}
}

func TestValkeyStreamsDoPublishUsesCommonCodec(t *testing.T) {
	server := miniredis.RunT(t)
	rawClient := newValkeyRawClient(t, server.Addr())
	client := newStreamsValkeyClient(t, rawClient, nil)
	streamAdapter := NewValkeyStreamsAdapter(
		socket.NewNamespace(socket.NewServer(nil, nil), "/publish"),
		client,
		nil,
	).(*valkeyStreamsAdapter)
	t.Cleanup(streamAdapter.Close)

	offset, err := streamAdapter.DoPublish(&adapter.ClusterMessage{
		Uid:  "remote",
		Nsp:  "/publish",
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", []byte{1, 2}}},
			Opts:   new(adapter.PacketOptions),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if offset == "" {
		t.Fatal("durable publish returned an empty offset")
	}

	entries, err := client.XRangeN(context.Background(), streamAdapter.streamName, string(offset), string(offset), 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("stream entries = %d, want 1", len(entries))
	}
	message, err := valkey.DecodeStreamMessage(valkey.RawClusterMessage(entries[0].FieldValues))
	if err != nil {
		t.Fatal(err)
	}
	data, ok := message.Data.(*adapter.BroadcastMessage)
	if !ok || data.Packet == nil || data.Opts == nil {
		t.Fatalf("decoded stream message = %#v", message)
	}
}

func newValkeyStreamsPersistenceAdapter(
	t *testing.T,
	duration int64,
) (*valkeyStreamsAdapter, *miniredis.Miniredis, *valkey.ValkeyClient) {
	t.Helper()
	server := miniredis.RunT(t)
	rawClient := newValkeyRawClient(t, server.Addr())
	client := newStreamsValkeyClient(t, rawClient, nil)

	recovery := socket.DefaultConnectionStateRecovery()
	recovery.SetMaxDisconnectionDuration(duration)
	serverOpts := socket.DefaultServerOptions()
	serverOpts.SetConnectionStateRecovery(recovery)

	streamAdapter := MakeValkeyStreamsAdapter().(*valkeyStreamsAdapter)
	streamAdapter.ClusterAdapter.Construct(
		socket.NewNamespace(socket.NewServer(nil, serverOpts), "/test"),
	)
	streamAdapter.valkeyClient = client
	streamAdapter.ctx = client.Context()
	streamAdapter.streamName = DefaultStreamName
	streamAdapter.opts.SetSessionKeyPrefix(DefaultSessionKeyPrefix)
	t.Cleanup(streamAdapter.Close)
	return streamAdapter, server, client
}

func TestValkeyStreamsPersistSessionTTLValidation(t *testing.T) {
	t.Run("positive TTL", func(t *testing.T) {
		const duration = int64(1_500)
		streamAdapter, server, _ := newValkeyStreamsPersistenceAdapter(t, duration)
		streamAdapter.PersistSession(&socket.SessionToPersist{Pid: "pid"})

		ttl := server.TTL(DefaultSessionKeyPrefix + "pid")
		if ttl <= 0 || ttl > time.Duration(duration)*time.Millisecond {
			t.Fatalf("session TTL = %v", ttl)
		}
	})

	for _, tt := range []struct {
		duration  int64
		wantError string
	}{
		{duration: 0, wantError: "must be positive"},
		{duration: -1, wantError: "must be positive"},
		{duration: math.MaxInt64, wantError: "overflows time.Duration"},
	} {
		t.Run(strconv.FormatInt(tt.duration, 10), func(t *testing.T) {
			streamAdapter, server, client := newValkeyStreamsPersistenceAdapter(t, tt.duration)
			errors := make(chan error, 1)
			_ = client.On("error", func(args ...any) {
				if len(args) > 0 {
					if err, ok := args[0].(error); ok {
						errors <- err
					}
				}
			})

			streamAdapter.PersistSession(&socket.SessionToPersist{Pid: "pid"})
			if server.Exists(DefaultSessionKeyPrefix + "pid") {
				t.Fatal("session with invalid recovery duration was persisted")
			}
			select {
			case err := <-errors:
				if !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("error = %v, want %q", err, tt.wantError)
				}
			case <-time.After(time.Second):
				t.Fatal("invalid recovery duration did not emit an error")
			}
		})
	}
}

func storeValkeySession(
	t *testing.T,
	client *valkey.ValkeyClient,
	session *socket.SessionToPersist,
) {
	t.Helper()
	payload, err := utils.MsgPack().Encode(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Set(
		context.Background(),
		DefaultSessionKeyPrefix+string(session.Pid),
		base64.StdEncoding.EncodeToString(payload),
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}
}

func TestValkeyStreamsRestoreSessionUsesGetDelAndValidatesSession(t *testing.T) {
	streamAdapter, _, client := newValkeyStreamsPersistenceAdapter(t, 1_000)
	session := &socket.SessionToPersist{Sid: "sid", Pid: "pid"}
	storeValkeySession(t, client, session)
	offset, err := client.XAdd(context.Background(), DefaultStreamName, valkey.RawClusterMessage{
		"uid":  "remote",
		"nsp":  "/test",
		"type": strconv.Itoa(int(adapter.HEARTBEAT)),
	}, DefaultStreamMaxLen)
	if err != nil {
		t.Fatal(err)
	}

	restored, err := streamAdapter.RestoreSession("pid", offset)
	if err != nil {
		t.Fatal(err)
	}
	if restored.SessionToPersist == nil || restored.Pid != "pid" {
		t.Fatalf("restored session = %#v", restored)
	}
	if restored.Rooms == nil || restored.MissedPackets == nil || len(restored.MissedPackets) != 0 {
		t.Fatalf("restored collections = rooms:%#v packets:%#v", restored.Rooms, restored.MissedPackets)
	}
	if _, err := streamAdapter.RestoreSession("pid", offset); err == nil || err.Error() != "session not found" {
		t.Fatalf("second restore error = %v", err)
	}

	if _, err := streamAdapter.RestoreSession("pid", "invalid"); err == nil || err.Error() != "invalid offset format" {
		t.Fatalf("invalid offset error = %v", err)
	}
}

func TestValkeyStreamsRestoreSessionReadsPagedMissedPackets(t *testing.T) {
	streamAdapter, _, client := newValkeyStreamsPersistenceAdapter(t, 1_000)
	storeValkeySession(t, client, &socket.SessionToPersist{Sid: "sid", Pid: "pid"})
	marker, err := client.XAdd(context.Background(), DefaultStreamName, valkey.RawClusterMessage{
		"uid":  "remote",
		"nsp":  "/test",
		"type": strconv.Itoa(int(adapter.HEARTBEAT)),
	}, DefaultStreamMaxLen)
	if err != nil {
		t.Fatal(err)
	}

	other := valkey.RawClusterMessage{
		"uid":  "remote",
		"nsp":  "/other",
		"type": strconv.Itoa(int(adapter.HEARTBEAT)),
	}
	for range restoreSessionPageSize + 1 {
		if _, err := client.XAdd(context.Background(), DefaultStreamName, other, DefaultStreamMaxLen); err != nil {
			t.Fatal(err)
		}
	}
	message, err := valkey.EncodeStreamMessage(&adapter.ClusterMessage{
		Uid:  "remote",
		Nsp:  "/test",
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"late"}},
			Opts:   new(adapter.PacketOptions),
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	lastOffset, err := client.XAdd(context.Background(), DefaultStreamName, message, DefaultStreamMaxLen)
	if err != nil {
		t.Fatal(err)
	}

	restored, err := streamAdapter.RestoreSession("pid", marker)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{[]any{"late", lastOffset}}
	if !reflect.DeepEqual(restored.MissedPackets, want) {
		t.Fatalf("missed packets = %#v, want %#v", restored.MissedPackets, want)
	}
}

func TestValkeyStreamsRestoreSessionRejectsMissingSessionData(t *testing.T) {
	streamAdapter, _, client := newValkeyStreamsPersistenceAdapter(t, 1_000)
	payload, err := utils.MsgPack().Encode((*socket.SessionToPersist)(nil))
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Set(
		context.Background(),
		DefaultSessionKeyPrefix+"pid",
		base64.StdEncoding.EncodeToString(payload),
		time.Minute,
	); err != nil {
		t.Fatal(err)
	}
	offset, err := client.XAdd(context.Background(), DefaultStreamName, valkey.RawClusterMessage{
		"uid":  "remote",
		"nsp":  "/test",
		"type": strconv.Itoa(int(adapter.HEARTBEAT)),
	}, DefaultStreamMaxLen)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := streamAdapter.RestoreSession("pid", offset); err == nil ||
		err.Error() != "invalid persisted session: missing session data" {
		t.Fatalf("error = %v", err)
	}
}

func TestValkeyStreamsIsEphemeral(t *testing.T) {
	requestID := "request"
	tests := []struct {
		message *adapter.ClusterMessage
		want    bool
	}{
		{message: &adapter.ClusterMessage{Type: adapter.FETCH_SOCKETS}, want: true},
		{message: &adapter.ClusterMessage{Type: adapter.SERVER_SIDE_EMIT}, want: true},
		{message: &adapter.ClusterMessage{Type: adapter.BROADCAST, Data: &adapter.BroadcastMessage{RequestId: &requestID}}, want: true},
		{message: &adapter.ClusterMessage{Type: adapter.BROADCAST, Data: &adapter.BroadcastMessage{}}, want: false},
		{message: &adapter.ClusterMessage{Type: adapter.SOCKETS_JOIN}, want: false},
	}
	for _, tt := range tests {
		if got := isEphemeral(tt.message); got != tt.want {
			t.Fatalf("isEphemeral(%d) = %t, want %t", tt.message.Type, got, tt.want)
		}
	}
}
