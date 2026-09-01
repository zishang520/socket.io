package adapter

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type clusterCloseReader struct {
	*strings.Reader
	closed bool
}

func (r *clusterCloseReader) Close() error {
	r.closed = true
	return nil
}

func TestClusterMessageCodecSupportsEveryMessageType(t *testing.T) {
	tests := []struct {
		name    string
		message *ClusterMessage
	}{
		{"initial heartbeat", &ClusterMessage{Type: INITIAL_HEARTBEAT}},
		{"heartbeat", &ClusterMessage{Type: HEARTBEAT}},
		{"broadcast", &ClusterMessage{Type: BROADCAST, Data: &BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", "value"}},
		}}},
		{"sockets join", &ClusterMessage{Type: SOCKETS_JOIN, Data: &SocketsJoinLeaveMessage{}}},
		{"sockets leave", &ClusterMessage{Type: SOCKETS_LEAVE, Data: &SocketsJoinLeaveMessage{}}},
		{"disconnect sockets", &ClusterMessage{Type: DISCONNECT_SOCKETS, Data: &DisconnectSocketsMessage{}}},
		{"fetch sockets", &ClusterMessage{Type: FETCH_SOCKETS, Data: &FetchSocketsMessage{RequestId: "request"}}},
		{"fetch sockets response", &ClusterMessage{Type: FETCH_SOCKETS_RESPONSE, Data: &FetchSocketsResponse{
			RequestId: "request",
			Sockets:   []SocketResponse{{Id: "socket"}},
		}}},
		{"server-side emit", &ClusterMessage{Type: SERVER_SIDE_EMIT, Data: &ServerSideEmitMessage{
			Packet: []any{"event", "value"},
		}}},
		{"server-side emit response", &ClusterMessage{Type: SERVER_SIDE_EMIT_RESPONSE, Data: &ServerSideEmitResponse{
			RequestId: "request",
			Packet:    "value",
		}}},
		{"broadcast client count", &ClusterMessage{Type: BROADCAST_CLIENT_COUNT, Data: &BroadcastClientCount{
			RequestId:   "request",
			ClientCount: 2,
		}}},
		{"broadcast ack", &ClusterMessage{Type: BROADCAST_ACK, Data: &BroadcastAck{
			RequestId: "request",
			Packet:    "value",
		}}},
		{"adapter close", &ClusterMessage{Type: ADAPTER_CLOSE}},
	}

	encoders := []struct {
		name   string
		encode func(*ClusterMessage) ([]byte, error)
	}{
		{"automatic", EncodeClusterMessage},
		{"MessagePack", EncodeClusterMessageMsgpack},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.message.Uid = "node"
			tt.message.Nsp = "/chat"
			for _, encoder := range encoders {
				t.Run(encoder.name, func(t *testing.T) {
					payload, err := encoder.encode(tt.message)
					if err != nil {
						t.Fatal(err)
					}
					if encoder.name == "automatic" && payload[0] != '{' {
						t.Fatalf("plaintext message used MessagePack: %x", payload)
					}

					decoded, err := DecodeClusterMessage(payload)
					if err != nil {
						t.Fatal(err)
					}
					if decoded.Uid != tt.message.Uid || decoded.Nsp != tt.message.Nsp || decoded.Type != tt.message.Type {
						t.Fatalf("decoded envelope = %#v", decoded)
					}
					if reflect.TypeOf(decoded.Data) != reflect.TypeOf(tt.message.Data) {
						t.Fatalf("decoded data type = %T, want %T", decoded.Data, tt.message.Data)
					}
				})
			}
		})
	}
}

