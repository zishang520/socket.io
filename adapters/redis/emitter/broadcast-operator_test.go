package emitter

import (
	"encoding/json"
	"testing"

	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
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
		if opts.RequestChannel != "" {
			t.Error("Expected empty RequestChannel")
		}
		if opts.Parser != nil {
			t.Error("Expected nil Parser")
		}
	})

	t.Run("set values", func(t *testing.T) {
		opts := &BroadcastOptions{
			Nsp:              "/chat",
			BroadcastChannel: "socket.io#/chat#",
			RequestChannel:   "socket.io-request#/chat#",
			Parser:           utils.MsgPack(),
		}
		if opts.Nsp != "/chat" {
			t.Errorf("Expected '/chat', got %q", opts.Nsp)
		}
		if opts.BroadcastChannel != "socket.io#/chat#" {
			t.Errorf("Expected 'socket.io#/chat#', got %q", opts.BroadcastChannel)
		}
		if opts.Parser == nil {
			t.Error("Expected non-nil Parser")
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

	// Chain operations
	b.To("room1").Except("room2").Volatile().Compress(true)

	// Original should be unchanged
	if b.rooms.Len() != 0 {
		t.Error("Original rooms should be unchanged")
	}
	if b.exceptRooms.Len() != 0 {
		t.Error("Original exceptRooms should be unchanged")
	}
	if b.flags.Volatile {
		t.Error("Original flags should be unchanged")
	}
}

func TestBroadcastOperator_Emit_ReservedEvent(t *testing.T) {
	b := MakeBroadcastOperator()
	b.Construct(nil, &BroadcastOptions{Parser: utils.MsgPack()}, nil, nil, nil)

	reservedList := []string{"connect", "disconnect", "connect_error"}
	for _, ev := range reservedList {
		t.Run(ev, func(t *testing.T) {
			err := b.Emit(ev, "data")
			if err == nil {
				t.Errorf("Expected error for reserved event %q", ev)
			}
		})
	}
}

func TestBroadcastOperator_Emit_NilParser(t *testing.T) {
	b := MakeBroadcastOperator()
	b.Construct(nil, &BroadcastOptions{}, nil, nil, nil) // No parser

	err := b.Emit("custom", "data")
	if err == nil {
		t.Error("Expected error when parser is nil")
	}
}

func TestNewBroadcastOperator_WithRooms(t *testing.T) {
	rooms := types.NewSet[socket.Room]("room1", "room2")
	exceptRooms := types.NewSet[socket.Room]("except1")
	flags := &socket.BroadcastFlags{}
	flags.Volatile = true

	b := NewBroadcastOperator(nil, nil, rooms, exceptRooms, flags)

	if b.rooms.Len() != 2 {
		t.Errorf("Expected 2 rooms, got %d", b.rooms.Len())
	}
	if b.exceptRooms.Len() != 1 {
		t.Errorf("Expected 1 except room, got %d", b.exceptRooms.Len())
	}
	if !b.flags.Volatile {
		t.Error("Expected Volatile flag to be set")
	}
}

func TestEmitterOptions_Assign(t *testing.T) {
	t.Run("assign with values", func(t *testing.T) {
		source := DefaultEmitterOptions()
		source.SetKey("source-key")
		source.SetParser(utils.MsgPack())

		target := DefaultEmitterOptions()
		target.Assign(source)

		if target.Key() != "source-key" {
			t.Errorf("Expected 'source-key', got %q", target.Key())
		}
		if target.Parser() == nil {
			t.Error("Expected non-nil parser")
		}
	})

	t.Run("partial assign", func(t *testing.T) {
		source := DefaultEmitterOptions()
		source.SetKey("partial-key")

		target := DefaultEmitterOptions()
		target.SetParser(utils.MsgPack())
		target.Assign(source)

		if target.Key() != "partial-key" {
			t.Errorf("Expected 'partial-key', got %q", target.Key())
		}
		// Original parser should be preserved
		if target.Parser() == nil {
			t.Error("Expected parser to be preserved")
		}
	})
}

func TestMakeEmitter(t *testing.T) {
	e := MakeEmitter()

	if e == nil {
		t.Fatal("Expected non-nil Emitter")
	}
	if e.opts == nil {
		t.Error("Expected non-nil opts")
	}
	if e.nsp != "/" {
		t.Errorf("Expected '/', got %q", e.nsp)
	}
}

func TestDefaultEmitterKey(t *testing.T) {
	if DefaultEmitterKey != "socket.io" {
		t.Errorf("Expected 'socket.io', got %q", DefaultEmitterKey)
	}
}

func TestRequest_JSONSerialization(t *testing.T) {
	t.Run("basic request", func(t *testing.T) {
		req := &Request{
			Type: 1,
			Uid:  "test-uid",
		}

		// Should be JSON serializable without error
		_, err := json.Marshal(req)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	})

	t.Run("request with rooms", func(t *testing.T) {
		req := &Request{
			Type:  2,
			Rooms: []socket.Room{"room1", "room2"},
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}

		// Unmarshal and verify
		var parsed Request
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Errorf("Unmarshal error: %v", err)
		}
		if len(parsed.Rooms) != 2 {
			t.Errorf("Expected 2 rooms, got %d", len(parsed.Rooms))
		}
	})
}
