package socket

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type parserRecordingConn struct {
	engine.Socket
	writes int
	closed bool
}

func (c *parserRecordingConn) ReadyState() string { return "open" }
func (c *parserRecordingConn) Close(bool)         { c.closed = true }
func (c *parserRecordingConn) Write(io.Reader, *packet.Options, engine.SendCallback) engine.Socket {
	c.writes++
	return c
}

func TestClientEncodingFailureDoesNotWrite(t *testing.T) {
	c := MakeClient()
	conn := &parserRecordingConn{}
	c.conn = conn
	c.encoder = parser.NewEncoder()
	s := MakeSocket()
	t.Cleanup(s.taskQueue.Close)
	c.sockets.Store("s", s)
	c.nsps.Store("/", s)
	errors := 0
	_ = s.On("error", func(...any) { errors++ })
	c._packet(&parser.Packet{Type: parser.EVENT, Nsp: "/", Data: []any{"x", []byte{1}, make(chan int)}}, nil)
	if errors != 1 || conn.writes != 0 || conn.closed {
		t.Fatalf("errors=%d writes=%d closed=%v", errors, conn.writes, conn.closed)
	}
	c._packet(&parser.Packet{Type: parser.EVENT, Data: []any{"next", "valid"}}, nil)
	if conn.writes != 1 || conn.closed {
		t.Fatal("encoding failure prevented a subsequent valid packet")
	}
}

func TestBroadcastEncodingFailure(t *testing.T) {
	a := newTestAdapter()
	errors := 0
	_ = a.On("error", func(...any) { errors++ })
	a.Broadcast(&parser.Packet{Type: parser.EVENT, Data: []any{"x", make(chan int)}}, &BroadcastOptions{})
	if errors != 1 {
		t.Fatalf("errors=%d", errors)
	}
	got := make(chan error, 2)
	operator := NewBroadcastOperator(a, nil, nil, nil).Timeout(time.Second)
	if err := operator.Emit("x", make(chan int), func(_ []any, err error) { got <- err }); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-got:
		if err == nil {
			t.Fatal("encoding error reported as successful ACK")
		}
	case <-time.After(time.Second):
		t.Fatal("missing encoding error")
	}
	select {
	case <-got:
		t.Fatal("duplicate ACK")
	default:
	}
}

func TestNumericEventDispatch(t *testing.T) {
	s := MakeSocket()
	t.Cleanup(s.taskQueue.Close)
	s.connected.Store(true)
	got := make(chan struct{}, 1)
	_ = s.On("123", func(...any) { got <- struct{}{} })
	d := parser.NewDecoder()
	_ = d.On("decoded", func(args ...any) { s.dispatch(args[0].(*parser.Packet).Data.([]any)) })
	if err := d.Add(`2[123,"payload"]`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-got:
	case <-time.After(time.Second):
		t.Fatal("numeric event was not dispatched to 123")
	}
}

func TestEmitEncodingFailureCompletesAck(t *testing.T) {
	for _, timed := range []bool{false, true} {
		t.Run(fmt.Sprint(timed), func(t *testing.T) {
			c := MakeClient()
			conn := &parserRecordingConn{}
			c.conn = conn
			c.encoder = parser.NewEncoder()
			s := MakeSocket()
			t.Cleanup(s.taskQueue.Close)
			s.nsp = NewServer(nil, nil).Sockets()
			s.client = c
			s.connected.Store(true)
			c.sockets.Store("s", s)
			c.nsps.Store("/", s)
			other := MakeSocket()
			t.Cleanup(other.taskQueue.Close)
			other.acks.Store(0, func([]any, error) { t.Error("unrelated ACK completed") })
			c.nsps.Store("/other", other)
			c.sockets.Store("other", other)
			_ = other.On("error", func(...any) { t.Error("encoding error reached another namespace") })
			errors := 0
			_ = s.On("error", func(...any) { errors++ })
			got := make(chan error, 2)
			if timed {
				s.Timeout(20 * time.Millisecond)
			}
			if err := s.Emit("x", make(chan int), func(_ []any, err error) { got <- err }); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-got:
				if err == nil {
					t.Fatal("missing encoding error")
				}
			default:
				t.Fatal("ACK was not completed synchronously")
			}
			if s.acks.Len() != 0 || other.acks.Len() != 1 || conn.writes != 0 || errors != 1 {
				t.Fatalf("pending=%d unrelated=%d writes=%d errors=%d", s.acks.Len(), other.acks.Len(), conn.writes, errors)
			}
			if timed {
				select {
				case <-got:
					t.Fatal("duplicate ACK after timeout")
				case <-time.After(40 * time.Millisecond):
				}
			}
		})
	}
}

func TestRecoveryEmitEncodingFailureCompletesAck(t *testing.T) {
	opts := DefaultServerOptions()
	opts.SetConnectionStateRecovery(DefaultConnectionStateRecovery())
	nsp := NewServer(nil, opts).Sockets()
	t.Cleanup(nsp.Adapter().Close)
	s := MakeSocket()
	t.Cleanup(s.taskQueue.Close)
	s.nsp = nsp
	s.adapter = nsp.Adapter()
	s.id = "s"
	s.connected.Store(true)
	nsp.Sockets().Store(s.id, s)
	s.adapter.AddAll(s.id, types.NewSet(Room(s.id)))
	errors := 0
	_ = s.adapter.On("error", func(...any) {
		errors++
		// Error listeners may synchronously change the broadcast's target rooms.
		s.Leave(Room(s.id))
	})
	called := false
	if err := s.Emit("x", make(chan int), func(_ []any, err error) {
		called = true
		if err == nil {
			t.Error("missing encoding error")
		}
	}); err != nil {
		t.Fatal(err)
	}
	if !called || s.acks.Len() != 0 || errors != 1 {
		t.Fatalf("callback=%v pending=%d errors=%d", called, s.acks.Len(), errors)
	}
}
