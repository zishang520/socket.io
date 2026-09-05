package postgres

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type failingBinaryReader struct {
	*bytes.Reader
}

func (*failingBinaryReader) MarshalBinary() ([]byte, error) {
	return nil, io.ErrUnexpectedEOF
}

func TestAdapterDataOptionsWireFormat(t *testing.T) {
	timeout := int64(750)
	wireData, _ := MarshalAdapterData(&adapter.BroadcastMessage{
		Packet: &parser.Packet{Type: parser.EVENT},
		Opts: &adapter.PacketOptions{
			Flags: &socket.BroadcastFlags{Timeout: &timeout},
		},
	})
	wire := wireData.(*PacketData[*parser.Packet])

	if wire.Opts.Rooms == nil || wire.Opts.Except == nil {
		t.Fatal("rooms and except must be encoded as arrays")
	}
	if wire.Opts.Flags.Timeout == nil || *wire.Opts.Flags.Timeout != 750 {
		t.Fatalf("expected timeout in milliseconds, got %v", wire.Opts.Flags.Timeout)
	}

	decoded := UnmarshalAdapterData(adapter.BROADCAST, wire).(*adapter.BroadcastMessage)
	if decoded.Opts.Flags.Timeout == nil || *decoded.Opts.Flags.Timeout != timeout {
		t.Fatalf("expected timeout %d, got %v", timeout, decoded.Opts.Flags.Timeout)
	}
}

func TestUnmarshalAdapterDataPreservesInvalidOptions(t *testing.T) {
	formats := []struct {
		name      string
		roundTrip func(any, any) error
	}{
		{
			name: "JSON",
			roundTrip: func(data, target any) error {
				payload, err := json.Marshal(data)
				if err != nil {
					return err
				}
				return json.Unmarshal(payload, target)
			},
		},
		{
			name: "MessagePack",
			roundTrip: func(data, target any) error {
				payload, err := utils.MsgPack().Encode(data)
				if err != nil {
					return err
				}
				return utils.MsgPack().Decode(payload, target)
			},
		},
	}
	tests := []struct {
		name        string
		messageType adapter.MessageType
		data        map[string]any
		wantOpts    bool
	}{
		{
			name:        "broadcast without opts",
			messageType: adapter.BROADCAST,
			data:        map[string]any{"packet": map[string]any{}},
		},
		{
			name:        "leave with missing option fields",
			messageType: adapter.SOCKETS_LEAVE,
			data: map[string]any{
				"opts":  map[string]any{},
				"rooms": []any{},
			},
			wantOpts: true,
		},
		{
			name:        "disconnect with null rooms",
			messageType: adapter.DISCONNECT_SOCKETS,
			data: map[string]any{
				"opts": map[string]any{
					"rooms":  nil,
					"except": []any{},
				},
				"close": false,
			},
			wantOpts: true,
		},
		{
			name:        "fetch with null except",
			messageType: adapter.FETCH_SOCKETS,
			data: map[string]any{
				"opts": map[string]any{
					"rooms":  []any{},
					"except": nil,
				},
				"requestId": "request-1",
			},
			wantOpts: true,
		},
	}

	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					target := AdapterDataTarget(tt.messageType)
					if err := format.roundTrip(tt.data, target); err != nil {
						t.Fatal(err)
					}

					opts := decodedPacketOptions(t, UnmarshalAdapterData(tt.messageType, target))
					if (opts != nil) != tt.wantOpts {
						t.Fatalf("options = %#v, want present %t", opts, tt.wantOpts)
					}
					if opts.IsValid() {
						t.Fatalf("invalid wire options were normalized: %#v", opts)
					}
				})
			}
		})
	}
}

