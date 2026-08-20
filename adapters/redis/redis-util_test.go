package redis

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type xaddHook struct {
	args []any
}

type closeReader struct {
	*strings.Reader
	closed bool
}

func (r *closeReader) Close() error {
	r.closed = true
	return nil
}

func (h *xaddHook) DialHook(next rds.DialHook) rds.DialHook { return next }

func (h *xaddHook) ProcessHook(rds.ProcessHook) rds.ProcessHook {
	return func(ctx context.Context, cmd rds.Cmder) error {
		h.args = append(h.args[:0], cmd.Args()...)
		cmd.(*rds.Cmd).SetVal("1-0")
		return nil
	}
}

func (h *xaddHook) ProcessPipelineHook(next rds.ProcessPipelineHook) rds.ProcessPipelineHook {
	return next
}

func TestXAddAlwaysIncludesMaxLen(t *testing.T) {
	client := rds.NewClient(&rds.Options{Addr: "unused"})
	hook := new(xaddHook)
	client.AddHook(hook)
	redisClient, err := NewRedisClient(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}

	offset, err := XAdd(redisClient, "stream", RawClusterMessage{
		"uid":  "emitter",
		"nsp":  "/",
		"type": "3",
		"data": `{"packet":{}}`,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if offset != "1-0" {
		t.Fatalf("offset = %q, want 1-0", offset)
	}
	want := []any{
		"XADD", "stream", "MAXLEN", "~", int64(0), "*",
		"uid", "emitter", "nsp", "/", "type", "3", "data", `{"packet":{}}`,
	}
	if !reflect.DeepEqual(hook.args, want) {
		t.Fatalf("XADD args = %#v, want %#v", hook.args, want)
	}
}

func TestRedisRequestNodeWireFields(t *testing.T) {
	tests := []struct {
		name    string
		request *RedisRequest
		field   string
		want    any
	}{
		{"sockets type and rooms", &RedisRequest{Type: SOCKETS}, "rooms", []any{}},
		{"join rooms", &RedisRequest{Type: REMOTE_JOIN, Opts: new(adapter.PacketOptions)}, "rooms", []any{}},
		{"leave rooms", &RedisRequest{Type: REMOTE_LEAVE, Opts: new(adapter.PacketOptions)}, "rooms", []any{}},
		{"disconnect close", &RedisRequest{Type: REMOTE_DISCONNECT, Close: new(true)}, "close", true},
		{"disconnect keep transport", &RedisRequest{Type: REMOTE_DISCONNECT, Close: new(false)}, "close", false},
		{"disconnect defaults to keeping transport", &RedisRequest{Type: REMOTE_DISCONNECT}, "close", false},
		{"server-side emit data", &RedisRequest{Type: SERVER_SIDE_EMIT}, "data", []any{}},
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

func TestRedisRequestOmitsCloseForOtherTypes(t *testing.T) {
	payload, err := json.Marshal(&RedisRequest{Type: REMOTE_JOIN})
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if _, exists := wire["close"]; exists {
		t.Fatalf("join request unexpectedly contains close: %s", payload)
	}
}

func TestRedisRequestMsgpackClosePresence(t *testing.T) {
	tests := []struct {
		name        string
		request     *RedisRequest
		wantPresent bool
	}{
		{"disconnect false", &RedisRequest{Type: REMOTE_DISCONNECT, Close: new(false)}, true},
		{"disconnect defaults to false", &RedisRequest{Type: REMOTE_DISCONNECT}, true},
		{"other request omits close", &RedisRequest{Type: REMOTE_JOIN}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := utils.MsgPack().Encode(tt.request)
			if err != nil {
				t.Fatal(err)
			}
			var restored RedisRequest
			if err = utils.MsgPack().Decode(payload, &restored); err != nil {
				t.Fatal(err)
			}
			if got := restored.Close != nil; got != tt.wantPresent {
				t.Fatalf("close presence = %t, want %t", got, tt.wantPresent)
			}
			if restored.Close != nil && *restored.Close {
				t.Fatalf("close = true, want false")
			}
		})
	}
}

func TestRedisRequestLegacyJoinLeaveOmitsRooms(t *testing.T) {
	for _, messageType := range []adapter.MessageType{REMOTE_JOIN, REMOTE_LEAVE} {
		payload, err := json.Marshal(&RedisRequest{Type: messageType, Sid: "socket", Room: "room"})
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]any
		if err = json.Unmarshal(payload, &wire); err != nil {
			t.Fatal(err)
		}
		if _, exists := wire["rooms"]; exists {
			t.Fatalf("legacy request %d contains rooms: %s", messageType, payload)
		}
		if _, exists := wire["opts"]; exists {
			t.Fatalf("legacy request %d contains opts: %s", messageType, payload)
		}
	}
}

func TestRedisRequestRejectsMissingType(t *testing.T) {
	var request RedisRequest
	if err := json.Unmarshal([]byte(`{}`), &request); !errors.Is(err, errRedisRequestMissingType) {
		t.Fatalf("JSON error = %v", err)
	}

	payload, err := utils.MsgPack().Encode(map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if err = utils.MsgPack().Decode(payload, &request); !errors.Is(err, errRedisRequestMissingType) {
		t.Fatalf("MessagePack error = %v", err)
	}
}

func TestRedisRequestSelectionOptionsOmitFlags(t *testing.T) {
	payload, err := json.Marshal(&RedisRequest{
		Type: REMOTE_FETCH,
		Opts: new(adapter.PacketOptions),
	})
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Opts map[string]any `json:"opts"`
	}
	if err = json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if _, exists := wire.Opts["flags"]; exists {
		t.Fatal("selection options contain flags")
	}
	if !reflect.DeepEqual(wire.Opts["rooms"], []any{}) || !reflect.DeepEqual(wire.Opts["except"], []any{}) {
		t.Fatalf("selection options = %#v", wire.Opts)
	}
}

func TestRedisPacketJSONBufferDoesNotMutatePacket(t *testing.T) {
	binary := []byte{0, 9, 10, 99, 100, 255}
	packet := &parser.Packet{Type: parser.EVENT, Data: []any{"event", binary}}
	payload, err := json.Marshal(&RedisPacket{Uid: "emitter", Packet: packet})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(payload), `{"type":"Buffer","data":[0,9,10,99,100,255]}`) {
		t.Fatalf("payload = %s", payload)
	}
	if got := packet.Data.([]any)[1]; !reflect.DeepEqual(got, binary) {
		t.Fatalf("packet was mutated: %#v", got)
	}
}

func TestRedisPacketMaterializesReaderForLocalBroadcast(t *testing.T) {
	packet := &parser.Packet{
		Type: parser.EVENT,
		Data: []any{"event", strings.NewReader("value")},
	}
	if _, err := json.Marshal(&RedisPacket{Uid: "node", Packet: packet}); err != nil {
		t.Fatal(err)
	}
	if got := packet.Data.([]any)[1]; got != "value" {
		t.Fatalf("materialized reader = %#v, want value", got)
	}
}

func TestClusterMessageCodec(t *testing.T) {
	message := &adapter.ClusterMessage{
		Uid:  "node-1",
		Nsp:  "/chat",
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", []byte{1, 2}}},
			Opts:   new(adapter.PacketOptions),
		},
	}
	payload, err := EncodeClusterMessage(message)
	if err != nil {
		t.Fatal(err)
	}
	if payload[0] == '{' {
		t.Fatal("binary cluster message was JSON encoded")
	}
	decoded, err := UnmarshalClusterMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	data, ok := decoded.Data.(*adapter.BroadcastMessage)
	if !ok {
		t.Fatalf("data type = %T", decoded.Data)
	}
	args := data.Packet.Data.([]any)
	if !reflect.DeepEqual(args[1], []byte{1, 2}) {
		t.Fatalf("binary argument = %#v", args[1])
	}
	if data.Opts.Rooms == nil || data.Opts.Except == nil || data.Opts.Flags == nil {
		t.Fatalf("options were not normalized: %#v", data.Opts)
	}
}

