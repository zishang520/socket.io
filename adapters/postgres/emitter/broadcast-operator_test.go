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

func TestBroadcastOptions(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		opts := &BroadcastOptions{}
		if opts.Nsp != "" {
			t.Error("Expected empty Nsp")
		}
		if opts.BroadcastChannel != "" {
			t.Error("Expected empty BroadcastChannel")
		}
		if opts.TableName != "" {
			t.Error("Expected empty TableName")
		}
		if opts.PayloadThreshold != 0 {
			t.Error("Expected zero PayloadThreshold")
		}
	})

	t.Run("set values", func(t *testing.T) {
		opts := &BroadcastOptions{
			Nsp:              "/chat",
			BroadcastChannel: "socket.io#/chat",
			TableName:        "socket_io_attachments",
			PayloadThreshold: 8000,
		}
		if opts.Nsp != "/chat" {
			t.Errorf("Expected '/chat', got %q", opts.Nsp)
		}
		if opts.BroadcastChannel != "socket.io#/chat" {
			t.Errorf("Expected 'socket.io#/chat', got %q", opts.BroadcastChannel)
		}
		if opts.TableName != "socket_io_attachments" {
			t.Errorf("Expected 'socket_io_attachments', got %q", opts.TableName)
		}
	})
}

func TestMakeBroadcastOperator(t *testing.T) {
	b := MakeBroadcastOperator()

	if b == nil {
		t.Fatal("Expected non-nil BroadcastOperator")
	}

	if b.rooms == nil {
		t.Error("Expected non-nil rooms set")
	}
	if b.rooms.Len() != 0 {
		t.Error("Expected empty rooms set")
	}

	if b.exceptRooms == nil {
		t.Error("Expected non-nil exceptRooms set")
	}
	if b.exceptRooms.Len() != 0 {
		t.Error("Expected empty exceptRooms set")
	}

	if b.flags == nil {
		t.Error("Expected non-nil flags")
	}
}

func TestBroadcastOperator_Construct_NilSafety(t *testing.T) {
	b := MakeBroadcastOperator()
	b.Construct(nil, nil, nil, nil, nil)

	// Should have safe defaults
	if b.broadcastOptions == nil {
		t.Error("Expected non-nil broadcastOptions after nil construct")
	}
	if b.rooms == nil {
		t.Error("Expected non-nil rooms after nil construct")
	}
	if b.exceptRooms == nil {
		t.Error("Expected non-nil exceptRooms after nil construct")
	}
	if b.flags == nil {
		t.Error("Expected non-nil flags after nil construct")
	}
}

func TestBroadcastOperator_To(t *testing.T) {
	b := MakeBroadcastOperator()
	b.Construct(nil, nil, nil, nil, nil)

	t.Run("single room", func(t *testing.T) {
		result := b.To("room1")
		if result == b {
			t.Error("Expected new BroadcastOperator instance")
		}
		bop := result.(*BroadcastOperator)
		if !bop.rooms.Has("room1") {
			t.Error("Expected room1 to be added")
		}
	})

	t.Run("multiple rooms", func(t *testing.T) {
		result := b.To("room1", "room2", "room3")
		bop := result.(*BroadcastOperator)
		if bop.rooms.Len() != 3 {
			t.Errorf("Expected 3 rooms, got %d", bop.rooms.Len())
		}
	})

	t.Run("chaining", func(t *testing.T) {
		result := b.To("room1").To("room2")
		bop := result.(*BroadcastOperator)
		if !bop.rooms.Has("room1") || !bop.rooms.Has("room2") {
			t.Error("Expected both rooms after chaining")
		}
	})
}

func TestBroadcastOperator_In(t *testing.T) {
	b := MakeBroadcastOperator()
	b.Construct(nil, nil, nil, nil, nil)

	// In should behave the same as To
	result := b.In("room1")
	bop := result.(*BroadcastOperator)
	if !bop.rooms.Has("room1") {
		t.Error("Expected room1 to be added via In")
	}
}

