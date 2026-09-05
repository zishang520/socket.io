package emitter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/postgres/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestBroadcastOperatorIsImmutable(t *testing.T) {
	base := NewBroadcastOperator(nil, &BroadcastOptions{Nsp: "/"}, nil, nil, nil)
	targeted := base.To("room1", "room2").In("room3").(*BroadcastOperator)
	excluded := targeted.Except("excluded1", "excluded2").(*BroadcastOperator)
	compressed := excluded.Compress(false).(*BroadcastOperator)
	volatile := compressed.Volatile().(*BroadcastOperator)

	if base.rooms.Len() != 0 || base.exceptRooms.Len() != 0 || base.flags.Compress != nil || base.flags.Volatile {
		t.Fatal("base operator was mutated")
	}
	if targeted.rooms.Len() != 3 || !targeted.rooms.Has("room1") || !targeted.rooms.Has("room2") || !targeted.rooms.Has("room3") {
		t.Fatal("To() and In() did not preserve the targeted rooms")
	}
	if targeted.exceptRooms.Len() != 0 || excluded.exceptRooms.Len() != 2 ||
		!excluded.exceptRooms.Has("excluded1") || !excluded.exceptRooms.Has("excluded2") {
		t.Fatal("Except() did not copy the excluded rooms")
	}
	if excluded.flags.Compress != nil || compressed.flags.Compress == nil || *compressed.flags.Compress {
		t.Fatal("Compress(false) did not copy the flags")
	}
	if compressed.flags.Volatile || !volatile.flags.Volatile {
		t.Fatal("Volatile() did not copy the flags")
	}
}

func TestBroadcastOperator_Emit_ReservedEvent(t *testing.T) {
	err := NewBroadcastOperator(nil, nil, nil, nil, nil).Emit("connect")
	if err == nil {
		t.Error("Expected error for reserved event")
	}
}

func TestBroadcastOperatorUsesAttachmentAtPayloadThreshold(t *testing.T) {
	pool, err := pgxpool.New(t.Context(), "postgres://localhost/socket_io_test")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	client, err := postgres.NewPostgresClient(t.Context(), pool)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	message := &adapter.ClusterMessage{Type: adapter.HEARTBEAT}
	wireMessage := *message
	wireMessage.Uid = adapter.EMITTER_UID
	wireMessage.Nsp = "/"
	wireMessage.Data, _ = postgres.MarshalAdapterData(message.Data)
	payload, err := json.Marshal(&wireMessage)
	if err != nil {
		t.Fatal(err)
	}

	operator := NewBroadcastOperator(client, &BroadcastOptions{
		Nsp:              "/",
		BroadcastChannel: "socket.io#/",
		TableName:        "socket_io_attachments",
		PayloadThreshold: len(payload),
	}, nil, nil, nil)
	err = operator.publish(message)
	if err == nil || !strings.Contains(err.Error(), "failed to insert attachment") {
		t.Fatalf("publish at threshold error = %v, want attachment insert error", err)
	}
}

func TestBroadcastOperator_SocketsJoin_Marshal(t *testing.T) {
	msg := &adapter.ClusterMessage{
		Type: adapter.SOCKETS_JOIN,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts: &adapter.PacketOptions{
				Rooms: []socket.Room{"room1"},
			},
			Rooms: []socket.Room{"target-room"},
		},
	}
	wireData, binary := postgres.MarshalAdapterData(msg.Data)
	if binary {
		t.Fatal("SOCKETS_JOIN must not be marked as binary")
	}
	msg.Uid = adapter.EMITTER_UID
	msg.Data = wireData
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal join message: %v", err)
	}

	var wire struct {
		Uid  adapter.ServerId    `json:"uid"`
		Type adapter.MessageType `json:"type"`
		Data struct {
			Opts struct {
				Rooms  []socket.Room `json:"rooms"`
				Except []socket.Room `json:"except"`
			} `json:"opts"`
			Rooms []socket.Room `json:"rooms"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("failed to parse join message: %v", err)
	}
	if wire.Uid != adapter.EMITTER_UID {
		t.Errorf("expected uid %q, got %q", adapter.EMITTER_UID, wire.Uid)
	}
	if wire.Type != adapter.SOCKETS_JOIN {
		t.Errorf("expected type %d, got %v", adapter.SOCKETS_JOIN, wire.Type)
	}
	if len(wire.Data.Opts.Rooms) != 1 || wire.Data.Opts.Rooms[0] != "room1" {
		t.Fatalf("unexpected target rooms: %v", wire.Data.Opts.Rooms)
	}
	if wire.Data.Opts.Except == nil {
		t.Fatal("Node.js wire format requires an empty except array")
	}
	if len(wire.Data.Rooms) != 1 || wire.Data.Rooms[0] != "target-room" {
		t.Fatalf("unexpected joined rooms: %v", wire.Data.Rooms)
	}
}
