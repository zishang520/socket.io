package redis

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestRedisPacket_MarshalMsgpackAsArray(t *testing.T) {
	data, err := utils.MsgPack().Encode(RedisPacket{Uid: "server1"})
	if err != nil {
		t.Fatal(err)
	}
	var values []any
	if err := utils.MsgPack().Decode(data, &values); err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 {
		t.Fatalf("Expected 3 elements, got %d", len(values))
	}
	var packet RedisPacket
	if err := utils.MsgPack().Decode(data, &packet); err != nil {
		t.Fatal(err)
	}
	if packet.Uid != "server1" || packet.Opts == nil {
		t.Fatalf("Decoded packet = %#v", packet)
	}
}

func TestRedisPacket_MarshalJSON(t *testing.T) {
	t.Run("nil packet", func(t *testing.T) {
		var p *RedisPacket
		data, err := p.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		if string(data) != "null" {
			t.Fatalf("Expected null, got %s", string(data))
		}
	})

	t.Run("empty packet", func(t *testing.T) {
		p := &RedisPacket{}
		data, err := p.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}
		if len(data) == 0 {
			t.Fatal("Expected non-empty JSON")
		}
	})

	t.Run("packet with uid only", func(t *testing.T) {
		p := &RedisPacket{
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
		p := &RedisPacket{
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
		packet, ok := arr[1].(map[string]any)
		if !ok {
			t.Fatalf("Expected packet object, got %T", arr[1])
		}
		if got := packet["type"]; got != float64(parser.EVENT) {
			t.Fatalf("Expected numeric packet type %d, got %v", parser.EVENT, got)
		}
	})
}

func TestRedisPacket_UnmarshalJSON(t *testing.T) {
	t.Run("nil receiver", func(t *testing.T) {
		var p *RedisPacket
		err := p.UnmarshalJSON([]byte(`["server1"]`))
		if err != ErrNilRedisPacket {
			t.Fatalf("Expected ErrNilRedisPacket, got %v", err)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		p := &RedisPacket{}
		err := p.UnmarshalJSON([]byte(`[]`))
		if err == nil {
			t.Fatal("Expected error for empty array")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		p := &RedisPacket{}
		err := p.UnmarshalJSON([]byte(`{invalid}`))
		if err == nil {
			t.Fatal("Expected error for invalid JSON")
		}
	})

	t.Run("uid only", func(t *testing.T) {
		p := &RedisPacket{}
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

	t.Run("uid and packet", func(t *testing.T) {
		p := &RedisPacket{}
		err := p.UnmarshalJSON([]byte(`["server1", {"type": 2, "nsp": "/test"}]`))
		if err != nil {
			t.Fatalf("UnmarshalJSON failed: %v", err)
		}
		if p.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", p.Uid)
		}
		if p.Packet == nil {
			t.Fatal("Expected non-nil Packet")
		}
		if p.Packet.Type != parser.EVENT {
			t.Fatalf("Expected packet type EVENT, got %v", p.Packet.Type)
		}
		if p.Packet.Nsp != "/test" {
			t.Fatalf("Expected nsp '/test', got %s", p.Packet.Nsp)
		}
	})

	t.Run("all fields", func(t *testing.T) {
		p := &RedisPacket{}
		err := p.UnmarshalJSON([]byte(`["server1", {"type": 2, "nsp": "/"}, {"rooms": ["room1"]}]`))
		if err != nil {
			t.Fatalf("UnmarshalJSON failed: %v", err)
		}
		if p.Uid != "server1" {
			t.Fatalf("Expected uid 'server1', got %s", p.Uid)
		}
		if p.Packet == nil {
			t.Fatal("Expected non-nil Packet")
		}
		if p.Opts == nil {
			t.Fatal("Expected non-nil Opts")
		}
		if len(p.Opts.Rooms) != 1 || p.Opts.Rooms[0] != "room1" {
			t.Fatalf("Expected rooms ['room1'], got %v", p.Opts.Rooms)
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		original := &RedisPacket{
			Uid: "server1",
			Packet: &parser.Packet{
				Type: parser.EVENT,
				Nsp:  "/",
			},
			Opts: &adapter.PacketOptions{
				Rooms:  []socket.Room{"room1"},
				Except: []socket.Room{"room2"},
			},
		}

		data, err := original.MarshalJSON()
		if err != nil {
			t.Fatalf("MarshalJSON failed: %v", err)
		}

		restored := &RedisPacket{}
		if err := restored.UnmarshalJSON(data); err != nil {
			t.Fatalf("UnmarshalJSON failed: %v", err)
		}

		if restored.Uid != original.Uid {
			t.Fatalf("Uid mismatch: expected %s, got %s", original.Uid, restored.Uid)
		}
		if restored.Packet.Type != original.Packet.Type {
			t.Fatalf("Packet.Type mismatch: expected %v, got %v", original.Packet.Type, restored.Packet.Type)
		}
		if restored.Packet.Nsp != original.Packet.Nsp {
			t.Fatalf("Packet.Nsp mismatch: expected %s, got %s", original.Packet.Nsp, restored.Packet.Nsp)
		}
	})
}

func TestRedisRequest_JSON(t *testing.T) {
	t.Run("marshal and unmarshal", func(t *testing.T) {
		req := &RedisRequest{
			Type:      ALL_ROOMS,
			RequestId: "req123",
			Uid:       "server1",
			Rooms:     []socket.Room{"room1", "room2"},
		}

		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var restored RedisRequest
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if restored.Type != req.Type {
			t.Fatalf("Type mismatch: expected %v, got %v", req.Type, restored.Type)
		}
		if restored.RequestId != req.RequestId {
			t.Fatalf("RequestId mismatch: expected %s, got %s", req.RequestId, restored.RequestId)
		}
		if restored.Uid != req.Uid {
			t.Fatalf("Uid mismatch: expected %s, got %s", req.Uid, restored.Uid)
		}
		if len(restored.Rooms) != len(req.Rooms) {
			t.Fatalf("Rooms length mismatch: expected %d, got %d", len(req.Rooms), len(restored.Rooms))
		}
	})

	t.Run("empty request", func(t *testing.T) {
		req := &RedisRequest{}
		data, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var restored RedisRequest
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}
	})
}

func TestRedisResponse_JSON(t *testing.T) {
	t.Run("nil response", func(t *testing.T) {
		var response *RedisResponse
		data, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != "null" {
			t.Fatalf("response = %s, want null", data)
		}
	})

	t.Run("missing sockets", func(t *testing.T) {
		var response RedisResponse
		if err := json.Unmarshal([]byte(`{"requestId":"request"}`), &response); err != nil {
			t.Fatal(err)
		}
		if response.Sockets != nil {
			t.Fatalf("sockets = %#v, want nil", response.Sockets)
		}
		data, err := json.Marshal(&response)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request"}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("marshal and unmarshal", func(t *testing.T) {
		resp := &RedisResponse{
			Type:        BROADCAST_CLIENT_COUNT,
			RequestId:   "req123",
			ClientCount: new(uint64(42)),
			Rooms:       []socket.Room{"room1"},
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var restored RedisResponse
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		if restored.Type != resp.Type {
			t.Fatalf("Type mismatch: expected %v, got %v", resp.Type, restored.Type)
		}
		if restored.RequestId != resp.RequestId {
			t.Fatalf("RequestId mismatch: expected %s, got %s", resp.RequestId, restored.RequestId)
		}
		if restored.ClientCount == nil || *restored.ClientCount != *resp.ClientCount {
			t.Fatalf("ClientCount mismatch: expected %d, got %v", *resp.ClientCount, restored.ClientCount)
		}
		var wire map[string]any
		if err := json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		if wire["clientCount"] != float64(*resp.ClientCount) {
			t.Fatalf("clientCount = %#v, want %d", wire["clientCount"], *resp.ClientCount)
		}
		if _, exists := wire["clientcount"]; exists {
			t.Fatal("response used non-Node.js clientcount field")
		}
	})

	t.Run("scalar acknowledgement", func(t *testing.T) {
		data, err := json.Marshal(&RedisResponse{
			RequestId: "request",
			Data:      "server response",
			Packet:    "client response",
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","data":"server response","packet":"client response"}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("binary acknowledgement", func(t *testing.T) {
		data, err := json.Marshal(&RedisResponse{
			RequestId: "request",
			Data:      []byte{1, 2},
		})
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(data), `{"requestId":"request","data":{"type":"Buffer","data":[1,2]}}`; got != want {
			t.Fatalf("response = %s, want %s", got, want)
		}
	})

	t.Run("with sockets", func(t *testing.T) {
		resp := &RedisResponse{
			RequestId: "req123",
			Sockets: []adapter.SocketResponse{
				{Id: "socket1"},
				{Id: "socket2"},
			},
		}

		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatalf("Marshal failed: %v", err)
		}

		var restored RedisResponse
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatalf("Unmarshal failed: %v", err)
		}

		raw, ok := restored.Sockets.(json.RawMessage)
		var sockets []adapter.SocketResponse
		if !ok || json.Unmarshal(raw, &sockets) != nil || len(sockets) != 2 {
			t.Fatalf("Expected 2 sockets, got %#v", restored.Sockets)
		}
	})

	t.Run("socket binary values", func(t *testing.T) {
		binary := []byte{1, 2}
		handshake := &socket.Handshake{Auth: map[string]any{"binary": binary}}
		sockets := []adapter.SocketResponse{{
			Id:        "socket",
			Handshake: handshake,
			Data:      map[string]any{"binary": binary},
		}}
		data, err := json.Marshal(&RedisResponse{RequestId: "request", Sockets: sockets})
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		encodedSockets := payload["sockets"].([]any)
		encoded := encodedSockets[0].(map[string]any)
		if encoded["data"].(map[string]any)["binary"].(map[string]any)["type"] != "Buffer" ||
			encoded["handshake"].(map[string]any)["auth"].(map[string]any)["binary"].(map[string]any)["type"] != "Buffer" {
			t.Fatalf("sockets = %#v, want Node.js Buffer values", encodedSockets)
		}
		if sockets[0].Rooms != nil || sockets[0].Handshake != handshake ||
			!reflect.DeepEqual(sockets[0].Data, map[string]any{"binary": binary}) ||
			!reflect.DeepEqual(handshake.Auth, map[string]any{"binary": binary}) {
			t.Fatalf("input was mutated: %#v", sockets[0])
		}
	})
}

func TestRedisResponse_MsgpackPreservesRequiredValues(t *testing.T) {
	data, err := utils.MsgPack().Encode(&RedisResponse{
		Type:        BROADCAST_CLIENT_COUNT,
		RequestId:   "request",
		ClientCount: new(uint64(0)),
	})
	if err != nil {
		t.Fatal(err)
	}

	var response map[string]any
	if err := utils.MsgPack().Decode(data, &response); err != nil {
		t.Fatal(err)
	}
	if value, exists := response["clientCount"]; !exists || value != uint64(0) {
		t.Fatalf("clientCount = %#v, want 0", value)
	}
	if _, exists := response["packet"]; exists {
		t.Fatal("nil packet must be omitted")
	}
}
