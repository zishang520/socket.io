package adapter

import (
	"encoding/base64"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestValkeyStreamsRecoveryIsolatesNamespaces(t *testing.T) {
	db := miniredis.RunT(t)
	client := newStreamsValkeyClient(t, newValkeyRawClient(t, db.Addr()), nil)
	options := socket.DefaultServerOptions()
	options.SetConnectionStateRecovery(socket.DefaultConnectionStateRecovery())
	server := socket.NewServer(nil, options)
	public := NewValkeyStreamsAdapter(socket.NewNamespace(server, "/public"), client, nil)
	admin := NewValkeyStreamsAdapter(socket.NewNamespace(server, "/admin"), client, nil)
	t.Cleanup(public.Close)
	t.Cleanup(admin.Close)
	publishOffset := func(current ValkeyStreamsAdapter) string {
		t.Helper()
		offset, err := current.PublishAndReturnOffset(&adapter.ClusterMessage{
			Type: adapter.BROADCAST,
			Data: &adapter.BroadcastMessage{
				Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"marker"}},
				Opts:   adapter.EncodeOptions(nil),
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(offset)
	}
	publicOffset, adminOffset := publishOffset(public), publishOffset(admin)
	publicSession := &socket.SessionToPersist{
		Sid: "public-sid", Pid: "shared-pid", Rooms: types.NewSet[socket.Room]("public-room"),
		Data: map[string]any{"source": "public"},
	}
	public.PersistSession(publicSession)

	for _, offset := range []string{publicOffset, adminOffset} {
		if session, err := admin.RestoreSession(publicSession.Pid, offset); session != nil || err == nil {
			t.Fatalf("admin restored public credentials with offset %q: %v, %v", offset, session, err)
		}
	}
	if session, err := public.RestoreSession(publicSession.Pid, "0-0"); session != nil || err == nil {
		t.Fatalf("missing offset restored a session: %v, %v", session, err)
	}

	// The same PID may exist independently in another namespace.
	admin.PersistSession(&socket.SessionToPersist{Sid: "admin-sid", Pid: publicSession.Pid})
	if session, err := admin.RestoreSession(publicSession.Pid, publicOffset); session != nil || err == nil {
		t.Fatalf("foreign offset claimed the admin session: %v, %v", session, err)
	}
	restored, err := public.RestoreSession(publicSession.Pid, publicOffset)
	if err != nil {
		t.Fatalf("failed recovery consumed the public session: %v", err)
	}
	if restored.Sid != publicSession.Sid || !restored.Rooms.Has("public-room") ||
		!reflect.DeepEqual(restored.Data, publicSession.Data) {
		t.Fatalf("public session changed: %#v", restored.SessionToPersist)
	}
	restored, err = admin.RestoreSession(publicSession.Pid, adminOffset)
	if err != nil || restored == nil || restored.Sid != "admin-sid" {
		t.Fatalf("public recovery affected the admin session: %v, %v", restored, err)
	}
	if session, restoreErr := public.RestoreSession(publicSession.Pid, publicOffset); session != nil || restoreErr == nil {
		t.Fatalf("session was claimed twice: %v, %v", session, restoreErr)
	}

	// Unscoped sessions cannot prove their namespace and remain unclaimed.
	payload, err := utils.MsgPack().Encode(publicSession)
	if err != nil {
		t.Fatal(err)
	}
	legacyKey := DefaultSessionKeyPrefix + string(publicSession.Pid)
	if err = client.Set(t.Context(), legacyKey, base64.StdEncoding.EncodeToString(payload), time.Minute); err != nil {
		t.Fatal(err)
	}
	if session, err := public.RestoreSession(publicSession.Pid, publicOffset); session != nil || err == nil {
		t.Fatalf("unscoped session was restored: %v, %v", session, err)
	}
	if !db.Exists(legacyKey) {
		t.Fatal("unscoped session was consumed")
	}
}

func TestValkeyStreamsRecoveryReadLimitFollowsRetention(t *testing.T) {
	db := miniredis.RunT(t)
	const laterEntries = 100001
	if _, err := db.XAdd(DefaultStreamName, "1-0", []string{"nsp", "/test", "type", "1"}); err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= laterEntries+1; i++ {
		if _, err := db.XAdd(DefaultStreamName, strconv.Itoa(i)+"-0", []string{"nsp", "/other", "type", "1"}); err != nil {
			t.Fatal(err)
		}
	}
	lastOffset := strconv.Itoa(laterEntries+2) + "-0"
	if _, err := db.XAdd(DefaultStreamName, lastOffset, []string{
		"nsp", "/test", "type", "3",
		"data", `{"packet":{"type":2,"data":["late"]},"opts":{"rooms":[],"except":[]}}`,
	}); err != nil {
		t.Fatal(err)
	}
	client := newStreamsValkeyClient(t, newValkeyRawClient(t, db.Addr()), nil)
	options := socket.DefaultServerOptions()
	options.SetConnectionStateRecovery(socket.DefaultConnectionStateRecovery())
	server := socket.NewServer(nil, options)

	for _, test := range []struct {
		name    string
		maxLen  int64
		wantErr error
	}{
		{"larger retention", 200000, nil},
		{"history beyond read budget", DefaultStreamMaxLen, errRestoreSessionReadLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			opts := DefaultValkeyStreamsAdapterOptions()
			opts.SetMaxLen(test.maxLen)
			current := NewValkeyStreamsAdapter(socket.NewNamespace(server, "/test"), client, opts)
			t.Cleanup(current.Close)
			current.PersistSession(&socket.SessionToPersist{Sid: "sid", Pid: "pid"})
			restored, err := current.RestoreSession("pid", "1-0")
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("recovery error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr != nil {
				if restored != nil {
					t.Fatal("recovery returned an incomplete session")
				}
				return
			}
			want := []any{[]any{"late", lastOffset}}
			if restored == nil || !reflect.DeepEqual(restored.MissedPackets, want) {
				t.Fatalf("recovered session = %#v, want missed packets %#v", restored, want)
			}
		})
	}
}
