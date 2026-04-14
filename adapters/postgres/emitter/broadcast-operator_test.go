package emitter

import (
	"encoding/json"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestReservedEvents(t *testing.T) {
	reserved := []string{
		"connect",
		"connect_error",
		"disconnect",
		"disconnecting",
		"newListener",
		"removeListener",
	}

	for _, ev := range reserved {
		t.Run(ev, func(t *testing.T) {
			if !reservedEvents.Has(ev) {
				t.Errorf("Expected %q to be reserved", ev)
			}
		})
	}

	t.Run("non-reserved events", func(t *testing.T) {
		nonReserved := []string{"message", "chat", "custom", ""}
		for _, ev := range nonReserved {
			if reservedEvents.Has(ev) {
				t.Errorf("Expected %q to NOT be reserved", ev)
			}
		}
	})
}

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

func TestBroadcastOperator_SocketsJoin_Marshal(t *testing.T) {
	// Verify ClusterMessage format for SOCKETS_JOIN matches Node.js wire format
	msg := &adapter.ClusterMessage{
		Uid:  "emitter",
		Type: adapter.SOCKETS_JOIN,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts: &adapter.PacketOptions{
				Rooms: []socket.Room{"room1"},
			},
			Rooms: []socket.Room{"target-room"},
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal join message: %v", err)
	}
	if len(data) == 0 {
		t.Error("Expected non-empty JSON")
	}

	// Verify structure matches Node.js: {uid, type, data: {opts: {rooms, except}, rooms}}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}
	if raw["uid"] != "emitter" {
		t.Errorf("Expected uid 'emitter', got %v", raw["uid"])
	}
	if raw["type"].(float64) != float64(adapter.SOCKETS_JOIN) {
		t.Errorf("Expected type %d, got %v", adapter.SOCKETS_JOIN, raw["type"])
	}
}

func TestBroadcastOperator_DisconnectSockets_Marshal(t *testing.T) {
	// Verify ClusterMessage format for DISCONNECT_SOCKETS matches Node.js wire format
	msg := &adapter.ClusterMessage{
		Uid:  "emitter",
		Type: adapter.DISCONNECT_SOCKETS,
		Data: &adapter.DisconnectSocketsMessage{
			Opts: &adapter.PacketOptions{
				Rooms: []socket.Room{"room1"},
			},
			Close: true,
		},
	}
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal disconnect message: %v", err)
	}
	if len(data) == 0 {
		t.Error("Expected non-empty JSON")
	}
}
