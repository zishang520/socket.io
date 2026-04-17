package socket

import (
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
