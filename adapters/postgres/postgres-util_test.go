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
			wire := wireData.(*PacketData[any])
			if wire.Packet != "response" {
				t.Fatalf("expected scalar packet, got %#v", wire.Packet)
			}

			decoded := UnmarshalAdapterData(test.messageType, wire)
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