func TestClusterMessageCodecNormalizesRequiredValues(t *testing.T) {
	tests := []struct {
		name    string
		message *ClusterMessage
		check   func(*testing.T, any)
	}{
		{
			name:    "broadcast options",
			message: &ClusterMessage{Type: BROADCAST, Data: &BroadcastMessage{Packet: new(parser.Packet)}},
			check: func(t *testing.T, data any) {
				opts := data.(*BroadcastMessage).Opts
				if opts == nil || opts.Rooms == nil || opts.Except == nil || opts.Flags == nil {
					t.Fatalf("options = %#v", opts)
				}
			},
		},
		{
			name:    "join rooms",
			message: &ClusterMessage{Type: SOCKETS_JOIN, Data: new(SocketsJoinLeaveMessage)},
			check: func(t *testing.T, data any) {
				message := data.(*SocketsJoinLeaveMessage)
				if message.Rooms == nil || message.Opts == nil || message.Opts.Rooms == nil ||
					message.Opts.Except == nil || message.Opts.Flags == nil {
					t.Fatalf("message = %#v", message)
				}
			},
		},
		{
			name: "socket response rooms",
			message: &ClusterMessage{Type: FETCH_SOCKETS_RESPONSE, Data: &FetchSocketsResponse{
				Sockets: []SocketResponse{{Id: "socket"}},
			}},
			check: func(t *testing.T, data any) {
				sockets := data.(*FetchSocketsResponse).Sockets
				if sockets == nil || sockets[0].Rooms == nil {
					t.Fatalf("sockets = %#v", sockets)
				}
			},
		},
		{
			name:    "empty socket response",
			message: &ClusterMessage{Type: FETCH_SOCKETS_RESPONSE, Data: new(FetchSocketsResponse)},
			check: func(t *testing.T, data any) {
				if data.(*FetchSocketsResponse).Sockets == nil {
					t.Fatal("sockets must be an empty slice")
				}
			},
		},
		{
			name:    "server-side emit packet",
			message: &ClusterMessage{Type: SERVER_SIDE_EMIT, Data: new(ServerSideEmitMessage)},
			check: func(t *testing.T, data any) {
				if data.(*ServerSideEmitMessage).Packet == nil {
					t.Fatal("packet must be an empty slice")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := EncodeClusterMessage(tt.message)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := DecodeClusterMessage(payload)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, decoded.Data)
		})
	}
}

func TestClusterMessageCodecPreservesBinaryData(t *testing.T) {
	binary := []byte{0, 1, 2, 255}
	tests := []struct {
		name    string
		message *ClusterMessage
		get     func(*testing.T, any) any
	}{
		{
			name: "broadcast",
			message: &ClusterMessage{Type: BROADCAST, Data: &BroadcastMessage{
				Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", binary}},
			}},
			get: func(_ *testing.T, data any) any { return data.(*BroadcastMessage).Packet.Data.([]any)[1] },
		},
		{
			name: "fetch sockets response",
			message: &ClusterMessage{Type: FETCH_SOCKETS_RESPONSE, Data: &FetchSocketsResponse{
				Sockets: []SocketResponse{{
					Handshake: &socket.Handshake{Auth: map[string]any{"binary": binary}},
					Data:      map[string]any{"binary": binary},
				}},
			}},
			get: func(t *testing.T, data any) any {
				socket := data.(*FetchSocketsResponse).Sockets[0]
				if got := socket.Handshake.Auth["binary"]; !reflect.DeepEqual(got, binary) {
					t.Fatalf("handshake binary = %#v", got)
				}
				return socket.Data.(map[string]any)["binary"]
			},
		},
		{
			name: "server-side emit",
			message: &ClusterMessage{Type: SERVER_SIDE_EMIT, Data: &ServerSideEmitMessage{
				Packet: []any{"event", binary},
			}},
			get: func(_ *testing.T, data any) any { return data.(*ServerSideEmitMessage).Packet[1] },
		},
		{
			name: "server-side emit response",
			message: &ClusterMessage{Type: SERVER_SIDE_EMIT_RESPONSE, Data: &ServerSideEmitResponse{
				Packet: map[string]any{"binary": binary},
			}},
			get: func(_ *testing.T, data any) any {
				return data.(*ServerSideEmitResponse).Packet.(map[string]any)["binary"]
			},
		},
		{
			name: "broadcast ack",
			message: &ClusterMessage{Type: BROADCAST_ACK, Data: &BroadcastAck{
				Packet: []any{"ack", binary},
			}},
			get: func(_ *testing.T, data any) any { return data.(*BroadcastAck).Packet.([]any)[1] },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload, err := EncodeClusterMessage(tt.message)
			if err != nil {
				t.Fatal(err)
			}
			if payload[0] == '{' {
				t.Fatal("binary message was JSON encoded")
			}
			decoded, err := DecodeClusterMessage(payload)
			if err != nil {
				t.Fatal(err)
			}
			if got := tt.get(t, decoded.Data); !reflect.DeepEqual(got, binary) {
				t.Fatalf("binary = %#v, want %#v", got, binary)
			}
		})
	}
}

