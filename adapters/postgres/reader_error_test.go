package postgres

import (
	"errors"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"testing"
)

type failingWireReader struct {
	cause  error
	closed bool
}

func (r *failingWireReader) Read(p []byte) (int, error) { return copy(p, "partial"), r.cause }
func (r *failingWireReader) Close() error               { r.closed = true; return nil }
func TestMarshalAdapterDataReturnsReaderErrors(t *testing.T) {
	for name, makeData := range map[string]func(*failingWireReader) any{
		"broadcast": func(r *failingWireReader) any {
			return &adapter.BroadcastMessage{Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", r}}}
		},
		"emit": func(r *failingWireReader) any { return &adapter.ServerSideEmitMessage{Packet: []any{"event", r}} },
		"ack":  func(r *failingWireReader) any { return &adapter.BroadcastAck{Packet: map[string]any{"data": r}} },
		"socket-data": func(r *failingWireReader) any {
			return &adapter.FetchSocketsResponse{Sockets: []adapter.SocketResponse{{Data: r}}}
		},
		"auth": func(r *failingWireReader) any {
			return &adapter.FetchSocketsResponse{Sockets: []adapter.SocketResponse{{Handshake: &socket.Handshake{Auth: map[string]any{"data": r}}}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			cause := errors.New("read failed")
			r := &failingWireReader{cause: cause}
			data := makeData(r)
			_, _, err := MarshalAdapterData(data)
			if !errors.Is(err, cause) {
				t.Fatalf("error=%v", err)
			}
			if !r.closed {
				t.Fatal("reader not closed")
			}
		})
	}
}
