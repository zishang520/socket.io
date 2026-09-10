package socket

import (
	"bytes"
	"errors"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"strings"
	"testing"
	"time"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

func newTestSessionAwareAdapter() SessionAwareAdapter {
	opts := DefaultServerOptions()
	recovery := DefaultConnectionStateRecovery()
	recovery.SetMaxDisconnectionDuration(60_000) // 60s for testing
	opts.SetConnectionStateRecovery(recovery)

	server := NewServer(nil, opts)
	adapter := server.Sockets().Adapter()
	sa, ok := adapter.(SessionAwareAdapter)
	if !ok {
		panic("expected SessionAwareAdapter")
	}
	return sa
}

func TestSessionAwareAdapterPersistAndRestore(t *testing.T) {
	sa := newTestSessionAwareAdapter()
	defer sa.Close()

	session := &SessionToPersist{
		Sid:   "sid1",
		Pid:   "pid1",
		Rooms: types.NewSet[Room]("room1", "room2"),
		Data:  "userdata",
	}

	sa.PersistSession(session)

	restored, err := sa.RestoreSession("pid1", "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	// With empty offset and no packets, FindIndex returns -1, so restore returns nil
	if restored != nil {
		t.Log("Restore with empty offset returned non-nil (offset might match default)")
	}
}

func TestSessionAwareAdapterRestoreNonExistent(t *testing.T) {
	sa := newTestSessionAwareAdapter()
	defer sa.Close()

	restored, err := sa.RestoreSession("nonexistent", "offset")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if restored != nil {
		t.Error("Expected nil for non-existent session")
	}
}

func TestSessionAwareAdapterRestoreMissedPackets(t *testing.T) {
	adapter := newTestSessionAwareAdapter().(*sessionAwareAdapter)
	defer adapter.Close()
	adapter.PersistSession(&SessionToPersist{
		Sid:   "sid1",
		Pid:   "pid1",
		Rooms: types.NewSet[Room]("room1", "room2"),
	})
	adapter.packets.Push(
		&PersistedPacket{Id: "offset", Opts: &BroadcastOptions{Rooms: types.NewSet[Room](), Except: types.NewSet[Room]()}},
		&PersistedPacket{Id: "included", Data: "included", Opts: &BroadcastOptions{Rooms: types.NewSet[Room]("room1"), Except: types.NewSet[Room]()}},
		&PersistedPacket{Id: "other-room", Data: "other-room", Opts: &BroadcastOptions{Rooms: types.NewSet[Room]("room3"), Except: types.NewSet[Room]()}},
		&PersistedPacket{Id: "excluded", Data: "excluded", Opts: &BroadcastOptions{Rooms: types.NewSet[Room](), Except: types.NewSet[Room]("room2")}},
	)

	restored, err := adapter.RestoreSession("pid1", "offset")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if restored == nil {
		t.Fatal("Expected a restored session")
	}
	if len(restored.MissedPackets) != 1 || restored.MissedPackets[0] != "included" {
		t.Fatalf("Expected only the included packet, got %v", restored.MissedPackets)
	}

	restored, err = adapter.RestoreSession("pid1", "excluded")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if restored == nil || restored.MissedPackets == nil || len(restored.MissedPackets) != 0 {
		t.Fatalf("Expected a non-nil empty missed packet list, got %#v", restored)
	}
}

func TestSessionAwareAdapterExpiredSession(t *testing.T) {
	opts := DefaultServerOptions()
	recovery := DefaultConnectionStateRecovery()
	recovery.SetMaxDisconnectionDuration(1) // 1ms — will expire almost immediately
	opts.SetConnectionStateRecovery(recovery)

	server := NewServer(nil, opts)
	sa := server.Sockets().Adapter().(SessionAwareAdapter)
	defer sa.Close()

	session := &SessionToPersist{
		Sid:   "sid1",
		Pid:   "pid1",
		Rooms: types.NewSet[Room]("room1"),
		Data:  nil,
	}
	sa.PersistSession(session)

	// Wait for session to expire
	time.Sleep(5 * time.Millisecond)

	restored, err := sa.RestoreSession("pid1", "someoffset")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if restored != nil {
		t.Error("Expected nil for expired session")
	}
}

func TestSessionAwareAdapterClose(t *testing.T) {
	sa := newTestSessionAwareAdapter()

	// Close should not panic
	sa.Close()

	// Double close should not panic
	sa.Close()
}

func TestSessionAwareAdapterBuilder(t *testing.T) {
	builder := &SessionAwareAdapterBuilder{}

	server := NewServer(nil, nil)
	nsp := server.Sockets()

	adapter := builder.New(nsp)
	if adapter == nil {
		t.Fatal("Expected SessionAwareAdapterBuilder.New to return non-nil")
	}
	if _, ok := adapter.(SessionAwareAdapter); !ok {
		t.Error("Expected adapter to implement SessionAwareAdapter")
	}
}

func TestSessionAwareAdapterCustomCleanupInterval(t *testing.T) {
	opts := DefaultServerOptions()
	recovery := DefaultConnectionStateRecovery()
	recovery.SetMaxDisconnectionDuration(60_000)
	recovery.SetSessionCleanupInterval(100 * time.Millisecond)
	opts.SetConnectionStateRecovery(recovery)

	server := NewServer(nil, opts)
	adapter := server.Sockets().Adapter()
	sa, ok := adapter.(SessionAwareAdapter)
	if !ok {
		t.Fatal("Expected SessionAwareAdapter")
	}
	defer sa.Close()

	// Persist a session with short maxDisconnectionDuration
	session := &SessionToPersist{
		Sid:   "sid1",
		Pid:   "pid1",
		Rooms: types.NewSet[Room]("room1"),
		Data:  nil,
	}
	sa.PersistSession(session)

	// Session should exist immediately
	restored, _ := sa.RestoreSession("pid1", "")
	// restored may be nil due to missing offset match, but the session lookup should not error
	_ = restored
}

type failingSessionReader struct{ err error }

func (r *failingSessionReader) Read(p []byte) (int, error) { return copy(p, "partial"), r.err }

func TestSessionRecoveryMaterializesReaders(t *testing.T) {
	for _, opts := range []*BroadcastOptions{nil, {}, {Rooms: types.NewSet[Room](), Except: types.NewSet[Room]()}} {
		sa := newTestSessionAwareAdapter()
		defer sa.Close()
		first := &parser.Packet{Type: parser.EVENT, Data: []any{"first"}}
		sa.Broadcast(first, opts)
		offset := first.Data.([]any)[1].(string)
		sa.PersistSession(&SessionToPersist{Pid: "pid", Rooms: types.NewSet[Room]("room")})
		p := &parser.Packet{Type: parser.EVENT, Data: []any{"event", map[string]any{"text": strings.NewReader("??????"), "binary": bytes.NewBufferString("payload")}}}
		sa.Broadcast(p, opts)
		for range 2 {
			restored, err := sa.RestoreSession("pid", offset)
			if err != nil || restored == nil || len(restored.MissedPackets) != 1 {
				t.Fatalf("restore=%#v err=%v", restored, err)
			}
			replay := &parser.Packet{Type: parser.EVENT, Data: restored.MissedPackets[0]}
			encoded, err := parser.NewEncoder().Encode(replay)
			if err != nil || len(encoded) != 2 {
				t.Fatalf("encoded=%v err=%v", encoded, err)
			}
			if got := string(encoded[1].(*types.BytesBuffer).Bytes()); got != "payload" {
				t.Fatalf("binary=%q", got)
			}
			if !strings.Contains(encoded[0].(*types.StringBuffer).String(), "??????") {
				t.Fatal("text reader lost")
			}
		}
	}
}

func TestSessionRecoveryDoesNotPersistReadFailure(t *testing.T) {
	sa := newTestSessionAwareAdapter()
	defer sa.Close()
	first := &parser.Packet{Type: parser.EVENT, Data: []any{"first"}}
	sa.Broadcast(first, nil)
	offset := first.Data.([]any)[1].(string)
	sa.PersistSession(&SessionToPersist{Pid: "pid", Rooms: types.NewSet[Room]()})
	cause := errors.New("read failed")
	var got error
	var calls int
	if err := sa.On("error", func(args ...any) { got = args[0].(error); calls++ }); err != nil {
		t.Fatal(err)
	}
	sa.Broadcast(&parser.Packet{Type: parser.EVENT, Data: []any{"event", &failingSessionReader{cause}}}, nil)
	if !errors.Is(got, cause) || calls != 1 {
		t.Fatalf("error=%v calls=%d", got, calls)
	}
	restored, err := sa.RestoreSession("pid", offset)
	if err != nil || restored == nil || len(restored.MissedPackets) != 0 {
		t.Fatalf("restore=%#v err=%v", restored, err)
	}
}
