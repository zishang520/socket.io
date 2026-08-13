package adapter

import (
	"context"
	"encoding/base64"
	"reflect"
	"strconv"
	"testing"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	rediswire "github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestRedisStreamsAdapterBuilderAppliesDefaults(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	redisClient := rediswire.NewRedisClient(context.Background(), client)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")

	streamAdapter := (&RedisStreamsAdapterBuilder{Redis: redisClient}).New(nsp).(*redisStreamsAdapter)
	t.Cleanup(streamAdapter.Close)

	if streamAdapter.streamName != DefaultStreamName {
		t.Fatalf("stream name = %q, want %q", streamAdapter.streamName, DefaultStreamName)
	}
	if streamAdapter.publicChannel != DefaultChannelPrefix+"#/test#" {
		t.Fatalf("public channel = %q", streamAdapter.publicChannel)
	}
	if streamAdapter.opts.MaxLen() != DefaultStreamMaxLen {
		t.Fatalf("max length = %d, want %d", streamAdapter.opts.MaxLen(), DefaultStreamMaxLen)
	}
}

func TestRedisStreamsAdapterBuilderLifecycle(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	redisClient := rediswire.NewRedisClient(context.Background(), client)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	builder := &RedisStreamsAdapterBuilder{Redis: redisClient}

	first := builder.New(nsp).(*redisStreamsAdapter)
	second := builder.New(nsp).(*redisStreamsAdapter)
	first.Close()

	if current, ok := builder.namespaceToAdapters.Load(nsp.Name()); !ok || current != second {
		t.Fatal("closing the replaced adapter removed the active adapter")
	}
	builder.mu.Lock()
	active := builder.cancel != nil
	builder.mu.Unlock()
	if !active {
		t.Fatal("polling stopped while an adapter was active")
	}

	otherNsp := socket.NewNamespace(socket.NewServer(nil, nil), "/other")
	other := builder.New(otherNsp).(*redisStreamsAdapter)
	second.Close()
	if builder.namespaceToAdapters.Len() != 1 {
		t.Fatal("polling stopped before the last adapter closed")
	}
	builder.mu.Lock()
	active = builder.cancel != nil
	builder.mu.Unlock()
	if !active {
		t.Fatal("polling stopped before the last adapter closed")
	}

	other.Close()
	builder.mu.Lock()
	active = builder.cancel != nil
	builder.mu.Unlock()
	if active || builder.namespaceToAdapters.Len() != 0 {
		t.Fatal("polling did not stop after the last adapter closed")
	}

	third := builder.New(nsp).(*redisStreamsAdapter)
	builder.mu.Lock()
	active = builder.cancel != nil
	builder.mu.Unlock()
	if !active {
		t.Fatal("polling did not restart for a new adapter")
	}
	third.Close()
}

func TestRestoreSessionReturnsEmptyMissedPackets(t *testing.T) {
	server := miniredis.RunT(t)
	client := rds.NewClient(&rds.Options{Addr: server.Addr()})
	redisClient := rediswire.NewRedisClient(context.Background(), client)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	streamAdapter.ClusterAdapter.Construct(nsp)
	streamAdapter.redisClient = redisClient
	streamAdapter.streamName = "stream"
	streamAdapter.opts.SetSessionKeyPrefix(DefaultSessionKeyPrefix)

	session := &socket.SessionToPersist{
		Sid:   "sid",
		Pid:   "pid",
		Rooms: types.NewSet[socket.Room](),
	}
	payload, err := utils.MsgPack().Encode(session)
	if err != nil {
		t.Fatal(err)
	}
	err = client.Set(context.Background(), DefaultSessionKeyPrefix+"pid", base64.StdEncoding.EncodeToString(payload), 0).Err()
	if err != nil {
		t.Fatal(err)
	}
	err = client.XAdd(context.Background(), &rds.XAddArgs{
		Stream: "stream",
		ID:     "1-0",
		Values: map[string]any{"uid": "node", "nsp": "/test", "type": "1"},
	}).Err()
	if err != nil {
		t.Fatal(err)
	}

	restored, err := streamAdapter.RestoreSession("pid", "1-0")
	if err != nil {
		t.Fatal(err)
	}
	if restored.MissedPackets == nil || len(restored.MissedPackets) != 0 {
		t.Fatalf("missed packets = %#v, want non-nil empty slice", restored.MissedPackets)
	}
}

