package engine

import (
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/queue"
	"github.com/zishang520/socket.io/v3/pkg/request"
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
		p := MakePolling()
		p.SetReadyState(TransportStateOpen)
		var got []string
		_ = p.On("packet", func(args ...any) { got = append(got, string(args[0].(*packet.Packet).Type)) })
		_ = p.On("error", func(...any) { got = append(got, "error"); p.SetReadyState(TransportStateClosed) })
		_ = p.On("poll", func(...any) { t.Error("started another poll after parse failure") })
		p.OnData(types.NewStringBufferString(tc.wire))
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%q: got %v want %v", tc.wire, got, tc.want)
		}
	}
}

type pollingPayloadReader struct {
	data   string
	err    error
	closed int
}

func (r *pollingPayloadReader) Read(p []byte) (int, error) {
	if r.data == "" {
		return 0, io.EOF
	}
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}

func (r *pollingPayloadReader) Close() error { r.closed++; return nil }

type pollingRoundTripper func(*http.Request) (*http.Response, error)

func (fn pollingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) { return fn(r) }

func TestPollingWriteReaderFailure(t *testing.T) {
	p := MakePolling().(*polling)
	opts := DefaultSocketOptions()
	opts.SetHostname("localhost")
	opts.SetPort("80")
	opts.SetPath("/engine.io")
	p.Transport.Construct(nil, opts)
	p.writeQueue = queue.New()
	t.Cleanup(p.writeQueue.Close)
	requests := 0
	p.client = request.NewHTTPClient(request.WithTransport(pollingRoundTripper(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok")), Request: r}, nil
	})))
	t.Cleanup(func() { _ = p.client.Close() })
	readErr := errors.New("payload read failed")
	r := &pollingPayloadReader{data: "partial", err: readErr}
	failures, drains := 0, 0
	_ = p.On("error", func(args ...any) {
		failures++
		if !errors.Is(args[0].(error), readErr) {
			t.Errorf("error=%v", args[0])
		}
	})
	_ = p.On("drain", func(...any) { drains++ })
	p.Write([]*packet.Packet{{Type: packet.MESSAGE, Data: r}})
	p.writeQueue.Close()
	if requests != 0 || failures != 1 || drains != 0 || r.closed != 1 || p.Writable() {
		t.Fatalf("requests=%d errors=%d drains=%d closes=%d writable=%v", requests, failures, drains, r.closed, p.Writable())
	}
}

type pollingBatchTransport struct {
	Transport
	sent []*packet.Packet
}

func (p *pollingBatchTransport) Name() string                   { return "polling" }
func (p *pollingBatchTransport) Write(packets []*packet.Packet) { p.sent = append(p.sent, packets...) }

func TestPollingBatchReader(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[fails], func(t *testing.T) {
			p := &pollingBatchTransport{Transport: MakeTransport()}
			p.Prototype(p)
			p.SetReadyState(TransportStateOpen)
			s := MakeSocketWithoutUpgrade().(*socketWithoutUpgrade)
			t.Cleanup(s.taskQueue.Close)
			s.opts = DefaultSocketOptions()
			s.readyState.Store(SocketStateOpen)
			s.SetTransport(p)
			s._maxPayload.Store(1000000)
			readErr := errors.New("payload read failed")
			r := &pollingPayloadReader{data: "payload"}
			if fails {
				r.err = readErr
			}
			failures, flushed := 0, 0
			_ = s.On("error", func(args ...any) {
				failures++
				if !errors.Is(args[0].(error), readErr) {
					t.Errorf("error=%v", args[0])
				}
				// Error callbacks may immediately retry flushing or close the socket.
				s.Flush()
				s.Close()
			})
			_ = s.On("flush", func(...any) { flushed++ })
			s.Write(strings.NewReader("first"), nil, nil)
			s.Write(r, nil, nil)
			p.SetWritable(true)
			s.Flush()
			if r.closed != 1 || s.writeBuffer.Len() != 0 {
				t.Fatalf("closes=%d buffered=%d", r.closed, s.writeBuffer.Len())
			}
			if fails {
				if failures != 1 || flushed != 0 || len(p.sent) != 0 || s.ReadyState() != SocketStateClosed {
					t.Fatalf("errors=%d flushes=%d sent=%d state=%v", failures, flushed, len(p.sent), s.ReadyState())
				}
				return
			}
			wire, err := parser.Parserv4().EncodePayload(p.sent)
			if err != nil {
				t.Fatal(err)
			}
			if failures != 0 || flushed != 1 || wire.String() != "4first\x1ebcGF5bG9hZA==" {
				t.Fatalf("errors=%d flushes=%d wire=%q", failures, flushed, wire.String())
			}
		})
	}
}
