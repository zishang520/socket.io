package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/queue"
	"github.com/zishang520/socket.io/v3/pkg/types"
	wire "github.com/zishang520/socket.io/v3/pkg/webtransport"
)

type transportWriteStream struct {
	bytes.Buffer
	writeErr error
}

func (s *transportWriteStream) Write(data []byte) (int, error) {
	if s.writeErr != nil {
		return 0, s.writeErr
	}
	return s.Buffer.Write(data)
}

func (*transportWriteStream) Close() error                     { return nil }
func (*transportWriteStream) SetReadDeadline(time.Time) error  { return nil }
func (*transportWriteStream) SetWriteDeadline(time.Time) error { return nil }

func newWriteTestWebTransport(t *testing.T, stream *transportWriteStream) *webTransport {
	t.Helper()
	w := MakeWebTransport().(*webTransport)
	w.Transport.Construct(nil, DefaultSocketOptions())
	w.writeQueue = queue.New()
	t.Cleanup(w.writeQueue.Close)
	w.session = &types.WebTransportConn{
		EventEmitter: types.NewEventEmitter(),
		Conn:         wire.NewConn(nil, stream, true, 0, 0, nil, nil, nil),
	}
	return w
}

func TestWebTransportHandshakeWriteFailureDoesNotOpen(t *testing.T) {
	wantErr := errors.New("handshake write failed")
	w := newWriteTestWebTransport(t, &transportWriteStream{writeErr: wantErr})
	w.SetReadyState(TransportStateOpening)
	var events []string
	_ = w.session.On("error", func(args ...any) {
		if args[0] != wantErr {
			t.Errorf("handshake error = %v, want original write error", args[0])
		}
		events = append(events, "error")
		w.SetReadyState(TransportStateClosed)
	})
	_ = w.On("open", func(...any) { events = append(events, "open") })
	w.handshake()
	if len(events) != 1 || events[0] != "error" || w.ReadyState() != TransportStateClosed || w.Writable() {
		t.Fatalf("events=%v state=%s writable=%t after failed handshake", events, w.ReadyState(), w.Writable())
	}
}

func TestWebTransportHandshakeOpensAfterWriting(t *testing.T) {
	for _, sid := range []string{"", "existing-session"} {
		t.Run(sid, func(t *testing.T) {
			stream := &transportWriteStream{}
			w := newWriteTestWebTransport(t, stream)
			opts := DefaultSocketOptions()
			opts.SetQuery(url.Values{})
			if sid != "" {
				opts.Query().Set("sid", sid)
			}
			w.Transport.Construct(nil, opts)
			opened := 0
			_ = w.On("open", func(...any) {
				opened++
				if stream.Len() == 0 {
					t.Error("opened before writing the handshake")
				}
			})
			w.handshake()
			messageType, data, err := w.session.ReadMessage()
			want := "0"
			if sid != "" {
				want += `{"sid":"existing-session"}`
			}
			if err != nil || messageType != wire.TextMessage || string(data) != want {
				t.Fatalf("handshake = (%d, %q, %v), want %q", messageType, data, err, want)
			}
			if opened != 1 || w.ReadyState() != TransportStateOpen || !w.Writable() {
				t.Fatalf("opened=%d state=%s writable=%t", opened, w.ReadyState(), w.Writable())
			}
		})
	}
}

type transportWriteConn struct {
	net.Conn
	writeErr error
	headers  []byte
}

func (c *transportWriteConn) Write(data []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	if len(data) > 0 {
		c.headers = append(c.headers, data[0])
	}
	return c.Conn.Write(data)
}

