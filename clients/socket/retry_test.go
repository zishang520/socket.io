package socket

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zishang520/socket.io/clients/engine/v3"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type retryWrite struct {
	wire     string
	compress *bool
}

type retryRecordingEngine struct {
	parserRecordingEngine
	written chan retryWrite
}

func (e *retryRecordingEngine) Write(data io.Reader, options *packet.Options, _ func()) engine.SocketWithoutUpgrade {
	raw, _ := io.ReadAll(data)
	e.written <- retryWrite{string(raw), options.Compress}
	return e
}

func TestRetryQueueSendsInOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := MakeManager()
		t.Cleanup(m.taskQueue.Close)
		e := &retryRecordingEngine{written: make(chan retryWrite, 4)}
		m.engine.Store(new(Engine(e)))
		m.encoder = parser.NewEncoder()
		opts := DefaultSocketOptions()
		opts.SetRetries(1)
		s := NewSocket(m, "/", opts)
		s.connected.Store(true)
		var responses []string
		ack := func(args []any, err error) {
			if err != nil {
				t.Errorf("unexpected ACK error: %v", err)
				return
			}
			responses = append(responses, args[0].(string))
		}
		if err := s.Timeout(time.Second).Compress(true).Emit("first", ack); err != nil {
			t.Fatal(err)
		}
		if err := s.Compress(false).Emit("second", ack); err != nil {
			t.Fatal(err)
		}
		if len(e.written) != 1 || s._queue.Len() != 2 || s.acks.Len() != 1 {
			t.Fatalf("writes=%d queued=%d acks=%d", len(e.written), s._queue.Len(), s.acks.Len())
		}
		first := <-e.written
		if first.wire != `20["first"]` || first.compress == nil || !*first.compress {
			t.Fatalf("first send=%+v", first)
		}
		s.onack(&parser.Packet{Id: new(uint64(0)), Data: []any{"one"}})
		if len(e.written) != 1 || s._queue.Len() != 1 {
			t.Fatalf("writes=%d queued=%d", len(e.written), s._queue.Len())
		}
		second := <-e.written
		if second.wire != `21["second"]` || second.compress == nil || *second.compress {
			t.Fatalf("second send=%+v", second)
		}
		s.onack(&parser.Packet{Id: new(uint64(1)), Data: []any{"two"}})
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if !reflect.DeepEqual(responses, []string{"one", "two"}) || s._queue.Len() != 0 || s.acks.Len() != 0 || len(e.written) != 0 {
			t.Fatalf("responses=%v queued=%d acks=%d extra writes=%d", responses, s._queue.Len(), s.acks.Len(), len(e.written))
		}
	})
}

func TestRetryQueueTimeoutAndLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := MakeManager()
		t.Cleanup(m.taskQueue.Close)
		e := &retryRecordingEngine{written: make(chan retryWrite, 4)}
		m.engine.Store(new(Engine(e)))
		m.encoder = parser.NewEncoder()
		opts := DefaultSocketOptions()
		opts.SetRetries(1)
		s := NewSocket(m, "/", opts)
		s.connected.Store(true)
		completed := make(chan error, 2)
		if err := s.Timeout(time.Second).Emit("retry", func(_ []any, err error) {
			completed <- err
		}); err != nil {
			t.Fatal(err)
		}
		if len(e.written) != 1 {
			t.Fatalf("first send count=%d", len(e.written))
		}
		if first := <-e.written; first.wire != `20["retry"]` {
			t.Fatalf("first send=%+v", first)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if len(e.written) != 1 || len(completed) != 0 {
			t.Fatalf("retry writes=%d callbacks=%d", len(e.written), len(completed))
		}
		if retry := <-e.written; retry.wire != `21["retry"]` {
			t.Fatalf("retry send=%+v", retry)
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if len(completed) != 1 || s._queue.Len() != 0 || s.acks.Len() != 0 {
			t.Fatalf("callbacks=%d queued=%d acks=%d", len(completed), s._queue.Len(), s.acks.Len())
		}
		if err := <-completed; err == nil {
			t.Fatal("missing timeout error")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if len(e.written) != 0 || len(completed) != 0 {
			t.Fatalf("retry limit exceeded: extra writes=%d callbacks=%d", len(e.written), len(completed))
		}
	})
}

