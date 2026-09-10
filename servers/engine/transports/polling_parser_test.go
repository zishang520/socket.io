package transports

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestPollingParserErrorOrder(t *testing.T) {
	for _, tc := range []struct {
		wire string
		want []string
	}{
		{"", []string{"error"}}, {"x\x1e4later", []string{"error"}}, {"\x1e4later", []string{"error"}},
		{"4ok\x1ex\x1e4later", []string{"message", "error"}}, {"4ok\x1e", []string{"message", "error"}},
		{"4ok\x1e\x1e4later", []string{"message", "error"}},
	} {
		p := MakePolling().(*polling)
		p.Transport.(*transport).parser = parser.Parserv4()
		var got []string
		_ = p.On("packet", func(args ...any) { got = append(got, string(args[0].(*packet.Packet).Type)) })
		_ = p.On("error", func(...any) { got = append(got, "error") })
		p.OnData(types.NewStringBufferString(tc.wire))
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %v want %v", tc.wire, got, tc.want)
		}
	}
}

type pollingFailReader struct {
	err    error
	closed int
}

func (r *pollingFailReader) Read(p []byte) (int, error) { return copy(p, "partial"), r.err }
func (r *pollingFailReader) Close() error               { r.closed++; return nil }

func TestPollingSendReaderFailure(t *testing.T) {
	for _, protocol := range []string{"3", "4"} {
		t.Run(protocol, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx := types.NewHttpContext(recorder, httptest.NewRequest("GET", "http://localhost/engine.io/?EIO="+protocol, nil))
			t.Cleanup(ctx.Flush)
			p := NewPolling(ctx).(*polling)
			t.Cleanup(p.writeQueue.Close)
			p.OnRequest(ctx)
			readErr := errors.New("payload read failed")
			r := &pollingFailReader{err: readErr}
			failures, drains := 0, 0
			_ = p.On("error", func(args ...any) {
				failures++
				if !errors.Is(args[0].(error), readErr) {
					t.Errorf("error=%v", args[0])
				}
				p.Discard()
				p.Close()
			})
			_ = p.On("drain", func(...any) { drains++ })
			p.Send([]*packet.Packet{{Type: packet.MESSAGE, Data: r}})
			p.writeQueue.Close()
			if failures != 1 || drains != 0 || r.closed != 1 || p.Writable() || p.ReadyState() != "closed" {
				t.Fatalf("errors=%d drains=%d closes=%d writable=%v state=%v", failures, drains, r.closed, p.Writable(), p.ReadyState())
			}
			if !ctx.IsDone() || p.req.Load() != nil || recorder.Code != http.StatusInternalServerError || recorder.Body.Len() != 0 {
				t.Fatalf("done=%v pending=%v status=%d body=%q", ctx.IsDone(), p.req.Load() != nil, recorder.Code, recorder.Body.String())
			}
		})
	}
}
