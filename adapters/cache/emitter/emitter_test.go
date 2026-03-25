package emitter

import (
	"strings"
	"testing"

	"github.com/zishang520/socket.io/servers/socket/v3"
)

// --- NewEmitter defaults ---

func TestNewEmitter_DefaultKey(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)
	if e.opts.Key() != DefaultEmitterKey {
		t.Errorf("default key: got %q, want %q", e.opts.Key(), DefaultEmitterKey)
	}
}

func TestNewEmitter_DefaultNamespace(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)
	if e.nsp != "/" {
		t.Errorf("default nsp: got %q, want %q", e.nsp, "/")
	}
}

func TestNewEmitter_DefaultParserIsSet(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)
	if e.opts.Parser() == nil {
		t.Error("default parser should not be nil")
	}
}

func TestNewEmitter_CustomOptions(t *testing.T) {
	mock := newMock()
	opts := DefaultEmitterOptions()
	opts.SetKey("mykey")
	e := NewEmitter(mock, opts)
	if e.opts.Key() != "mykey" {
		t.Errorf("key: got %q, want %q", e.opts.Key(), "mykey")
	}
}

// --- Of ---

func TestEmitter_Of_AddsLeadingSlash(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)
	e2 := e.Of("admin")
	if e2.nsp != "/admin" {
		t.Errorf("Of('admin').nsp: got %q, want %q", e2.nsp, "/admin")
	}
}

func TestEmitter_Of_PreservesSlash(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)
	e2 := e.Of("/chat")
	if e2.nsp != "/chat" {
		t.Errorf("Of('/chat').nsp: got %q, want %q", e2.nsp, "/chat")
	}
}

func TestEmitter_Of_ReturnsDifferentEmitter(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)
	e2 := e.Of("/other")
	if e == e2 {
		t.Error("Of should return a new Emitter, not the same instance")
	}
}

// --- broadcast channel routing ---

func TestEmitter_Emit_PublishesToBroadcastChannel(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)

	if err := e.Emit("hello"); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	call := mock.lastPublish()
	// Default broadcast channel is "socket.io#/#"
	if call.channel != "socket.io#/#" {
		t.Errorf("channel: got %q, want %q", call.channel, "socket.io#/#")
	}
}

func TestEmitter_Emit_CustomNamespace(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil, "/admin")

	if err := e.Emit("event"); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	call := mock.lastPublish()
	if !strings.HasPrefix(call.channel, "socket.io#/admin#") {
		t.Errorf("channel should start with socket.io#/admin#, got %q", call.channel)
	}
}

func TestEmitter_Emit_SingleRoom_UsesRoomChannel(t *testing.T) {
	mock := newMock()
	opts := DefaultEmitterOptions()
	opts.SetSubscriptionMode("dynamic")
	e := NewEmitter(mock, opts)

	if err := e.To(socket.Room("room1")).Emit("msg", "data"); err != nil {
		t.Fatalf("Emit to room: %v", err)
	}

	call := mock.lastPublish()
	// Room-specific channel: "socket.io#/#room1#"
	if !strings.Contains(call.channel, "room1") {
		t.Errorf("expected room in channel name, got %q", call.channel)
	}
}

func TestEmitter_Emit_MultipleRooms_UsesBroadcastChannel(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)

	if err := e.To(socket.Room("r1"), socket.Room("r2")).Emit("msg"); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	call := mock.lastPublish()
	// Multiple rooms → fallback to base broadcast channel.
	if call.channel != "socket.io#/#" {
		t.Errorf("channel: got %q, want %q", call.channel, "socket.io#/#")
	}
}

// --- reserved event names ---

func TestEmitter_Emit_ReservedEvent_ReturnsError(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)

	for _, ev := range []string{"connect", "disconnect", "connect_error"} {
		if err := e.Emit(ev); err == nil {
			t.Errorf("Emit(%q) should return error for reserved event", ev)
		}
	}
}

func TestEmitter_Emit_ReservedEvent_DoesNotPublish(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)
	before := mock.publishCount()
	_ = e.Emit("connect")
	if mock.publishCount() != before {
		t.Error("Publish should not be called for reserved events")
	}
}

// --- ServerSideEmit ---

func TestEmitter_ServerSideEmit_PublishesToRequestChannel(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)

	if err := e.ServerSideEmit("ping"); err != nil {
		t.Fatalf("ServerSideEmit: %v", err)
	}

	call := mock.lastPublish()
	// Request channel: "socket.io-request#/#"
	if call.channel != "socket.io-request#/#" {
		t.Errorf("channel: got %q, want %q", call.channel, "socket.io-request#/#")
	}
}

func TestEmitter_ServerSideEmit_WithAck_ReturnsError(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)

	ack := func(...any) {}
	if err := e.ServerSideEmit("event", ack); err == nil {
		t.Error("ServerSideEmit with ack should return an error")
	}
}

// --- SocketsJoin / SocketsLeave / DisconnectSockets ---

func TestEmitter_SocketsJoin_PublishesToRequestChannel(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)

	if err := e.SocketsJoin(socket.Room("r1")); err != nil {
		t.Fatalf("SocketsJoin: %v", err)
	}

	call := mock.lastPublish()
	if call.channel != "socket.io-request#/#" {
		t.Errorf("channel: got %q, want %q", call.channel, "socket.io-request#/#")
	}
}

func TestEmitter_SocketsLeave_PublishesToRequestChannel(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)

	if err := e.SocketsLeave(socket.Room("r1")); err != nil {
		t.Fatalf("SocketsLeave: %v", err)
	}

	call := mock.lastPublish()
	if call.channel != "socket.io-request#/#" {
		t.Errorf("channel: got %q, want %q", call.channel, "socket.io-request#/#")
	}
}

func TestEmitter_DisconnectSockets_PublishesToRequestChannel(t *testing.T) {
	mock := newMock()
	e := NewEmitter(mock, nil)

	if err := e.DisconnectSockets(true); err != nil {
		t.Fatalf("DisconnectSockets: %v", err)
	}

	call := mock.lastPublish()
	if call.channel != "socket.io-request#/#" {
		t.Errorf("channel: got %q, want %q", call.channel, "socket.io-request#/#")
	}
}

// --- sharded mode ---

func TestEmitter_Sharded_Emit_UsesSPublish(t *testing.T) {
	mock := newMock()
	opts := DefaultEmitterOptions()
	opts.SetSharded(true)
	e := NewEmitter(mock, opts)

	if err := e.Emit("event"); err != nil {
		t.Fatalf("sharded Emit: %v", err)
	}

	if mock.publishCount() != 0 {
		t.Error("sharded mode should use SPublish, not Publish")
	}
	_ = mock.lastSPublish() // panics if no SPublish call was made
}

func TestEmitter_Sharded_ServerSideEmit_UsesSPublish(t *testing.T) {
	mock := newMock()
	opts := DefaultEmitterOptions()
	opts.SetSharded(true)
	e := NewEmitter(mock, opts)

	if err := e.ServerSideEmit("ping"); err != nil {
		t.Fatalf("sharded ServerSideEmit: %v", err)
	}

	if mock.publishCount() != 0 {
		t.Error("sharded ServerSideEmit should use SPublish, not Publish")
	}
	_ = mock.lastSPublish()
}