func TestClusterMessageJSONRequiredValues(t *testing.T) {
	payload, err := EncodeClusterMessage(&adapter.ClusterMessage{
		Uid:  "node-1",
		Nsp:  "/",
		Type: adapter.DISCONNECT_SOCKETS,
		Data: &adapter.DisconnectSocketsMessage{Opts: new(adapter.PacketOptions)},
	})
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Data struct {
			Opts  map[string]any `json:"opts"`
			Close *bool          `json:"close"`
		} `json:"data"`
	}
	if err = json.Unmarshal(payload, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Data.Close == nil || *wire.Data.Close {
		t.Fatalf("close = %#v, want false", wire.Data.Close)
	}
	if !reflect.DeepEqual(wire.Data.Opts["rooms"], []any{}) ||
		!reflect.DeepEqual(wire.Data.Opts["except"], []any{}) ||
		!reflect.DeepEqual(wire.Data.Opts["flags"], map[string]any{}) {
		t.Fatalf("options = %#v", wire.Data.Opts)
	}

	payload, err = EncodeClusterMessage(&adapter.ClusterMessage{
		Uid:  "node-1",
		Nsp:  "/",
		Type: adapter.BROADCAST_ACK,
		Data: &adapter.BroadcastAck{RequestId: "request"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var ack struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(payload, &ack); err != nil {
		t.Fatal(err)
	}
	if _, exists := ack.Data["packet"]; exists {
		t.Fatal("undefined packet must be omitted")
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

func TestStreamMessageOnlyPlaintextSkipsBinaryEncoding(t *testing.T) {
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
	if !strings.HasPrefix(raw.Data(), "{") {
		t.Fatalf("data = %q", raw.Data())
	}
	if !strings.Contains(raw.Data(), `"type":"Buffer","data":[1,2]`) {
		t.Fatalf("data = %q, want Node.js Buffer JSON", raw.Data())
	}
}

func TestFetchSocketsResponseEncodingDoesNotMutateInput(t *testing.T) {
	binary := []byte{1, 2}
	handshake := &socket.Handshake{Auth: map[string]any{"binary": binary}}
	sockets := []adapter.SocketResponse{{
		Id:        "socket",
		Handshake: handshake,
		Data:      map[string]any{"binary": binary},
	}}
	raw, err := EncodeStreamMessage(&adapter.ClusterMessage{
		Uid:  "node",
		Nsp:  "/",
		Type: adapter.FETCH_SOCKETS_RESPONSE,
		Data: &adapter.FetchSocketsResponse{RequestId: "request", Sockets: sockets},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(raw.Data(), `{"type":"Buffer","data":[1,2]}`) != 2 ||
		!strings.Contains(raw.Data(), `"rooms":[]`) {
		t.Fatalf("data = %s", raw.Data())
	}
	if sockets[0].Rooms != nil || sockets[0].Handshake != handshake ||
		!reflect.DeepEqual(sockets[0].Data, map[string]any{"binary": binary}) ||
		!reflect.DeepEqual(handshake.Auth, map[string]any{"binary": binary}) {
		t.Fatalf("input was mutated: %#v", sockets[0])
	}
}

func TestNormalizeDataClosesReader(t *testing.T) {
	reader := &closeReader{Reader: strings.NewReader("value")}
	if got := NormalizeData(reader); !reflect.DeepEqual(got, []byte("value")) {
		t.Fatalf("normalized reader = %#v", got)
	}
	if !reader.closed {
		t.Fatal("reader was not closed")
	}
}

func TestNormalizeEmptyBytesBuffer(t *testing.T) {
	buffer := new(types.BytesBuffer)
	if got := NormalizeData(buffer); !reflect.DeepEqual(got, []byte{}) {
		t.Fatalf("normalized buffer = %#v, want empty bytes", got)
	}
	if got := NormalizeJSONData(buffer); !reflect.DeepEqual(got, nodeBufferJSON{}) {
		t.Fatalf("normalized JSON buffer = %#v, want empty Buffer", got)
	}
}

func TestShouldUseDynamicChannelUsesUTF16Length(t *testing.T) {
	if ShouldUseDynamicChannel(DynamicSubscriptionMode, socket.Room("abcdefghijklmnopqr😀")) {
		t.Fatal("20 UTF-16 code units should be treated as a private room")
	}
	if !ShouldUseDynamicChannel(DynamicSubscriptionMode, socket.Room("abcdefghijklmnopq😀")) {
		t.Fatal("19 UTF-16 code units should use a dynamic public channel")
	}
}
