package adapter

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestRawClusterMessage_Getters(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/chat",
		"type": "1",
		"data": `{"key":"value"}`,
	}

	t.Run("Uid", func(t *testing.T) {
		if got := rawMsg.Uid(); got != "server-1" {
			t.Errorf("Expected 'server-1', got %q", got)
		}
	})

	t.Run("Nsp", func(t *testing.T) {
		if got := rawMsg.Nsp(); got != "/chat" {
			t.Errorf("Expected '/chat', got %q", got)
		}
	})

	t.Run("Type", func(t *testing.T) {
		if got := rawMsg.Type(); got != "1" {
			t.Errorf("Expected '1', got %q", got)
		}
	})

	t.Run("Data", func(t *testing.T) {
		if got := rawMsg.Data(); got != `{"key":"value"}` {
			t.Errorf("Expected JSON data, got %q", got)
		}
	})
}

func TestRawClusterMessage_EmptyValues(t *testing.T) {
	rawMsg := RawClusterMessage{}

	if rawMsg.Uid() != "" {
		t.Error("Expected empty Uid")
	}
	if rawMsg.Nsp() != "" {
		t.Error("Expected empty Nsp")
	}
	if rawMsg.Type() != "" {
		t.Error("Expected empty Type")
	}
	if rawMsg.Data() != "" {
		t.Error("Expected empty Data")
	}
}

func TestRawClusterMessage_WrongType(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  123,    // Not a string
		"nsp":  true,   // Not a string
		"type": 1,      // Not a string
		"data": []int{}, // Not a string
	}

	// All getters should return empty string for wrong types
	if rawMsg.Uid() != "" {
		t.Error("Expected empty string for wrong Uid type")
	}
	if rawMsg.Nsp() != "" {
		t.Error("Expected empty string for wrong Nsp type")
	}
	if rawMsg.Type() != "" {
		t.Error("Expected empty string for wrong Type type")
	}
	if rawMsg.Data() != "" {
		t.Error("Expected empty string for wrong Data type")
	}
}

func TestNextOffset(t *testing.T) {
	a := &redisStreamsAdapter{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "normal offset",
			input:    "1234567890-0",
			expected: "1234567890-1",
		},
		{
			name:     "increment sequence",
			input:    "1234567890-99",
			expected: "1234567890-100",
		},
		{
			name:     "large timestamp",
			input:    "1749618000000-5",
			expected: "1749618000000-6",
		},
		{
			name:     "zero sequence",
			input:    "1000000000000-0",
			expected: "1000000000000-1",
		},
		{
			name:     "no dash returns original",
			input:    "invalid",
			expected: "invalid",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only dash",
			input:    "-",
			expected: "-", // sequence part is empty, parsing will fail, return original
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := a.nextOffset(tt.input)
			if result != tt.expected {
				t.Errorf("nextOffset(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShouldIncludePacket(t *testing.T) {
	a := &redisStreamsAdapter{}

	t.Run("include when no rooms specified", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{},
			Except: []socket.Room{},
		}
		if !a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected true when no rooms specified")
		}
	})

	t.Run("include when session is in target room", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		sessionRooms.Add("room2")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{"room2"},
			Except: []socket.Room{},
		}
		if !a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected true when session is in target room")
		}
	})

	t.Run("exclude when session not in target room", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{"room2", "room3"},
			Except: []socket.Room{},
		}
		if a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected false when session is not in target rooms")
		}
	})

	t.Run("exclude when session is in except list", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{},
			Except: []socket.Room{"room1"},
		}
		if a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected false when session is in except list")
		}
	})

	t.Run("exclude takes priority over include", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		sessionRooms.Add("room2")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{"room1"},
			Except: []socket.Room{"room2"},
		}
		if a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected false when session is in both target and except")
		}
	})

	t.Run("include when session is in target but not in except", func(t *testing.T) {
		sessionRooms := types.NewSet[socket.Room]()
		sessionRooms.Add("room1")
		opts := &adapter.PacketOptions{
			Rooms:  []socket.Room{"room1"},
			Except: []socket.Room{"room2"},
		}
		if !a.shouldIncludePacket(sessionRooms, opts) {
			t.Error("Expected true when session is in target but not in except")
		}
	})
}

