package valkey

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestValkeyResponseJSONAckFields(t *testing.T) {
	for _, test := range []struct {
		name     string
		response ValkeyResponse
		want     string
	}{
		{"server null", ValkeyResponse{Type: SERVER_SIDE_EMIT}, `{"type":6,"requestId":"request","data":null}`},
		{"broadcast null", ValkeyResponse{Type: BROADCAST_ACK}, `{"type":9,"requestId":"request","packet":null}`},
		{"server buffer", ValkeyResponse{Type: SERVER_SIDE_EMIT, Data: []byte{1, 2}}, `{"type":6,"requestId":"request","data":{"type":"Buffer","data":[1,2]}}`},
		{"broadcast buffer", ValkeyResponse{Type: BROADCAST_ACK, Packet: []byte{1, 2}}, `{"type":9,"requestId":"request","packet":{"type":"Buffer","data":[1,2]}}`},
		{"empty buffer", ValkeyResponse{Type: BROADCAST_ACK, Packet: new(types.BytesBuffer)}, `{"type":9,"requestId":"request","packet":{"type":"Buffer","data":[]}}`},
		{"client count", ValkeyResponse{Type: BROADCAST_CLIENT_COUNT, ClientCount: new(uint64(0))}, `{"type":8,"requestId":"request","clientCount":0}`},
		{"legacy response", ValkeyResponse{}, `{"requestId":"request"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			test.response.RequestId = "request"
			payload, err := json.Marshal(&test.response)
			if err != nil {
				t.Fatal(err)
			}
			var got, want map[string]any
			if err := json.Unmarshal(payload, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal([]byte(test.want), &want); err != nil {
				t.Fatal(err)
			}
			for field, value := range want {
				if actual, exists := got[field]; !exists || !reflect.DeepEqual(actual, value) {
					t.Fatalf("%s = %#v (present=%t), want %#v", field, actual, exists, value)
				}
			}
		})
	}
}

type packetReader struct {
	*strings.Reader
	closed int
}

func (r *packetReader) Close() error {
	r.closed++
	return nil
}

func TestValkeyPacketMaterializesReader(t *testing.T) {
	for _, format := range []string{"JSON", "MessagePack"} {
		t.Run(format, func(t *testing.T) {
			reader := &packetReader{Reader: strings.NewReader("value")}
			packet := &parser.Packet{Type: parser.EVENT, Data: []any{"event", map[string]any{"binary": reader}}}
			value := &ValkeyPacket{Uid: "node", Packet: packet}
			encode := json.Marshal
			if format == "MessagePack" {
				encode = utils.MsgPack().Encode
			}
			payload, err := encode(value)
			if err != nil {
				t.Fatal(err)
			}
			if got := packet.Data.([]any)[1].(map[string]any)["binary"]; !reflect.DeepEqual(got, []byte("value")) {
				t.Fatalf("local packet data = %#v, want native bytes", got)
			}
			if format == "JSON" {
				if !bytes.Contains(payload, []byte(`{"type":"Buffer","data":[118,97,108,117,101]}`)) {
					t.Fatalf("JSON payload = %s", payload)
				}
			} else {
				var decoded ValkeyPacket
				if err = utils.MsgPack().Decode(payload, &decoded); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(decoded.Packet.Data, packet.Data) {
					t.Fatalf("MessagePack data = %#v, want %#v", decoded.Packet.Data, packet.Data)
				}
			}
			repeated, err := encode(value)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(payload, repeated) || reader.closed != 1 {
				t.Fatalf("repeated encoding changed payload or consumed the reader again: closes=%d", reader.closed)
			}
		})
	}
}