func TestClusterMessageCodecMaterializesReader(t *testing.T) {
	reader := &clusterCloseReader{Reader: strings.NewReader("value")}
	packet := &parser.Packet{Type: parser.EVENT, Data: []any{"event", reader}}
	payload, err := EncodeClusterMessage(&ClusterMessage{
		Type: BROADCAST,
		Data: &BroadcastMessage{Packet: packet},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reader.closed {
		t.Fatal("reader was not closed")
	}
	if got := packet.Data.([]any)[1]; !reflect.DeepEqual(got, []byte("value")) {
		t.Fatalf("materialized packet data = %#v", got)
	}
	decoded, err := DecodeClusterMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if got := decoded.Data.(*BroadcastMessage).Packet.Data.([]any)[1]; !reflect.DeepEqual(got, []byte("value")) {
		t.Fatalf("decoded reader data = %#v", got)
	}
}

func TestClusterMessageCodecRejectsInvalidInput(t *testing.T) {
	if _, err := DecodeClusterMessage(nil); err == nil {
		t.Fatal("empty message was accepted")
	}

	jsonPayload := []byte(`{"uid":"node","nsp":"/","type":999}`)
	if _, err := DecodeClusterMessage(jsonPayload); err == nil || err.Error() != "unknown message type" {
		t.Fatalf("JSON unknown type error = %v", err)
	}

	msgpackPayload, err := msgpack.Marshal(&ClusterMessage{Type: MessageType(999)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = DecodeClusterMessage(msgpackPayload); err == nil || err.Error() != "unknown message type" {
		t.Fatalf("MessagePack unknown type error = %v", err)
	}

	jsonPayload = []byte(`{"uid":"node","nsp":"/","type":8,"data":{"requestId":"request","sockets":null}}`)
	if _, err = DecodeClusterMessage(jsonPayload); err == nil || err.Error() != "invalid fetch sockets response" {
		t.Fatalf("JSON nil sockets error = %v", err)
	}

	msgpackPayload, err = msgpack.Marshal(&ClusterMessage{
		Type: FETCH_SOCKETS_RESPONSE,
		Data: &FetchSocketsResponse{RequestId: "request"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = DecodeClusterMessage(msgpackPayload); err == nil || err.Error() != "invalid fetch sockets response" {
		t.Fatalf("MessagePack nil sockets error = %v", err)
	}
}

func TestClusterMessageJSONRequiredValues(t *testing.T) {
	payload, err := EncodeClusterMessage(&ClusterMessage{
		Type: DISCONNECT_SOCKETS,
		Data: &DisconnectSocketsMessage{},
	})
	if err != nil {
		t.Fatal(err)
	}
	var disconnect struct {
		Data struct {
			Opts  map[string]any `json:"opts"`
			Close *bool          `json:"close"`
		} `json:"data"`
	}
	if err = json.Unmarshal(payload, &disconnect); err != nil {
		t.Fatal(err)
	}
	if disconnect.Data.Close == nil || *disconnect.Data.Close {
		t.Fatalf("close = %#v, want false", disconnect.Data.Close)
	}
	if !reflect.DeepEqual(disconnect.Data.Opts["rooms"], []any{}) ||
		!reflect.DeepEqual(disconnect.Data.Opts["except"], []any{}) ||
		!reflect.DeepEqual(disconnect.Data.Opts["flags"], map[string]any{}) {
		t.Fatalf("options = %#v", disconnect.Data.Opts)
	}

	payload, err = EncodeClusterMessage(&ClusterMessage{
		Type: BROADCAST_ACK,
		Data: &BroadcastAck{RequestId: "request"},
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