func TestEncode(t *testing.T) {
	a := &redisStreamsAdapter{}

	t.Run("encode message without data", func(t *testing.T) {
		msg := &adapter.ClusterResponse{
			Uid:  "server-1",
			Nsp:  "/",
			Type: adapter.INITIAL_HEARTBEAT,
			Data: nil,
		}

		raw := a.encode(msg)

		if raw.Uid() != "server-1" {
			t.Errorf("Expected uid 'server-1', got %q", raw.Uid())
		}
		if raw.Nsp() != "/" {
			t.Errorf("Expected nsp '/', got %q", raw.Nsp())
		}
		if raw.Type() != "1" {
			t.Errorf("Expected type '1', got %q", raw.Type())
		}
		if raw.Data() != "" {
			t.Errorf("Expected empty data, got %q", raw.Data())
		}
	})

	t.Run("encode JSON data", func(t *testing.T) {
		testData := &adapter.FetchSocketsMessage{
			RequestId: "req-1",
			Opts: &adapter.PacketOptions{
				Rooms: []socket.Room{"room1"},
			},
		}
		msg := &adapter.ClusterResponse{
			Uid:  "server-1",
			Nsp:  "/test",
			Type: adapter.FETCH_SOCKETS,
			Data: testData,
		}

		raw := a.encode(msg)

		// Data should be JSON encoded
		data := raw.Data()
		if data == "" {
			t.Fatal("Expected non-empty data")
		}
		if data[0] != '{' {
			t.Error("Expected JSON format (starting with '{')")
		}
	})
}

func TestDefaultStreamName(t *testing.T) {
	if DefaultStreamName != "socket.io" {
		t.Errorf("Expected 'socket.io', got %q", DefaultStreamName)
	}
}

func TestDefaultSessionKeyPrefix(t *testing.T) {
	if DefaultSessionKeyPrefix != "sio:session:" {
		t.Errorf("Expected 'sio:session:', got %q", DefaultSessionKeyPrefix)
	}
}

func TestDefaultStreamReadCount(t *testing.T) {
	if DefaultStreamReadCount != 100 {
		t.Errorf("Expected 100, got %d", DefaultStreamReadCount)
	}
}

func TestOffsetRegex(t *testing.T) {
	validOffsets := []string{
		"0-0",
		"1234567890123-0",
		"1749618000000-999",
		"0-1",
	}

	invalidOffsets := []string{
		"",
		"invalid",
		"1234567890123",
		"-1",
		"1234567890123-",
		"abc-123",
		"123-abc",
		"$",
		"*",
	}

	for _, offset := range validOffsets {
		t.Run("valid: "+offset, func(t *testing.T) {
			if !offsetRegex.MatchString(offset) {
				t.Errorf("Expected %q to be valid", offset)
			}
		})
	}

	for _, offset := range invalidOffsets {
		t.Run("invalid: "+offset, func(t *testing.T) {
			if offsetRegex.MatchString(offset) {
				t.Errorf("Expected %q to be invalid", offset)
			}
		})
	}
}

func TestDecodeData_UnknownMessageType(t *testing.T) {
	a := &redisStreamsAdapter{}

	_, err := a.decodeData(adapter.MessageType(999), json.RawMessage(`{}`))
	if err == nil {
		t.Error("Expected error for unknown message type")
	}
}

func TestDecodeData_NilPayloadTypes(t *testing.T) {
	a := &redisStreamsAdapter{}

	tests := []struct {
		name    string
		msgType adapter.MessageType
	}{
		{"INITIAL_HEARTBEAT", adapter.INITIAL_HEARTBEAT},
		{"HEARTBEAT", adapter.HEARTBEAT},
		{"ADAPTER_CLOSE", adapter.ADAPTER_CLOSE},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := a.decodeData(tt.msgType, nil)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != nil {
				t.Errorf("Expected nil result, got %v", result)
			}
		})
	}
}

