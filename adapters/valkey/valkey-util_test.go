package valkey

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestRequestTypesMatchNodeProtocol(t *testing.T) {
	requestTypes := []RequestType{
		SOCKETS,
		ALL_ROOMS,
		REMOTE_JOIN,
		REMOTE_LEAVE,
		REMOTE_DISCONNECT,
		REMOTE_FETCH,
		SERVER_SIDE_EMIT,
		BROADCAST,
		BROADCAST_CLIENT_COUNT,
		BROADCAST_ACK,
	}
	for value, requestType := range requestTypes {
		if int(requestType) != value {
			t.Fatalf("request type %d = %d", value, requestType)
		}
	}
}

func TestValkeyRequestNodeWireFields(t *testing.T) {
	tests := []struct {
		name    string
		request *ValkeyRequest
		field   string
		want    any
	}{
		{"sockets type and rooms", &ValkeyRequest{Type: SOCKETS}, "rooms", []any{}},
		{"join rooms", &ValkeyRequest{Type: REMOTE_JOIN, Opts: new(adapter.PacketOptions)}, "rooms", []any{}},
		{"leave rooms", &ValkeyRequest{Type: REMOTE_LEAVE, Opts: new(adapter.PacketOptions)}, "rooms", []any{}},
		{"disconnect close", &ValkeyRequest{Type: REMOTE_DISCONNECT, Close: new(true)}, "close", true},
		{"disconnect keep transport", &ValkeyRequest{Type: REMOTE_DISCONNECT, Close: new(false)}, "close", false},
		{"disconnect default", &ValkeyRequest{Type: REMOTE_DISCONNECT}, "close", false},
		{"server-side emit data", &ValkeyRequest{Type: SERVER_SIDE_EMIT}, "data", []any{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := json.Marshal(tt.request)
			if err != nil {
				t.Fatal(err)
			}
			var wire map[string]any
			if err := json.Unmarshal(payload, &wire); err != nil {
				t.Fatal(err)
			}
			if got := wire["type"]; got != float64(tt.request.Type) {
				t.Fatalf("type = %#v, want %d", got, tt.request.Type)
			}
			if got := wire[tt.field]; !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("%s = %#v, want %#v", tt.field, got, tt.want)
			}
		})
	}
}

func TestValkeyRequestRequiresType(t *testing.T) {
	var request ValkeyRequest
	if err := json.Unmarshal([]byte(`{}`), &request); !errors.Is(err, errValkeyRequestMissingType) {
		t.Fatalf("JSON error = %v", err)
	}

	payload, err := utils.MsgPack().Encode(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if err = utils.MsgPack().Decode(payload, &request); !errors.Is(err, errValkeyRequestMissingType) {
		t.Fatalf("MessagePack error = %v", err)
	}
}

func TestValkeyResponseNodeWireFields(t *testing.T) {
	clientCount := uint64(0)
	payload, err := json.Marshal(&ValkeyResponse{
		Type:        BROADCAST_CLIENT_COUNT,
		RequestId:   "request",
		ClientCount: &clientCount,
		Data:        []byte{1, 2},
		Packet:      "ack",
	})
	if err != nil {
		t.Fatal(err)
	}

	var wire map[string]any
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if count, exists := wire["clientCount"]; !exists || count != float64(0) {
		t.Fatalf("clientCount = %#v", count)
	}
	if _, exists := wire["clientcount"]; exists {
		t.Fatal("response used non-Node.js clientcount field")
	}
	if got := wire["data"].(map[string]any)["type"]; got != "Buffer" {
		t.Fatalf("buffer type = %#v", got)
	}
	if wire["packet"] != "ack" {
		t.Fatalf("packet = %#v", wire["packet"])
	}
}

func TestValkeyPacketWireEncoding(t *testing.T) {
	binary := []byte{0, 9, 255}
	packet := &parser.Packet{Type: parser.EVENT, Data: []any{"event", binary}}
	payload, err := json.Marshal(&ValkeyPacket{Uid: "node", Packet: packet})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `{"type":"Buffer","data":[0,9,255]}`) {
		t.Fatalf("payload = %s", payload)
	}
	if got := packet.Data.([]any)[1]; !reflect.DeepEqual(got, binary) {
		t.Fatalf("packet was mutated: %#v", got)
	}

	encoded, err := utils.MsgPack().Encode(ValkeyPacket{Uid: "node"})
	if err != nil {
		t.Fatal(err)
	}
	var values []any
	if err := utils.MsgPack().Decode(encoded, &values); err != nil {
		t.Fatal(err)
	}
	if len(values) != 3 {
		t.Fatalf("packet contains %d fields, want 3", len(values))
	}
}

func TestStreamMessageCodec(t *testing.T) {
	tests := []struct {
		name   string
		packet any
		binary bool
	}{
		{"JSON", "value", false},
		{"MessagePack", []byte{1, 2}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := &adapter.ClusterMessage{
				Uid:  "emitter",
				Nsp:  "/chat",
				Type: adapter.SERVER_SIDE_EMIT,
				Data: &adapter.ServerSideEmitMessage{Packet: []any{"event", tt.packet}},
			}
			raw, err := EncodeStreamMessage(message, false)
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.HasPrefix(raw.Data(), "{"); got == tt.binary {
				t.Fatalf("data = %q, binary = %t", raw.Data(), tt.binary)
			}
			decoded, err := DecodeStreamMessage(raw)
			if err != nil {
				t.Fatal(err)
			}
			data, ok := decoded.Data.(*adapter.ServerSideEmitMessage)
			if !ok || len(data.Packet) != 2 {
				t.Fatalf("decoded data = %#v (%T)", decoded.Data, decoded.Data)
			}
		})
	}
}

func TestStreamMessageOnlyPlaintext(t *testing.T) {
	raw, err := EncodeStreamMessage(&adapter.ClusterMessage{
		Uid:  "node",
		Nsp:  "/",
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", []byte{1, 2}}},
			Opts:   new(adapter.PacketOptions),
		},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(raw.Data(), "{") ||
		!strings.Contains(raw.Data(), `"rooms":[]`) ||
		!strings.Contains(raw.Data(), `"except":[]`) ||
		!strings.Contains(raw.Data(), `"flags":{}`) {
		t.Fatalf("data = %q", raw.Data())
	}
}

func TestShouldUseDynamicChannelUsesUTF16Length(t *testing.T) {
	if ShouldUseDynamicChannel(DynamicSubscriptionMode, socket.Room("abcdefghijklmnopqr\U0001F600")) {
		t.Fatal("20 UTF-16 code units should be treated as a private room")
	}
	if !ShouldUseDynamicChannel(DynamicSubscriptionMode, socket.Room("abcdefghijklmnopqrstuvwx")) {
		t.Fatal("24 UTF-16 code units should use a dynamic channel")
	}
	if !ShouldUseDynamicChannel(DynamicSubscriptionMode, socket.Room("abcdefghijklmnopq\U0001F600")) {
		t.Fatal("19 UTF-16 code units should use a dynamic public channel")
	}
}