func TestUnmarshalAdapterDataRequiresFetchSockets(t *testing.T) {
	formats := []struct {
		name      string
		roundTrip func(any, any) error
	}{
		{
			name: "JSON",
			roundTrip: func(data, target any) error {
				payload, err := json.Marshal(data)
				if err != nil {
					return err
				}
				return json.Unmarshal(payload, target)
			},
		},
		{
			name: "MessagePack",
			roundTrip: func(data, target any) error {
				payload, err := utils.MsgPack().Encode(data)
				if err != nil {
					return err
				}
				return utils.MsgPack().Decode(payload, target)
			},
		},
	}
	tests := []struct {
		name         string
		data         map[string]any
		wantResponse bool
	}{
		{name: "missing", data: map[string]any{"requestId": "request"}},
		{name: "null", data: map[string]any{"requestId": "request", "sockets": nil}},
		{name: "empty", data: map[string]any{"requestId": "request", "sockets": []any{}}, wantResponse: true},
	}

	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					target := AdapterDataTarget(adapter.FETCH_SOCKETS_RESPONSE)
					if err := format.roundTrip(tt.data, target); err != nil {
						t.Fatal(err)
					}
					decoded := UnmarshalAdapterData(adapter.FETCH_SOCKETS_RESPONSE, target)
					if !tt.wantResponse {
						if decoded != nil {
							t.Fatalf("decoded response = %T, want nil", decoded)
						}
						return
					}
					response, ok := decoded.(*adapter.FetchSocketsResponse)
					if !ok {
						t.Fatalf("decoded response type = %T", decoded)
					}
					if response.Sockets == nil || len(response.Sockets) != 0 {
						t.Fatalf("sockets = %#v, want non-nil empty slice", response.Sockets)
					}
				})
			}
		})
	}
}

func TestAdapterDataRequiredWireFields(t *testing.T) {
	wire, _ := MarshalAdapterData(&adapter.DisconnectSocketsMessage{Close: false})
	payload, err := json.Marshal(wire)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	encoded := string(payload)
	for _, field := range []string{`"rooms":[]`, `"except":[]`, `"close":false`} {
		if !strings.Contains(encoded, field) {
			t.Fatalf("missing %s in %s", field, encoded)
		}
	}

	trueWire, _ := MarshalAdapterData(&adapter.DisconnectSocketsMessage{Close: true})
	if close := trueWire.(*EventData).Close; close == nil || !*close {
		t.Fatalf("close = %v, want true", close)
	}
}

func TestAdapterDataScalarResponses(t *testing.T) {
	for _, test := range []struct {
		name        string
		messageType adapter.MessageType
		data        any
	}{
		{
			name:        "server-side emit response",
			messageType: adapter.SERVER_SIDE_EMIT_RESPONSE,
			data:        &adapter.ServerSideEmitResponse{RequestId: "request", Packet: "response"},
		},
		{
			name:        "broadcast acknowledgement",
			messageType: adapter.BROADCAST_ACK,
			data:        &adapter.BroadcastAck{RequestId: "request", Packet: "response"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			wireData, _ := MarshalAdapterData(test.data)
			decoded := UnmarshalAdapterData(test.messageType, wireData)
			var packet any
			switch value := decoded.(type) {
			case *adapter.ServerSideEmitResponse:
				packet = value.Packet
			case *adapter.BroadcastAck:
				packet = value.Packet
			}
			if packet != "response" {
				t.Fatalf("expected scalar packet, got %#v", packet)
			}
		})
	}
}

func TestMarshalAdapterDataBinary(t *testing.T) {
	var nilBuffer *bytes.Buffer
	tests := []struct {
		name    string
		message *adapter.ClusterMessage
		want    bool
	}{
		{
			name: "broadcast",
			message: &adapter.ClusterMessage{Type: adapter.BROADCAST, Data: &adapter.BroadcastMessage{
				Packet: &parser.Packet{Data: []any{"event", []byte{1}}},
			}},
			want: true,
		},
		{
			name:    "server-side emit",
			message: &adapter.ClusterMessage{Type: adapter.SERVER_SIDE_EMIT, Data: &adapter.ServerSideEmitMessage{Packet: []any{[]byte{1}}}},
			want:    true,
		},
		{
			name: "nested value",
			message: &adapter.ClusterMessage{Type: adapter.BROADCAST, Data: &adapter.BroadcastMessage{
				Packet: &parser.Packet{Data: map[string]any{
					"payload": map[string]any{"file": []byte{1}},
				}},
			}},
			want: true,
		},
		{
			name: "bytes buffer",
			message: &adapter.ClusterMessage{Type: adapter.BROADCAST, Data: &adapter.BroadcastMessage{
				Packet: &parser.Packet{Data: []any{types.NewBytesBuffer([]byte{1})}},
			}},
			want: true,
		},
		{
			name: "string buffer",
			message: &adapter.ClusterMessage{Type: adapter.BROADCAST, Data: &adapter.BroadcastMessage{
				Packet: &parser.Packet{Data: []any{types.NewStringBufferString("text")}},
			}},
		},
		{
			name: "strings reader",
			message: &adapter.ClusterMessage{Type: adapter.BROADCAST, Data: &adapter.BroadcastMessage{
				Packet: &parser.Packet{Data: []any{strings.NewReader("text")}},
			}},
		},
		{
			name: "nil reader",
			message: &adapter.ClusterMessage{Type: adapter.BROADCAST, Data: &adapter.BroadcastMessage{
				Packet: &parser.Packet{Data: []any{nilBuffer}},
			}},
		},
		{
			name:    "array acknowledgement",
			message: &adapter.ClusterMessage{Type: adapter.BROADCAST_ACK, Data: &adapter.BroadcastAck{Packet: []any{"first", []byte{1}}}},
			want:    true,
		},
		{
			name:    "heartbeat",
			message: &adapter.ClusterMessage{Type: adapter.HEARTBEAT},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, got := MarshalAdapterData(test.message.Data)
			if got != test.want {
				t.Fatalf("binary = %t, want %t", got, test.want)
			}
		})
	}
}