func newWriteTestWebSocket(t *testing.T, opts SocketOptionsInterface) (*websocket, *transportWriteConn, *ws.Conn) {
	t.Helper()
	upgrader := ws.Upgrader{EnableCompression: true}
	accepted := make(chan *ws.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		peer, err := upgrader.Upgrade(response, request, nil)
		if err != nil {
			t.Error(err)
			return
		}
		accepted <- peer
	}))
	t.Cleanup(server.Close)
	stream := &transportWriteConn{}
	dialer := &ws.Dialer{
		EnableCompression: opts.PerMessageDeflate() != nil,
		NetDialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			var dialer net.Dialer
			conn, err := dialer.DialContext(ctx, network, address)
			stream.Conn = conn
			return stream, err
		},
	}
	conn, _, err := dialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	peer := <-accepted
	t.Cleanup(func() { _ = peer.Close() })
	stream.headers = nil // Only record WebSocket frames after the HTTP handshake.
	w := MakeWebSocket().(*websocket)
	w.Transport.Construct(nil, opts)
	w.writeQueue = queue.New()
	t.Cleanup(w.writeQueue.Close)
	w.socket = &types.WebSocketConn{EventEmitter: types.NewEventEmitter(), Conn: conn}
	return w, stream, peer
}

type transportCopyBuffer struct {
	types.BufferInterface
	err   error
	calls int
}

func (b *transportCopyBuffer) WriteTo(writer io.Writer) (int64, error) {
	b.calls++
	if b.err != nil {
		return 0, b.err
	}
	return b.BufferInterface.WriteTo(writer)
}

func TestTransportWriteErrorPrecedence(t *testing.T) {
	for _, protocol := range []string{"websocket", "webtransport"} {
		for _, failure := range []string{"copy", "close", "copy and close"} {
			t.Run(protocol+"/"+failure, func(t *testing.T) {
				var copyErr, closeErr error
				if failure != "close" {
					copyErr = errors.New("copy failed")
				}
				if failure != "copy" {
					closeErr = errors.New("close failed")
				}
				var write func(types.BufferInterface) error
				var emitter types.EventEmitter
				if protocol == "websocket" {
					w, stream, _ := newWriteTestWebSocket(t, DefaultSocketOptions())
					stream.writeErr = closeErr
					write = func(data types.BufferInterface) error { return w.doWrite(data, false) }
					emitter = w.socket
				} else {
					w := newWriteTestWebTransport(t, &transportWriteStream{writeErr: closeErr})
					write = w.doWrite
					emitter = w.session
				}
				events := 0
				_ = emitter.On("error", func(...any) { events++ })
				body := &transportCopyBuffer{BufferInterface: types.NewBytesBufferString("payload"), err: copyErr}
				wantErr := copyErr
				if closeErr != nil {
					wantErr = closeErr
				}
				if got := write(body); got != wantErr {
					t.Fatalf("write = %v, want original %v", got, wantErr)
				}
				if body.calls != 1 || events != 0 {
					t.Fatalf("copy calls=%d error events=%d, want 1 and 0", body.calls, events)
				}
				if closeErr != nil {
					next := &transportCopyBuffer{BufferInterface: types.NewBytesBufferString("next")}
					if got := write(next); got != closeErr || next.calls != 0 {
						t.Fatalf("NextWriter failure = %v, copy calls=%d", got, next.calls)
					}
				}
			})
		}
	}
}

