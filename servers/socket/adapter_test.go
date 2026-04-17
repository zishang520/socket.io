package socket

import (
	"testing"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// newTestAdapter creates a fully initialized adapter via a real server.
func newTestAdapter() Adapter {
	server := NewServer(nil, nil)
	nsp := server.Sockets()
	return nsp.Adapter()
}

func TestAdapterAddAll(t *testing.T) {
	adapter := newTestAdapter()

	sid := SocketId("s1")
	rooms := types.NewSet[Room]("room1", "room2")
	adapter.AddAll(sid, rooms)

	got := adapter.SocketRooms(sid)
	if got == nil {
		t.Fatal("Expected SocketRooms to return non-nil set")
	}
	if got.Len() != 2 {
		t.Errorf("Expected 2 rooms, got %d", got.Len())
	}
	if !got.Has("room1") || !got.Has("room2") {
		t.Error("Expected rooms to contain 'room1' and 'room2'")
	}

	// Verify room→socket mapping
	r1ids := adapter.Rooms()
	if ids, ok := r1ids.Load("room1"); !ok || !ids.Has(sid) {
		t.Error("Expected room1 to contain socket s1")
	}
	if ids, ok := r1ids.Load("room2"); !ok || !ids.Has(sid) {
		t.Error("Expected room2 to contain socket s1")
	}
}

func TestAdapterAddAllMultipleSockets(t *testing.T) {
	adapter := newTestAdapter()

	adapter.AddAll("s1", types.NewSet[Room]("room1"))
	adapter.AddAll("s2", types.NewSet[Room]("room1"))

	ids, ok := adapter.Rooms().Load("room1")
	if !ok {
		t.Fatal("Expected room1 to exist")
	}
	if ids.Len() != 2 {
		t.Errorf("Expected 2 sockets in room1, got %d", ids.Len())
	}
}

func TestAdapterDel(t *testing.T) {
	adapter := newTestAdapter()

	adapter.AddAll("s1", types.NewSet[Room]("room1", "room2"))
	adapter.Del("s1", "room1")

	rooms := adapter.SocketRooms("s1")
	if rooms.Has("room1") {
		t.Error("Expected room1 to be removed from socket s1")
	}
	if !rooms.Has("room2") {
		t.Error("Expected room2 to still be present for socket s1")
	}

	// room1 should be deleted entirely since no sockets remain
	if _, ok := adapter.Rooms().Load("room1"); ok {
		t.Error("Expected room1 to be deleted when empty")
	}
}

func TestAdapterDelAll(t *testing.T) {
	adapter := newTestAdapter()

	adapter.AddAll("s1", types.NewSet[Room]("room1", "room2", "room3"))
	adapter.DelAll("s1")

	if rooms := adapter.SocketRooms("s1"); rooms != nil {
		t.Error("Expected SocketRooms to return nil after DelAll")
	}

	// All rooms should be cleaned up
	adapter.Rooms().Range(func(room Room, ids *types.Set[SocketId]) bool {
		if ids.Has("s1") {
			t.Errorf("Expected socket s1 to be removed from room %s", room)
		}
		return true
	})
}

func TestAdapterDelNonExistentSocket(t *testing.T) {
	adapter := newTestAdapter()

	// Should not panic
	adapter.Del("nonexistent", "room1")
	adapter.DelAll("nonexistent")
}

func TestAdapterSocketRoomsNonExistent(t *testing.T) {
	adapter := newTestAdapter()

	if rooms := adapter.SocketRooms("nonexistent"); rooms != nil {
		t.Error("Expected nil for non-existent socket")
	}
}

func TestAdapterSockets(t *testing.T) {
	adapter := newTestAdapter()

	adapter.AddAll("s1", types.NewSet[Room]("room1"))
	adapter.AddAll("s2", types.NewSet[Room]("room1", "room2"))
	adapter.AddAll("s3", types.NewSet[Room]("room2"))

	// Sockets in room1 (note: apply() uses nsp.Sockets() which won't have our fake sids,
	// so we test via Rooms() directly)
	ids, ok := adapter.Rooms().Load("room1")
	if !ok {
		t.Fatal("Expected room1 to exist")
	}
	if ids.Len() != 2 {
		t.Errorf("Expected 2 sockets in room1, got %d", ids.Len())
	}
	if !ids.Has("s1") || !ids.Has("s2") {
		t.Error("Expected room1 to contain s1 and s2")
	}
}

func TestAdapterServerCount(t *testing.T) {
	adapter := newTestAdapter()

	if count := adapter.ServerCount(); count != 1 {
		t.Errorf("Expected ServerCount() = 1, got %d", count)
	}
}

func TestAdapterRoomEvents(t *testing.T) {
	adapter := newTestAdapter()

	var createdRooms []Room
	var joinedPairs [][2]string
	var leftPairs [][2]string
	var deletedRooms []Room

	_ = adapter.On("create-room", func(args ...any) {
		createdRooms = append(createdRooms, args[0].(Room))
	})
	_ = adapter.On("join-room", func(args ...any) {
		joinedPairs = append(joinedPairs, [2]string{string(args[0].(Room)), string(args[1].(SocketId))})
	})
	_ = adapter.On("leave-room", func(args ...any) {
		leftPairs = append(leftPairs, [2]string{string(args[0].(Room)), string(args[1].(SocketId))})
	})
	_ = adapter.On("delete-room", func(args ...any) {
		deletedRooms = append(deletedRooms, args[0].(Room))
	})

	adapter.AddAll("s1", types.NewSet[Room]("room1"))
	if len(createdRooms) != 1 || createdRooms[0] != "room1" {
		t.Errorf("Expected create-room event for room1, got %v", createdRooms)
	}
	if len(joinedPairs) != 1 || joinedPairs[0] != [2]string{"room1", "s1"} {
		t.Errorf("Expected join-room event for (room1, s1), got %v", joinedPairs)
	}

	adapter.Del("s1", "room1")
	if len(leftPairs) != 1 || leftPairs[0] != [2]string{"room1", "s1"} {
		t.Errorf("Expected leave-room event for (room1, s1), got %v", leftPairs)
	}
	if len(deletedRooms) != 1 || deletedRooms[0] != "room1" {
		t.Errorf("Expected delete-room event for room1, got %v", deletedRooms)
	}
}

func TestAdapterAddAllIdempotent(t *testing.T) {
	adapter := newTestAdapter()

	adapter.AddAll("s1", types.NewSet[Room]("room1"))
	adapter.AddAll("s1", types.NewSet[Room]("room1"))

	rooms := adapter.SocketRooms("s1")
	if rooms.Len() != 1 {
		t.Errorf("Expected 1 room after duplicate AddAll, got %d", rooms.Len())
	}

	ids, _ := adapter.Rooms().Load("room1")
	if ids.Len() != 1 {
		t.Errorf("Expected 1 socket in room1 after duplicate AddAll, got %d", ids.Len())
	}
}
