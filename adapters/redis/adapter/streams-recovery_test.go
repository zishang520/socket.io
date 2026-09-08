package adapter

import (
	"encoding/base64"
	"reflect"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestRedisStreamsRecoveryIsolatesNamespaces(t *testing.T) {
	db := miniredis.RunT(t)
	raw := rds.NewClient(&rds.Options{Addr: db.Addr()})
	t.Cleanup(func() { _ = raw.Close() })
	client := mustRedisClient(t, t.Context(), raw)
	options := socket.DefaultServerOptions()
	options.SetConnectionStateRecovery(socket.DefaultConnectionStateRecovery())
	server := socket.NewServer(nil, options)
	public := NewRedisStreamsAdapter(socket.NewNamespace(server, "/public"), client, nil).(*redisStreamsAdapter)
	admin := NewRedisStreamsAdapter(socket.NewNamespace(server, "/admin"), client, nil).(*redisStreamsAdapter)
	t.Cleanup(public.Close)
	t.Cleanup(admin.Close)
	publishOffset := func(current *redisStreamsAdapter) string {
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
	if err = raw.Set(t.Context(), legacyKey, base64.StdEncoding.EncodeToString(payload), time.Minute).Err(); err != nil {
		t.Fatal(err)
	}
	if session, err := public.RestoreSession(publicSession.Pid, publicOffset); session != nil || err == nil {
		t.Fatalf("unscoped session was restored: %v, %v", session, err)
	}
	if exists, err := raw.Exists(t.Context(), legacyKey).Result(); err != nil || exists != 1 {
		t.Fatalf("unscoped session was consumed: exists=%d, error=%v", exists, err)
	}
}