func TestAdapterDataMessagePackBytesBuffer(t *testing.T) {
	wire, _ := MarshalAdapterData(&adapter.BroadcastMessage{
		Packet: &parser.Packet{Data: []any{"event", types.NewBytesBuffer([]byte{1, 2, 3})}},
	})
	payload, err := utils.MsgPack().Encode(wire)
	if err != nil {
		t.Fatalf("MessagePack encode failed: %v", err)
	}

	decoded := &PacketData[*parser.Packet]{}
	if err := utils.MsgPack().Decode(payload, decoded); err != nil {
		t.Fatalf("MessagePack decode failed: %v", err)
	}
	packet := decoded.Packet.Data.([]any)
	binary, ok := packet[1].([]byte)
	if !ok || !bytes.Equal(binary, []byte{1, 2, 3}) {
		t.Fatalf("unexpected binary payload: %#v", packet[1])
	}
}

func TestAdapterDataMessagePackReader(t *testing.T) {
	message := &adapter.ClusterMessage{
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: &parser.Packet{Data: []any{"event", bytes.NewBuffer([]byte{1, 2, 3})}},
		},
	}
	wire, binary := MarshalAdapterData(message.Data)
	if !binary {
		t.Fatal("reader must be detected as binary")
	}
	encoded, err := utils.MsgPack().Encode(wire)
	if err != nil {
		t.Fatalf("MessagePack encode failed: %v", err)
	}

	decoded := &PacketData[*parser.Packet]{}
	if err := utils.MsgPack().Decode(encoded, decoded); err != nil {
		t.Fatalf("MessagePack decode failed: %v", err)
	}
	packet := decoded.Packet.Data.([]any)
	if data, ok := packet[1].([]byte); !ok || !bytes.Equal(data, []byte{1, 2, 3}) {
		t.Fatalf("unexpected binary payload: %#v", packet[1])
	}
	localPacket := message.Data.(*adapter.BroadcastMessage).Packet.Data.([]any)
	if data, ok := localPacket[1].([]byte); !ok || !bytes.Equal(data, []byte{1, 2, 3}) {
		t.Fatalf("local packet was consumed: %#v", localPacket[1])
	}
}

