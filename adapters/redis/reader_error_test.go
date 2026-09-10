package redis

import (
	"encoding/json"
	"errors"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"testing"
)

type failingWireReader struct {
	cause  error
	closed bool
}

func (r *failingWireReader) Read(p []byte) (int, error) { return copy(p, "partial"), r.cause }
func (r *failingWireReader) Close() error               { r.closed = true; return nil }
func TestWireEncodersReturnReaderErrors(t *testing.T) {
	for name, encode := range map[string]func(*failingWireReader) error{
		"packet-json": func(r *failingWireReader) error {
			_, err := json.Marshal(&RedisPacket{Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", r}}})
			return err
		},
		"packet-msgpack": func(r *failingWireReader) error {
			_, err := msgpack.Marshal(&RedisPacket{Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", r}}})
			return err
		},
		"request-json": func(r *failingWireReader) error {
			_, err := json.Marshal(&RedisRequest{Type: SERVER_SIDE_EMIT, Data: []any{"event", r}})
			return err
		},
		"request-msgpack": func(r *failingWireReader) error {
			_, err := msgpack.Marshal(&RedisRequest{Type: SERVER_SIDE_EMIT, Data: []any{"event", r}})
			return err
		},
		"response-json": func(r *failingWireReader) error {
			_, err := json.Marshal(&RedisResponse{Packet: map[string]any{"data": r}})
			return err
		},
		"streams": func(r *failingWireReader) error {
			_, err := EncodeStreamMessage(&adapter.ClusterMessage{Type: adapter.BROADCAST, Data: &adapter.BroadcastMessage{Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", r}}}}, false)
			return err
		},
		"normalize": func(r *failingWireReader) error {
			_, err := NormalizeData([]any{map[string]any{"data": r}})
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			cause := errors.New("read failed")
			r := &failingWireReader{cause: cause}
			err := encode(r)
			if !errors.Is(err, cause) {
				t.Fatalf("error=%v", err)
			}
			if !r.closed {
				t.Fatal("reader not closed")
			}
		})
	}
}
