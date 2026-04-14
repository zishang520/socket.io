package postgres

import (
	"encoding/json"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestPostgresPacket_MarshalJSON(t *testing.T) {
	t.Run("nil packet", func(t *testing.T) {
		var p *PostgresPacket
		data, err := p.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		if string(data) != "null" {
			t.Fatalf("Expected null, got %s", string(data))
		}
	})

	t.Run("empty packet", func(t *testing.T) {
		p := &PostgresPacket{}
		data, err := p.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		if len(data) == 0 {
			t.Fatal("Expected non-empty JSON")
		}
	})

	t.Run("packet with uid only", func(t *testing.T) {
		p := &PostgresPacket{
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
		p := &PostgresPacket{
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

func TestPostgresPacket_UnmarshalJSON(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var p *PostgresPacket
		err := p.UnmarshalJSON([]byte(`["server1"]`))
		if err != ErrNilPostgresPacket {
			t.Fatalf("Expected ErrNilPostgresPacket, got %v", err)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		p := &PostgresPacket{}
		err := p.UnmarshalJSON([]byte(`[]`))
		if err == nil {
			t.Fatal("Expected error for empty array")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		p := &PostgresPacket{}
		err := p.UnmarshalJSON([]byte(`{invalid}`))
		if err == nil {
			t.Fatal("Expected error for invalid JSON")
		}
	})

	t.Run("uid only", func(t *testing.T) {
		p := &PostgresPacket{}
		err := p.UnmarshalJSON([]byte(`["server1"]`))
		if err != nil {
			t.Fatalf("UnmarshalJSON failed: %v", err)
		}
		if p.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", p.Uid)
		}
		if p.Packet != nil {
			t.Fatal("Expected nil Packet")
		}
		if p.Opts != nil {
			t.Fatal("Expected nil Opts")
		}
	})

	t.Run("round trip", func(t *testing.T) {
		original := &PostgresPacket{
			Uid: "server1",
			Packet: &parser.Packet{
				Type: parser.EVENT,
				Nsp:  "/",
				Data: []any{"message", "hello"},
			},
			Opts: &adapter.PacketOptions{
				Rooms:  []socket.Room{"room1"},
				Except: []socket.Room{},
			},
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		restored := &PostgresPacket{}
		if err := json.Unmarshal(data, restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if restored.Uid != original.Uid {
			t.Fatalf("Uid mismatch: got %s, want %s", restored.Uid, original.Uid)
		}
		if restored.Packet == nil {
			t.Fatal("Expected non-nil Packet")
		}
		if restored.Packet.Nsp != "/" {
			t.Fatalf("Expected Nsp '/', got %s", restored.Packet.Nsp)
		}
	})
}