func TestTransportWriteErrorAllowsReentry(t *testing.T) {
	for _, protocol := range []string{"websocket", "webtransport"} {
		t.Run(protocol, func(t *testing.T) {
			var transport Transport
			var emitter types.EventEmitter
			var readMessage func() (int, []byte, error)
			if protocol == "websocket" {
				w, _, peer := newWriteTestWebSocket(t, DefaultSocketOptions())
				transport, emitter = w, w.socket
				readMessage = peer.ReadMessage
				_ = peer.SetReadDeadline(time.Now().Add(time.Second))
			} else {
				w := newWriteTestWebTransport(t, &transportWriteStream{})
				transport, emitter = w, w.session
				readMessage = w.session.ReadMessage
			}
			wantErr := errors.New("reader failed")
			reader := &pollingPayloadReader{data: "broken", err: wantErr}
			skipped := strings.NewReader("not sent")
			var events []string
			drained := make(chan struct{}, 2)
			_ = emitter.On("error", func(args ...any) {
				events = append(events, "error")
				if args[0] != wantErr {
					t.Errorf("error = %v, want original reader error", args[0])
				}
				transport.Write([]*packet.Packet{{Type: packet.MESSAGE, Data: types.NewStringBufferString("reentered")}})
			})
			_ = transport.On("drain", func(...any) {
				events = append(events, "drain")
				drained <- struct{}{}
			})
			transport.Write([]*packet.Packet{{Type: packet.MESSAGE, Data: reader}, {Type: packet.MESSAGE, Data: skipped}})
			for range 2 {
				select {
				case <-drained:
				case <-time.After(time.Second):
					t.Fatal("write queue did not finish the reentrant packet")
				}
			}
			if !reflect.DeepEqual(events, []string{"error", "drain", "drain"}) || reader.closed != 1 || skipped.Len() != len("not sent") {
				t.Fatalf("events=%v reader closes=%d skipped remaining=%d", events, reader.closed, skipped.Len())
			}
			messageType, data, err := readMessage()
			if err != nil || messageType != ws.TextMessage || string(data) != "4reentered" {
				t.Fatalf("reentrant packet = (%d, %q, %v)", messageType, data, err)
			}
		})
	}
}

func TestWebTransportConcurrentWritePreservesBatches(t *testing.T) {
	w := newWriteTestWebTransport(t, &transportWriteStream{})
	_ = w.session.On("error", func(args ...any) { t.Errorf("write error: %v", args) })
	const producers, batches = 8, 16
	var group sync.WaitGroup
	for producer := range producers {
		group.Go(func() {
			for batch := range batches {
				id := fmt.Sprintf("%d:%d", producer, batch)
				w.Write([]*packet.Packet{
					{Type: packet.MESSAGE, Data: types.NewStringBufferString(id + "/first")},
					{Type: packet.MESSAGE, Data: types.NewStringBufferString(id + "/second")},
				})
			}
		})
	}
	group.Wait()
	w.writeQueue.Close()

	// The stream is only read after all producers and the write worker finish.
	seen := make(map[string]bool, producers*batches)
	for range producers * batches {
		messageType, first, err := w.session.ReadMessage()
		if err != nil || messageType != wire.TextMessage {
			t.Fatalf("first packet = (%d, %q, %v)", messageType, first, err)
		}
		id, ok := strings.CutSuffix(string(first), "/first")
		if !ok || seen[id] {
			t.Fatalf("missing or repeated batch start: %q", first)
		}
		seen[id] = true
		messageType, second, err := w.session.ReadMessage()
		if err != nil || messageType != wire.TextMessage || string(second) != id+"/second" {
			t.Fatalf("interleaved batch: first=%q second=(%d, %q, %v)", first, messageType, second, err)
		}
	}
	if _, extra, err := w.session.ReadMessage(); err == nil {
		t.Fatalf("unexpected extra packet: %q", extra)
	}
}

func TestWebSocketWriteCompressionThreshold(t *testing.T) {
	for _, test := range []struct {
		name      string
		threshold int
		compress  bool
		want      bool
	}{
		{"enabled", 0, true, true},
		{"packet disabled", 0, false, false},
		{"below threshold", 100, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			opts := DefaultSocketOptions()
			opts.SetPerMessageDeflate(&types.PerMessageDeflate{Threshold: test.threshold})
			w, stream, peer := newWriteTestWebSocket(t, opts)
			if err := w.doWrite(types.NewStringBufferString("4payload"), test.compress); err != nil {
				t.Fatal(err)
			}
			if len(stream.headers) != 1 || (stream.headers[0]&0x40 != 0) != test.want {
				t.Fatalf("frame headers=%x, want compressed=%t", stream.headers, test.want)
			}
			_ = peer.SetReadDeadline(time.Now().Add(time.Second))
			messageType, data, err := peer.ReadMessage()
			if err != nil || messageType != ws.TextMessage || string(data) != "4payload" {
				t.Fatalf("received = (%d, %q, %v)", messageType, data, err)
			}
		})
	}
}