func TestBroadcastOperator_Except(t *testing.T) {
	b := MakeBroadcastOperator()
	b.Construct(nil, nil, nil, nil, nil)

	t.Run("single room", func(t *testing.T) {
		result := b.Except("excluded")
		if result == b {
			t.Error("Expected new BroadcastOperator instance")
		}
		bop := result.(*BroadcastOperator)
		if !bop.exceptRooms.Has("excluded") {
			t.Error("Expected room to be added to exceptRooms")
		}
	})

	t.Run("multiple rooms", func(t *testing.T) {
		result := b.Except("ex1", "ex2")
		bop := result.(*BroadcastOperator)
		if bop.exceptRooms.Len() != 2 {
			t.Errorf("Expected 2 except rooms, got %d", bop.exceptRooms.Len())
		}
	})
}

func TestBroadcastOperator_Compress(t *testing.T) {
	b := MakeBroadcastOperator()
	b.Construct(nil, nil, nil, nil, nil)

	t.Run("compress true", func(t *testing.T) {
		result := b.Compress(true)
		if result == b {
			t.Error("Expected new BroadcastOperator instance")
		}
		bop := result.(*BroadcastOperator)
		if bop.flags.Compress == nil || !*bop.flags.Compress {
			t.Error("Expected Compress to be true")
		}
	})

	t.Run("compress false", func(t *testing.T) {
		result := b.Compress(false)
		bop := result.(*BroadcastOperator)
		if bop.flags.Compress == nil || *bop.flags.Compress {
			t.Error("Expected Compress to be false")
		}
	})
}

func TestBroadcastOperator_Volatile(t *testing.T) {
	b := MakeBroadcastOperator()
	b.Construct(nil, nil, nil, nil, nil)

	result := b.Volatile()
	if result == b {
		t.Error("Expected new BroadcastOperator instance")
	}
	bop := result.(*BroadcastOperator)
	if !bop.flags.Volatile {
		t.Error("Expected Volatile to be true")
	}
}

func TestBroadcastOperator_Immutability(t *testing.T) {
	b := MakeBroadcastOperator()
	b.Construct(nil, nil, nil, nil, nil)

	// Chain operations and verify original is unchanged
	_ = b.To("room1").Except("room2").Volatile().Compress(true)

	if b.rooms.Len() != 0 {
		t.Error("Original rooms should be empty")
	}
	if b.exceptRooms.Len() != 0 {
		t.Error("Original exceptRooms should be empty")
	}
	if b.flags.Volatile {
		t.Error("Original flags should not be volatile")
	}
}

func TestBroadcastOperator_Emit_ReservedEvent(t *testing.T) {
	b := MakeBroadcastOperator()
	b.Construct(nil, &BroadcastOptions{}, nil, nil, nil)

	err := b.Emit("connect")
	if err == nil {
		t.Error("Expected error for reserved event")
	}
}

func TestBroadcastOperator_Emit_NilClient(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic when postgres client is nil")
		}
	}()

	b := MakeBroadcastOperator()
	b.Construct(nil, &BroadcastOptions{}, nil, nil, nil)

	// Emit on non-reserved should panic due to nil postgres client
	_ = b.Emit("test")
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

func TestBroadcastOperator_DisconnectSockets_Marshal(t *testing.T) {
	msg := &adapter.ClusterMessage{
		Type: adapter.DISCONNECT_SOCKETS,
		Data: &adapter.DisconnectSocketsMessage{
			Opts: &adapter.PacketOptions{
				Rooms: []socket.Room{"room1"},
			},
			Close: true,
		},
	}
	wireData, binary := postgres.MarshalAdapterData(msg.Data)
	if binary {
		t.Fatal("DISCONNECT_SOCKETS must not be marked as binary")
	}
	msg.Uid = adapter.EMITTER_UID
	msg.Data = wireData
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal disconnect message: %v", err)
	}

	var wire struct {
		Uid  adapter.ServerId    `json:"uid"`
		Type adapter.MessageType `json:"type"`
		Data struct {
			Opts struct {
				Rooms  []socket.Room `json:"rooms"`
				Except []socket.Room `json:"except"`
			} `json:"opts"`
			Close bool `json:"close"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("failed to parse disconnect message: %v", err)
	}
	if wire.Uid != adapter.EMITTER_UID || wire.Type != adapter.DISCONNECT_SOCKETS {
		t.Fatalf("unexpected header: uid=%q type=%d", wire.Uid, wire.Type)
	}
	if wire.Data.Opts.Except == nil {
		t.Fatal("Node.js wire format requires an empty except array")
	}
	if !wire.Data.Close {
		t.Fatal("expected close to be true")
	}
}