func TestRestoreSessionUsesSubClientForStreamReads(t *testing.T) {
	writeServer := miniredis.RunT(t)
	readServer := miniredis.RunT(t)
	writeClient := rds.NewClient(&rds.Options{Addr: writeServer.Addr()})
	readClient := rds.NewClient(&rds.Options{Addr: readServer.Addr()})
	t.Cleanup(func() {
		_ = writeClient.Close()
		_ = readClient.Close()
	})

	ctx := context.Background()
	redisClient := rediswire.NewRedisClientWithSub(ctx, writeClient, readClient)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	streamAdapter := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	streamAdapter.ClusterAdapter.Construct(nsp)
	streamAdapter.redisClient = redisClient
	streamAdapter.streamName = "stream"
	streamAdapter.opts.SetSessionKeyPrefix(DefaultSessionKeyPrefix)

	session := &socket.SessionToPersist{
		Sid:   "sid",
		Pid:   "pid",
		Rooms: types.NewSet[socket.Room](),
	}
	payload, err := utils.MsgPack().Encode(session)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeClient.Set(ctx, DefaultSessionKeyPrefix+"pid", base64.StdEncoding.EncodeToString(payload), 0).Err(); err != nil {
		t.Fatal(err)
	}

	if err = readClient.XAdd(ctx, &rds.XAddArgs{
		Stream: "stream",
		ID:     "1-0",
		Values: map[string]any{"uid": "node", "nsp": "/test", "type": "1"},
	}).Err(); err != nil {
		t.Fatal(err)
	}
	wireMessage, err := rediswire.EncodeStreamMessage(&adapter.ClusterMessage{
		Uid:  "remote",
		Nsp:  "/test",
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", "payload"}},
			Opts:   new(adapter.PacketOptions),
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = readClient.XAdd(ctx, &rds.XAddArgs{
		Stream: "stream",
		ID:     "2-0",
		Values: map[string]any(wireMessage),
	}).Err(); err != nil {
		t.Fatal(err)
	}

	restored, err := streamAdapter.RestoreSession("pid", "1-0")
	if err != nil {
		t.Fatal(err)
	}
	want := []any{[]any{"event", "payload", "2-0"}}
	if !reflect.DeepEqual(restored.MissedPackets, want) {
		t.Fatalf("missed packets = %#v, want %#v", restored.MissedPackets, want)
	}
}

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
		"uid":  123,     // Not a string
		"nsp":  true,    // Not a string
		"type": 1,       // Not a string
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
	t.Run("encode message without data", func(t *testing.T) {
		msg := &adapter.ClusterResponse{
			Uid:  "server-1",
			Nsp:  "/",
			Type: adapter.INITIAL_HEARTBEAT,
			Data: nil,
		}

		raw, err := rediswire.EncodeStreamMessage(msg, false)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

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

		raw, err := rediswire.EncodeStreamMessage(msg, false)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Data should be JSON encoded
		data := raw.Data()
		if data == "" {
			t.Fatal("Expected non-empty data")
		}
		if data[0] != '{' {
			t.Error("Expected JSON format (starting with '{')")
		}
	})

	t.Run("propagate encoding errors", func(t *testing.T) {
		_, err := rediswire.EncodeStreamMessage(&adapter.ClusterMessage{
			Uid:  "server-1",
			Nsp:  "/",
			Type: adapter.MessageType(999),
			Data: func() {},
		}, false)
		if err == nil {
			t.Fatal("Expected encoding error")
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

func TestDecode_JSONData(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "7", // FETCH_SOCKETS
		"data": `{"requestId":"req-1"}`,
	}

	result, err := rediswire.DecodeStreamMessage(rawMsg)
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

	result, err := rediswire.DecodeStreamMessage(rawMsg)
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
	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "invalid",
	}

	_, err := rediswire.DecodeStreamMessage(rawMsg)
	if err == nil {
		t.Error("Expected error for invalid type")
	}
}

func TestDecode_NoData(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "0", // INITIAL_HEARTBEAT
	}

	result, err := rediswire.DecodeStreamMessage(rawMsg)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if result.Data != nil {
		t.Errorf("Expected nil data, got %v", result.Data)
	}
}

func TestDecode_InvalidBase64(t *testing.T) {
	rawMsg := RawClusterMessage{
		"uid":  "server-1",
		"nsp":  "/",
		"type": "4",
		"data": "not-valid-base64!!!",
	}

	_, err := rediswire.DecodeStreamMessage(rawMsg)
	if err == nil {
		t.Error("Expected error for invalid base64")
	}
}

func TestDecodePubSubMessage(t *testing.T) {
	message := &adapter.ClusterMessage{
		Uid:  "server-1",
		Nsp:  "/chat",
		Type: adapter.FETCH_SOCKETS,
		Data: &adapter.FetchSocketsMessage{
			RequestId: "request-1",
			Opts:      &adapter.PacketOptions{},
		},
	}
	payload, err := rediswire.EncodeClusterMessageMsgpack(message)
	if err != nil {
		t.Fatalf("Failed to encode message: %v", err)
	}

	decoded, err := rediswire.UnmarshalClusterMessage(payload)
	if err != nil {
		t.Fatalf("Failed to decode message: %v", err)
	}
	data, ok := decoded.Data.(*adapter.FetchSocketsMessage)
	if !ok {
		t.Fatalf("Expected *FetchSocketsMessage, got %T", decoded.Data)
	}
	if data.RequestId != "request-1" {
		t.Fatalf("Expected request-1, got %q", data.RequestId)
	}
}

func TestHashCode(t *testing.T) {
	tests := []struct {
		input    string
		expected int32
	}{
		{"/", 47},
		{"/namespace-0", -1732195153},
		{"/😀", 1818066},
		{"", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := hashCode(tt.input)
			if result != tt.expected {
				t.Errorf("hashCode(%q) = %d, want %d", tt.input, result, tt.expected)
			}
			// Ensure deterministic
			if hashCode(tt.input) != result {
				t.Error("hashCode is not deterministic")
			}
		})
	}
}

func TestComputeStreamName(t *testing.T) {
	t.Run("single stream", func(t *testing.T) {
		opts := DefaultRedisStreamsAdapterOptions()
		opts.SetStreamName("socket.io")
		opts.SetStreamCount(1)

		result := computeStreamName("/chat", opts)
		if result != "socket.io" {
			t.Errorf("Expected 'socket.io', got %q", result)
		}
	})

	t.Run("multiple streams", func(t *testing.T) {
		opts := DefaultRedisStreamsAdapterOptions()
		opts.SetStreamName("socket.io")
		opts.SetStreamCount(4)

		result := computeStreamName("/chat", opts)
		expected := "socket.io-" + strconv.FormatInt(int64(hashCode("/chat"))%4, 10)
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})

	t.Run("negative hash", func(t *testing.T) {
		opts := DefaultRedisStreamsAdapterOptions()
		opts.SetStreamName("socket.io")
		opts.SetStreamCount(5)

		if result := computeStreamName("/namespace-0", opts); result != "socket.io--3" {
			t.Errorf("Expected 'socket.io--3', got %q", result)
		}
	})
}

func TestIsEphemeral(t *testing.T) {
	t.Run("broadcast without requestId is not ephemeral", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.BROADCAST,
			Data: &adapter.BroadcastMessage{},
		}
		if isEphemeral(msg) {
			t.Error("Expected false for broadcast without requestId")
		}
	})

	t.Run("broadcast with requestId is ephemeral", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.BROADCAST,
			Data: &adapter.BroadcastMessage{RequestId: new("req-1")},
		}
		if !isEphemeral(msg) {
			t.Error("Expected true for broadcast with requestId")
		}
	})

	t.Run("SERVER_SIDE_EMIT is ephemeral", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.SERVER_SIDE_EMIT,
		}
		if !isEphemeral(msg) {
			t.Error("Expected true for SERVER_SIDE_EMIT")
		}
	})

	t.Run("FETCH_SOCKETS is ephemeral", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.FETCH_SOCKETS,
		}
		if !isEphemeral(msg) {
			t.Error("Expected true for FETCH_SOCKETS")
		}
	})

	t.Run("SOCKETS_JOIN is not ephemeral", func(t *testing.T) {
		msg := &adapter.ClusterMessage{
			Type: adapter.SOCKETS_JOIN,
		}
		if isEphemeral(msg) {
			t.Error("Expected false for SOCKETS_JOIN")
		}
	})
}