func TestFetchSocketsResponseBufferWireFormats(t *testing.T) {
	authBytes := []byte{1, 2}
	dataBytes := []byte{3, 4}
	response := &adapter.FetchSocketsResponse{
		RequestId: "request",
		Sockets: []adapter.SocketResponse{{
			Id: "socket",
			Handshake: &socket.Handshake{
				Auth: map[string]any{"token": authBytes},
			},
			Data: map[string]any{"payload": dataBytes},
		}},
	}

	wireData, binary := MarshalAdapterData(response)
	if binary {
		t.Fatal("fetch sockets response must use the JSON path below the payload threshold")
	}

	jsonData, err := json.Marshal(wireData)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`"rooms":[]`,
		`"token":{"type":"Buffer","data":[1,2]}`,
		`"payload":{"type":"Buffer","data":[3,4]}`,
	} {
		if !bytes.Contains(jsonData, []byte(expected)) {
			t.Fatalf("JSON payload %s does not contain %s", jsonData, expected)
		}
	}

	socketResponse := response.Sockets[0]
	if got := socketResponse.Handshake.Auth["token"]; !bytes.Equal(got.([]byte), authBytes) {
		t.Fatalf("input auth was modified: %#v", got)
	}
	if got := socketResponse.Data.(map[string]any)["payload"]; !bytes.Equal(got.([]byte), dataBytes) {
		t.Fatalf("input data was modified: %#v", got)
	}

	msgpackData, err := utils.MsgPack().Encode(wireData)
	if err != nil {
		t.Fatal(err)
	}
	var decoded EventData
	if err := utils.MsgPack().Decode(msgpackData, &decoded); err != nil {
		t.Fatal(err)
	}
	decodedSocket := (*decoded.Sockets)[0]
	if got := decodedSocket.Handshake.Auth["token"]; !bytes.Equal(got.([]byte), authBytes) {
		t.Fatalf("MessagePack auth = %#v, want native bytes", got)
	}
	if got := decodedSocket.Data.(map[string]any)["payload"]; !bytes.Equal(got.([]byte), dataBytes) {
		t.Fatalf("MessagePack data = %#v, want native bytes", got)
	}
}

func TestMarshalAdapterDataZeroValueBytesBuffer(t *testing.T) {
	wireData, binary := MarshalAdapterData(&adapter.BroadcastMessage{
		Packet: &parser.Packet{Data: []any{"event", new(types.BytesBuffer)}},
	})
	if !binary {
		t.Fatal("BytesBuffer must use the attachment table")
	}
	payload := wireData.(*PacketData[*parser.Packet]).Packet.Data.([]any)[1]
	data, ok := payload.([]byte)
	if !ok || data == nil || len(data) != 0 {
		t.Fatalf("zero-value BytesBuffer = %#v, want non-nil empty bytes", payload)
	}
}

func TestAdapterDataFailingBinaryMarshalerReader(t *testing.T) {
	want := []byte{1, 2, 3}
	message := &adapter.BroadcastMessage{
		Packet: &parser.Packet{
			Data: []any{"event", &failingBinaryReader{Reader: bytes.NewReader(want)}},
		},
	}

	wireData, binary := MarshalAdapterData(message)
	if !binary {
		t.Fatal("reader must use the attachment table")
	}
	packet := wireData.(*PacketData[*parser.Packet]).Packet.Data.([]any)
	if got, ok := packet[1].([]byte); !ok || !bytes.Equal(got, want) {
		t.Fatalf("unexpected wire payload: %#v", packet[1])
	}
	local := message.Packet.Data.([]any)
	if got, ok := local[1].([]byte); !ok || !bytes.Equal(got, want) {
		t.Fatalf("local payload was not preserved: %#v", local[1])
	}
}

func TestAdapterDataTextReaders(t *testing.T) {
	for _, test := range []struct {
		name string
		data any
	}{
		{name: "strings reader", data: strings.NewReader("text")},
		{name: "string buffer", data: types.NewStringBufferString("text")},
	} {
		t.Run(test.name, func(t *testing.T) {
			message := &adapter.BroadcastMessage{
				Packet: &parser.Packet{Data: []any{"event", test.data, []byte{1}}},
			}
			wireData, binary := MarshalAdapterData(message)
			if !binary {
				t.Fatal("the binary sibling must use the attachment table")
			}

			wire := wireData.(*PacketData[*parser.Packet])
			packet := wire.Packet.Data.([]any)
			if packet[1] != "text" {
				t.Fatalf("unexpected wire text: %#v", packet[1])
			}
			localPacket := message.Packet.Data.([]any)
			if localPacket[1] != "text" {
				t.Fatalf("unexpected local text: %#v", localPacket[1])
			}
		})
	}
}

func decodedPacketOptions(t *testing.T, data any) *adapter.PacketOptions {
	t.Helper()
	switch value := data.(type) {
	case *adapter.BroadcastMessage:
		return value.Opts
	case *adapter.SocketsJoinLeaveMessage:
		return value.Opts
	case *adapter.DisconnectSocketsMessage:
		return value.Opts
	case *adapter.FetchSocketsMessage:
		return value.Opts
	default:
		t.Fatalf("unexpected adapter data type: %T", data)
		return nil
	}
}
