package adapter

import (
	"encoding/json"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestNotificationMessage_Marshal(t *testing.T) {
	t.Run("with attachment", func(t *testing.T) {
		msg := &NotificationMessage{
			Uid:          "server1",
			Type:         adapter.BROADCAST,
			AttachmentId: "12345",
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var restored NotificationMessage
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
		if restored.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", restored.Uid)
		}
		if restored.AttachmentId != "12345" {
			t.Fatalf("Expected attachmentId '12345', got %s", restored.AttachmentId)
		}
	})

	t.Run("without attachment", func(t *testing.T) {
		msg := &NotificationMessage{
			Uid:  "server1",
			Type: adapter.HEARTBEAT,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}
		if len(data) == 0 {
			t.Fatal("Expected non-empty JSON")
		}
	})
}

func TestPostgresAdapter_MakePostgresAdapter(t *testing.T) {
	a := MakePostgresAdapter()
	if a == nil {
		t.Fatal("Expected non-nil adapter")
	}
}

func TestPostgresAdapter_SetChannel(t *testing.T) {
	a := MakePostgresAdapter()
	a.SetChannel("socket.io#/")
	pa := a.(*postgresAdapter)
	if pa.channel != "socket.io#/" {
		t.Fatalf("Expected channel 'socket.io#/', got %s", pa.channel)
	}
}

func TestPostgresAdapter_JsonRoundTrip(t *testing.T) {
	a := MakePostgresAdapter()
	pa := a.(*postgresAdapter)

	t.Run("ClusterMessage JSON matches Node.js format", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Uid:  "server1",
			Nsp:  "/",
			Type: adapter.BROADCAST,
			Data: &adapter.BroadcastMessage{
				Opts: &adapter.PacketOptions{
					Rooms: []socket.Room{"room1"},
				},
			},
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		// Verify Node.js compatible field names
		var raw map[string]any
		if unmarshalErr := json.Unmarshal(payload, &raw); unmarshalErr != nil {
			t.Fatalf("Failed to parse: %v", unmarshalErr)
		}
		if raw["uid"] != "server1" {
			t.Fatalf("Expected uid 'server1', got %v", raw["uid"])
		}
		if raw["nsp"] != "/" {
			t.Fatalf("Expected nsp '/', got %v", raw["nsp"])
		}
		if raw["type"].(float64) != float64(adapter.BROADCAST) {
			t.Fatalf("Expected type %d, got %v", adapter.BROADCAST, raw["type"])
		}
		if raw["data"] == nil {
			t.Fatal("Expected non-nil data")
		}

		// Verify decode roundtrip
		decoded, err := pa.decode(payload)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if decoded.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", decoded.Uid)
		}
		if decoded.Type != adapter.BROADCAST {
			t.Fatalf("Expected type BROADCAST, got %v", decoded.Type)
		}
	})

	t.Run("heartbeat without data", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Uid:  "server1",
			Nsp:  "/",
			Type: adapter.HEARTBEAT,
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			t.Fatalf("json.Marshal failed: %v", err)
		}

		decoded, err := pa.decode(payload)
		if err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if decoded.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", decoded.Uid)
		}
		if decoded.Type != adapter.HEARTBEAT {
			t.Fatalf("Expected type HEARTBEAT, got %v", decoded.Type)
		}
		if decoded.Data != nil {
			t.Fatal("Expected nil data for heartbeat")
		}
	})

	t.Run("decode invalid JSON", func(t *testing.T) {
		_, err := pa.decode([]byte(`{invalid}`))
		if err == nil {
			t.Fatal("Expected error for invalid JSON")
		}
	})
}

func TestPostgresAdapter_MsgpackRoundTrip(t *testing.T) {
	a := MakePostgresAdapter()
	pa := a.(*postgresAdapter)

	t.Run("encode and decode msgpack", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Uid:  "server1",
			Nsp:  "/",
			Type: adapter.SOCKETS_JOIN,
			Data: &adapter.SocketsJoinLeaveMessage{
				Opts: &adapter.PacketOptions{
					Rooms: []socket.Room{"room1"},
				},
				Rooms: []socket.Room{"target-room"},
			},
		}

		// Encode with msgpack (as attachment would)
		encoded, err := utils.MsgPack().Encode(msg)
		if err != nil {
			t.Fatalf("msgpack encode failed: %v", err)
		}

		// Decode with decodeMsgpack
		decoded, err := pa.decodeMsgpack(encoded)
		if err != nil {
			t.Fatalf("decodeMsgpack failed: %v", err)
		}

		if decoded.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", decoded.Uid)
		}
		if decoded.Type != adapter.SOCKETS_JOIN {
			t.Fatalf("Expected type SOCKETS_JOIN, got %v", decoded.Type)
		}
	})
}

func TestPostgresAdapter_HasBinary(t *testing.T) {
	t.Run("nil data", func(t *testing.T) {
		msg := &adapter.ClusterMessage{Type: adapter.BROADCAST}
		if hasBinary(msg) {
			t.Error("Expected false for nil data")
		}
	})

	t.Run("heartbeat type", func(t *testing.T) {
		msg := &adapter.ClusterMessage{Type: adapter.HEARTBEAT, Data: "test"}
		if hasBinary(msg) {
			t.Error("Expected false for heartbeat type")
		}
	})
}

func TestPostgresAdapterBuilder(t *testing.T) {
	t.Run("creates builder", func(t *testing.T) {
		builder := &PostgresAdapterBuilder{}
		if builder.Postgres != nil {
			t.Fatal("Expected nil Postgres initially")
		}
	})
}
