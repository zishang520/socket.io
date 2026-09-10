package socket

import (
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/zishang520/socket.io/clients/engine/v3"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type parserRecordingEngine struct {
	Engine
	writes []string
}

func (e *parserRecordingEngine) Write(data io.Reader, _ *packet.Options, _ func()) engine.SocketWithoutUpgrade {
	raw, _ := io.ReadAll(data)
	e.writes = append(e.writes, string(raw))
	return e
}

func TestManagerEncodingFailureAndConnectWithoutAuth(t *testing.T) {
	m := MakeManager()
	t.Cleanup(m.taskQueue.Close)
	e := &parserRecordingEngine{}
	m.engine.Store(new(Engine(e)))
	m.encoder = parser.NewEncoder()
	s := MakeSocket()
	s.io = m
	s.nsp = "/"
	err := m._packet(&Packet{Packet: &parser.Packet{Type: parser.EVENT, Nsp: s.nsp, Data: []any{"x", []byte{1}, make(chan int)}}})
	if err == nil || len(e.writes) != 0 {
		t.Fatalf("err=%v writes=%v", err, e.writes)
	}
	s._sendConnectPacket(nil)
	if len(e.writes) != 1 || e.writes[0] != "0" {
		t.Fatalf("CONNECT=%v", e.writes)
	}
	if err := parser.NewDecoder().Add(e.writes[0]); err != nil {
		t.Fatal(err)
	}
}

func TestEncodingFailureStaysInNamespace(t *testing.T) {
	for _, packetType := range []parser.PacketType{parser.EVENT, parser.CONNECT} {
		t.Run(fmt.Sprint(packetType), func(t *testing.T) {
			m := MakeManager()
			t.Cleanup(m.taskQueue.Close)
			e := &parserRecordingEngine{}
			m.engine.Store(new(Engine(e)))
			m.encoder = parser.NewEncoder()
			s := MakeSocket()
			s.io = m
			s.nsp = "/"
			s._opts = DefaultSocketOptions()
			s.connected.Store(packetType == parser.EVENT)
			m.nsps.Store(s.nsp, s)
			s.subEvents()
			other := MakeSocket()
			other.io = m
			other.nsp = "/other"
			m.nsps.Store(other.nsp, other)
			other.subEvents()
			_ = other.On("connect_error", func(...any) { t.Error("encoding error reached another namespace") })
			_ = m.On("error", func(...any) { t.Error("encoding error reported as a connection-wide error") })
			event := types.EventName("error")
			if packetType == parser.CONNECT {
				event = "connect_error"
			}
			errors := 0
			_ = s.On(event, func(...any) { errors++ })
			if packetType == parser.CONNECT {
				s._sendConnectPacket(map[string]any{"invalid": make(chan int)})
			} else if err := s.Emit("x", make(chan int)); err != nil {
				t.Fatal(err)
			}
			if errors != 1 || len(e.writes) != 0 {
				t.Fatalf("errors=%d writes=%v", errors, e.writes)
			}
		})
	}
}

func TestNumericEventDispatch(t *testing.T) {
	s := MakeSocket()
	got := false
	_ = s.On("123", func(...any) { got = true })
	d := parser.NewDecoder()
	_ = d.On("decoded", func(args ...any) { s.emitEvent(args[0].(*parser.Packet).Data.([]any)) })
	if err := d.Add(`2[123,"payload"]`); err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("numeric event was not dispatched to 123")
	}
}

func TestConstructReportsConnectEncodingFailure(t *testing.T) {
	m := MakeManager()
	t.Cleanup(m.taskQueue.Close)
	e := &parserRecordingEngine{}
	m.engine.Store(new(Engine(e)))
	m.encoder = parser.NewEncoder()
	m._autoConnect = true
	m._readyState.Store(ReadyStateOpen)
	opts := DefaultSocketOptions()
	opts.SetAuth(map[string]any{"invalid": make(chan int)})
	s := MakeSocket()
	errors := 0
	_ = s.On("connect_error", func(...any) { errors++ })
	// Construct can send CONNECT before Manager.Socket registers the namespace.
	s.Construct(m, "/direct", opts)
	if errors != 1 || len(e.writes) != 0 {
		t.Fatalf("errors=%d writes=%v", errors, e.writes)
	}
}

func (e *parserRecordingEngine) Transport() engine.Transport { return nil }
func (e *parserRecordingEngine) HasPingExpired() bool        { return false }
func TestEmitEncodingFailureCompletesAck(t *testing.T) {
	for _, buffered := range []bool{false, true} {
		for _, timed := range []bool{false, true} {
			t.Run(fmt.Sprintf("buffered=%v/timed=%v", buffered, timed), func(t *testing.T) {
				m := MakeManager()
				t.Cleanup(m.taskQueue.Close)
				e := &parserRecordingEngine{}
				m.engine.Store(new(Engine(e)))
				m.encoder = parser.NewEncoder()
				s := NewSocket(m, "/", nil)
				s.connected.Store(!buffered)
				errors := 0
				_ = s.On("error", func(...any) { errors++ })
				other := MakeSocket()
				other.acks.Store(0, func([]any, error) { t.Error("unrelated ACK completed") })
				m.nsps.Store("/other", other)
				got := make(chan error, 2)
				if timed {
					s.Timeout(100 * time.Millisecond)
				}
				if err := s.Emit("x", make(chan int), func(_ []any, err error) {
					// Re-enter Emit while flushing: the send buffer must not be locked.
					s.connected.Store(false)
					_ = s.Emit("next", "valid")
					got <- err
				}); err != nil {
					t.Fatal(err)
				}
				if buffered {
					s.connected.Store(true)
					s.emitBuffered()
				}
				select {
				case err := <-got:
					if err == nil {
						t.Fatal("missing encoding error")
					}
				default:
					t.Fatal("ACK was not completed synchronously")
				}
				if s.acks.Len() != 0 || other.acks.Len() != 1 || len(e.writes) != 0 || s.sendBuffer.Len() != 1 || errors != 1 {
					t.Fatalf("pending=%d unrelated=%d writes=%d buffered=%d errors=%d", s.acks.Len(), other.acks.Len(), len(e.writes), s.sendBuffer.Len(), errors)
				}
				if timed {
					select {
					case <-got:
						t.Fatal("duplicate ACK after timeout")
					case <-time.After(150 * time.Millisecond):
					}
				}
			})
		}
	}
}