func TestDecodeData_JSONFormat(t *testing.T) {
	a := &redisStreamsAdapter{}

	jsonData := json.RawMessage(`{"requestId":"test-req","opts":{"rooms":["room1"]}}`)

	result, err := a.decodeData(adapter.FETCH_SOCKETS, jsonData)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	msg, ok := result.(*adapter.FetchSocketsMessage)
	if !ok {
		t.Fatalf("Expected *FetchSocketsMessage, got %T", result)
	}
	if msg.RequestId != "test-req" {
		t.Errorf("Expected RequestId 'test-req', got %q", msg.RequestId)
	}
}

func TestDecodeData_MsgpackFormat(t *testing.T) {
	a := &redisStreamsAdapter{}

	// Create MessagePack encoded data
	testData := &adapter.FetchSocketsMessage{
		RequestId: "msgpack-req",
		Opts:      &adapter.PacketOptions{},
	}
	encoded, err := msgpack.Marshal(testData)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	result, err := a.decodeData(adapter.FETCH_SOCKETS, msgpack.RawMessage(encoded))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	msg, ok := result.(*adapter.FetchSocketsMessage)
	if !ok {
		t.Fatalf("Expected *FetchSocketsMessage, got %T", result)
	}
	if msg.RequestId != "msgpack-req" {
		t.Errorf("Expected RequestId 'msgpack-req', got %q", msg.RequestId)
	}
}

func TestDecode_JSONData(t *testing.T) {
	a := &redisStreamsAdapter{}

	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "7", // FETCH_SOCKETS
		"data": `{"requestId":"req-1"}`,
	}

	result, err := a.decode(rawMsg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Uid != "server-1" {
		t.Errorf("Expected uid 'server-1', got %q", result.Uid)
	}
	if result.Nsp != "/" {
		t.Errorf("Expected nsp '/', got %q", result.Nsp)
	}
	if result.Type != adapter.FETCH_SOCKETS {
		t.Errorf("Expected FETCH_SOCKETS type, got %v", result.Type)
	}
}

func TestDecode_Base64MsgpackData(t *testing.T) {
	a := &redisStreamsAdapter{}

	// Create base64-encoded MessagePack data
	testData := &adapter.FetchSocketsMessage{
		RequestId: "base64-req",
	}
	encoded, _ := msgpack.Marshal(testData)
	base64Data := base64.StdEncoding.EncodeToString(encoded)

	rawMsg := RawClusterMessage{
		"uid":  "server-2",
		"nsp":  "/chat",
		"type": "7", // FETCH_SOCKETS
		"data": base64Data,
	}

	result, err := a.decode(rawMsg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Uid != "server-2" {
		t.Errorf("Expected uid 'server-2', got %q", result.Uid)
	}
	if result.Type != adapter.FETCH_SOCKETS {
		t.Errorf("Expected FETCH_SOCKETS type, got %v", result.Type)
	}
}

func TestDecode_InvalidType(t *testing.T) {
	a := &redisStreamsAdapter{}

	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "invalid",
	}

	_, err := a.decode(rawMsg)
	if err == nil {
		t.Error("Expected error for invalid type")
	}
}

func TestDecode_NoData(t *testing.T) {
	a := &redisStreamsAdapter{}

	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "0", // INITIAL_HEARTBEAT
	}

	result, err := a.decode(rawMsg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Data != nil {
		t.Errorf("Expected nil data, got %v", result.Data)
	}
}

func TestDecode_InvalidBase64(t *testing.T) {
	a := &redisStreamsAdapter{}

	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "4",
		"data": "not-valid-base64!!!",
	}

	_, err := a.decode(rawMsg)
	if err == nil {
		t.Error("Expected error for invalid base64")
	}
}
