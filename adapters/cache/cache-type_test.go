package cache

import (
	"encoding/json"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestCachePacket_MarshalUnmarshal(t *testing.T) {
	original := &CachePacket{
		Uid: adapter.ServerId("server-1"),
		Packet: &parser.Packet{
			Type: parser.EVENT,
			Nsp:  "/",
			Data: []any{"hello", "world"},
		},
		Opts: &adapter.PacketOptions{
			Rooms:  []socket.Room{"room1"},
			Except: []socket.Room{"room2"},
		},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	var decoded CachePacket
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	if decoded.Uid != original.Uid {
		t.Errorf("Uid mismatch: got %q, want %q", decoded.Uid, original.Uid)
	}
	if decoded.Packet == nil {
		t.Fatal("decoded Packet is nil")
	}
	if decoded.Packet.Nsp != original.Packet.Nsp {
		t.Errorf("Packet.Nsp mismatch: got %q, want %q", decoded.Packet.Nsp, original.Packet.Nsp)
	}
	if decoded.Opts == nil {
		t.Fatal("decoded Opts is nil")
	}
}

func TestCachePacket_UnmarshalUidOnly(t *testing.T) {
	data := `["uid-only"]`
	var p CachePacket
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Uid != "uid-only" {
		t.Errorf("Uid: got %q, want %q", p.Uid, "uid-only")
	}
	if p.Packet != nil {
		t.Errorf("expected nil Packet, got %+v", p.Packet)
	}
	if p.Opts != nil {
		t.Errorf("expected nil Opts, got %+v", p.Opts)
	}
}

func TestCachePacket_UnmarshalNilError(t *testing.T) {
	var p *CachePacket
	err := p.UnmarshalJSON([]byte(`["uid"]`))
	if err != ErrNilCachePacket {
		t.Errorf("expected ErrNilCachePacket, got %v", err)
	}
}

func TestCachePacket_UnmarshalEmptyArray(t *testing.T) {
	var p CachePacket
	err := json.Unmarshal([]byte(`[]`), &p)
	if err == nil {
		t.Error("expected error for empty array, got nil")
	}
}

func TestShouldUseDynamicChannel(t *testing.T) {
	shortRoom := socket.Room("short")          // len < PrivateRoomIdLength
	longRoom := socket.Room("12345678901234567890") // len == PrivateRoomIdLength

	tests := []struct {
		mode  SubscriptionMode
		room  socket.Room
		want  bool
		label string
	}{
		{StaticSubscriptionMode, shortRoom, false, "static/short"},
		{StaticSubscriptionMode, longRoom, false, "static/long"},
		{DynamicSubscriptionMode, shortRoom, true, "dynamic/short"},
		{DynamicSubscriptionMode, longRoom, false, "dynamic/privateLen"},
		{DynamicPrivateSubscriptionMode, shortRoom, true, "dynamicPrivate/short"},
		{DynamicPrivateSubscriptionMode, longRoom, true, "dynamicPrivate/long"},
	}

	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			got := ShouldUseDynamicChannel(tt.mode, tt.room)
			if got != tt.want {
				t.Errorf("ShouldUseDynamicChannel(%q, %q) = %v, want %v", tt.mode, tt.room, got, tt.want)
			}
		})
	}
}

func TestPrivateRoomIdLength(t *testing.T) {
	if PrivateRoomIdLength != 20 {
		t.Errorf("PrivateRoomIdLength = %d, want 20", PrivateRoomIdLength)
	}
}
