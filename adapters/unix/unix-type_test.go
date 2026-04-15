package unix

import (
	"encoding/json"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestUnixPacket_MarshalJSON(t *testing.T) {
	t.Run("nil packet", func(t *testing.T) {
		var p *UnixPacket
		data, err := p.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		if string(data) != "null" {
			t.Fatalf("Expected null, got %s", string(data))
		}
	})

	t.Run("empty packet", func(t *testing.T) {
		p := &UnixPacket{}
		data, err := p.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		if len(data) == 0 {
			t.Fatal("Expected non-empty JSON")
		}
	})

	t.Run("packet with uid only", func(t *testing.T) {
		p := &UnixPacket{
			Uid: "server1",
		}
		data, err := p.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}

		var arr []any
		if err := json.Unmarshal(data, &arr); err != nil {
			t.Fatalf("Failed to unmarshal result: %v", err)
		}
		if len(arr) != 3 {
			t.Fatalf("Expected 3 elements, got %d", len(arr))
		}
		if arr[0] != "server1" {
			t.Fatalf("Expected uid 'server1', got %v", arr[0])
		}
	})

	t.Run("packet with all fields", func(t *testing.T) {
		p := &UnixPacket{
			Uid: "server1",
			Packet: &parser.Packet{
				Type: parser.EVENT,
				Nsp:  "/",
				Data: []any{"message", "hello"},
			},
			Opts: &adapter.PacketOptions{
				Rooms:  []socket.Room{"room1", "room2"},
				Except: []socket.Room{"room3"},
			},
		}
		data, err := p.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}

		var arr []any
		if err := json.Unmarshal(data, &arr); err != nil {
			t.Fatalf("Failed to unmarshal result: %v", err)
		}
		if len(arr) != 3 {
			t.Fatalf("Expected 3 elements, got %d", len(arr))
		}
	})
}

func TestUnixPacket_UnmarshalJSON(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var p *UnixPacket
		err := p.UnmarshalJSON([]byte(`["server1"]`))
		if err != ErrNilUnixPacket {
			t.Fatalf("Expected ErrNilUnixPacket, got %v", err)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		p := &UnixPacket{}
		err := p.UnmarshalJSON([]byte(`[]`))
		if err == nil {
			t.Fatal("Expected error for empty array")
		}
	})

	t.Run("uid only", func(t *testing.T) {
		p := &UnixPacket{}
		err := p.UnmarshalJSON([]byte(`["server1"]`))
		if err != nil {
			t.Fatalf("UnmarshalJSON failed: %v", err)
		}
		if p.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %v", p.Uid)
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		original := &UnixPacket{
			Uid: "server1",
			Packet: &parser.Packet{
				Type: parser.EVENT,
				Nsp:  "/",
				Data: []any{"message", "hello"},
			},
			Opts: &adapter.PacketOptions{
				Rooms:  []socket.Room{"room1"},
				Except: []socket.Room{"room2"},
			},
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		restored := &UnixPacket{}
		if err := json.Unmarshal(data, restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if restored.Uid != original.Uid {
			t.Fatalf("Uid mismatch: got %v, want %v", restored.Uid, original.Uid)
		}
	})
}