func TestRetryQueueEncodingFailureCompletesAck(t *testing.T) {
	m := MakeManager()
	t.Cleanup(m.taskQueue.Close)
	e := &parserRecordingEngine{}
	m.engine.Store(new(Engine(e)))
	m.encoder = parser.NewEncoder()
	opts := DefaultSocketOptions()
	opts.SetRetries(1)
	s := NewSocket(m, "/", opts)
	s.connected.Store(true)
	callbacks := 0
	if err := s.Emit("invalid", make(chan int), func(_ []any, err error) {
		callbacks++
		if err == nil {
			t.Error("missing encoding error")
		}
	}); err != nil {
		t.Fatal(err)
	}
	if callbacks != 1 || s._queue.Len() != 0 || s.acks.Len() != 0 || len(e.writes) != 0 {
		t.Fatalf("callbacks=%d queued=%d acks=%d writes=%v", callbacks, s._queue.Len(), s.acks.Len(), e.writes)
	}
	if err := s.Emit("valid"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(e.writes, []string{`22["valid"]`}) {
		t.Fatalf("queue did not resume: %v", e.writes)
	}
	s.onack(&parser.Packet{Id: new(uint64(2)), Data: []any{}})
	if s._queue.Len() != 0 || s.acks.Len() != 0 {
		t.Fatal("successful packet left pending queue entries")
	}
}

func TestRetryQueuePreservesReaders(t *testing.T) {
	for _, tc := range []struct {
		name string
		data func() any
		want []string
	}{
		{"text", func() any { return strings.NewReader("payload") }, []string{`20["upload","payload"]`}},
		{"text buffer", func() any { return types.NewStringBufferString("payload") }, []string{`20["upload","payload"]`}},
		{"binary", func() any { return bytes.NewBufferString("payload") }, []string{`51-0["upload",{"_placeholder":true,"num":0}]`, "payload"}},
		{"nested", func() any {
			return []any{strings.NewReader("text"), map[string]any{"file": bytes.NewBufferString("payload")}, []any(nil), map[string]any(nil)}
		}, []string{`51-0["upload",["text",{"file":{"_placeholder":true,"num":0}},null,null]]`, "payload"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := MakeManager()
				t.Cleanup(m.taskQueue.Close)
				e := &retryRecordingEngine{written: make(chan retryWrite, 4)}
				m.engine.Store(new(Engine(e)))
				m.encoder = parser.NewEncoder()
				opts := DefaultSocketOptions()
				opts.SetRetries(1)
				s := NewSocket(m, "/", opts)
				s.connected.Store(true)
				if err := s.Timeout(time.Second).Emit("upload", tc.data()); err != nil {
					t.Fatal(err)
				}
				var first []string
				for len(e.written) > 0 {
					first = append(first, (<-e.written).wire)
				}
				if !reflect.DeepEqual(first, tc.want) {
					t.Fatalf("first send=%q want=%q", first, tc.want)
				}
				time.Sleep(time.Second)
				synctest.Wait()
				var retry []string
				for len(e.written) > 0 {
					retry = append(retry, (<-e.written).wire)
				}
				want := append([]string(nil), tc.want...)
				want[0] = strings.Replace(want[0], "0[", "1[", 1)
				if !reflect.DeepEqual(retry, want) {
					t.Errorf("retry send=%q want=%q", retry, want)
				}
				s.onack(&parser.Packet{Id: new(uint64(1)), Data: []any{}})
			})
		})
	}
}

type retryFailingReader struct {
	err    error
	reads  int
	closes int
}

func (r *retryFailingReader) Read(p []byte) (int, error) {
	r.reads++
	return copy(p, "partial"), r.err
}

func (r *retryFailingReader) Close() error {
	r.closes++
	return nil
}

func TestRetryQueueDoesNotRetryReaderFailure(t *testing.T) {
	m := MakeManager()
	t.Cleanup(m.taskQueue.Close)
	e := &parserRecordingEngine{}
	m.engine.Store(new(Engine(e)))
	m.encoder = parser.NewEncoder()
	opts := DefaultSocketOptions()
	opts.SetRetries(1)
	s := NewSocket(m, "/", opts)
	s.connected.Store(true)
	r := &retryFailingReader{err: errors.New("read failed")}
	callbacks, reported := 0, 0
	_ = s.On("error", func(args ...any) {
		reported++
		if !errors.Is(args[0].(error), r.err) {
			t.Errorf("reported error=%v", args[0])
		}
	})
	if err := s.Emit("upload", map[string]any{"file": r}, func(_ []any, err error) {
		callbacks++
		if !errors.Is(err, r.err) {
			t.Errorf("ACK error=%v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	if callbacks != 1 || reported != 1 || r.reads != 1 || r.closes != 1 || len(e.writes) != 0 || s._queue.Len() != 0 || s.acks.Len() != 0 {
		t.Fatalf("callbacks=%d errors=%d reads=%d closes=%d writes=%v queued=%d acks=%d", callbacks, reported, r.reads, r.closes, e.writes, s._queue.Len(), s.acks.Len())
	}
}
