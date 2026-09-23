package webtransport

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestMessageWritersPreservePacketBoundaries(t *testing.T) {
	writers := []struct {
		name  string
		write func(*Conn, int, []byte) error
	}{
		{"WriteMessage", (*Conn).WriteMessage},
		{"PreparedMessage", func(conn *Conn, messageType int, data []byte) error {
			prepared, err := NewPreparedMessage(messageType, data)
			if err != nil {
				return err
			}
			return conn.WritePreparedMessage(prepared)
		}},
		{"Write", func(conn *Conn, messageType int, data []byte) error {
			writer, err := conn.NextWriter(messageType)
			if err != nil {
				return err
			}
			if _, err = writer.Write(data); err != nil {
				return err
			}
			return writer.Close()
		}},
		{"WriteString", func(conn *Conn, messageType int, data []byte) error {
			writer, err := conn.NextWriter(messageType)
			if err != nil {
				return err
			}
			if _, err = writer.(io.StringWriter).WriteString(string(data)); err != nil {
				return err
			}
			return writer.Close()
		}},
		{"ReadFrom", func(conn *Conn, messageType int, data []byte) error {
			writer, err := conn.NextWriter(messageType)
			if err != nil {
				return err
			}
			if _, err = writer.(io.ReaderFrom).ReadFrom(bytes.NewReader(data)); err != nil {
				return err
			}
			return writer.Close()
		}},
	}
	for _, isServer := range []bool{true, false} {
		for _, writer := range writers {
			for _, size := range []int{0, 125, 126, 4095, 4096, 4097, 4900, 9000, 65535, 65536} {
				for _, messageType := range []int{TextMessage, BinaryMessage} {
					t.Run(fmt.Sprintf("server=%t/%s/type=%d/size=%d", isServer, writer.name, messageType, size), func(t *testing.T) {
						conn, _ := newTestConn(isServer)
						payload := bytes.Repeat([]byte("x"), size)
						if err := writer.write(conn, messageType, payload); err != nil {
							t.Fatal(err)
						}
						if err := conn.WriteMessage(BinaryMessage, []byte("next packet")); err != nil {
							t.Fatal(err)
						}
						gotType, got, err := conn.ReadMessage()
						if err != nil || gotType != messageType || !bytes.Equal(got, payload) {
							t.Fatalf("first packet = (type %d, %d bytes, %v), want (type %d, %d bytes)", gotType, len(got), err, messageType, size)
						}
						gotType, got, err = conn.ReadMessage()
						if err != nil || gotType != BinaryMessage || string(got) != "next packet" {
							t.Fatalf("next packet = (type %d, %q, %v)", gotType, got, err)
						}
					})
				}
			}
		}
	}
}

func TestNextWriterUnicodeChunks(t *testing.T) {
	conn, stream := newTestConn(true)
	writer, err := conn.NextWriter(TextMessage)
	if err != nil {
		t.Fatal(err)
	}
	payload := strings.Repeat("4你好🙂", 1000)
	for i := range len(payload) {
		if _, err = writer.Write([]byte{payload[i]}); err != nil {
			t.Fatal(err)
		}
	}
	if stream.buf.Len() != 0 {
		t.Fatal("writer emitted an incomplete message before its final length was known")
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	messageType, data, err := conn.ReadMessage()
	if err != nil || messageType != TextMessage || string(data) != payload {
		t.Fatalf("Unicode packet = (type %d, %d bytes, %v)", messageType, len(data), err)
	}
}

type messageBufferPool struct {
	value      any
	gets, puts int
}

func (p *messageBufferPool) Get() any {
	p.gets++
	value := p.value
	p.value = nil
	return value
}

func (p *messageBufferPool) Put(value any) {
	p.puts++
	p.value = value
}

func TestMessageWriterBufferPool(t *testing.T) {
	for _, suppliedBuffer := range []bool{false, true} {
		t.Run(fmt.Sprintf("supplied=%t", suppliedBuffer), func(t *testing.T) {
			pool := &messageBufferPool{value: "unrelated pool value"}
			stream := &prepareConn{}
			var buffer []byte
			if suppliedBuffer {
				buffer = make([]byte, 16+maxFrameHeaderSize)
			}
			conn := NewConn(nil, stream, false, 0, 16, pool, nil, buffer)
			writer, err := conn.NextWriter(BinaryMessage)
			if err != nil {
				t.Fatal(err)
			}
			payload := bytes.Repeat([]byte{1, 2, 3}, 3000)
			if _, err = writer.Write(payload); err != nil {
				t.Fatal(err)
			}
			if pool.puts != 0 {
				t.Fatal("returned a buffer before its message was closed")
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err = writer.Close(); err != errWriteClosed {
				t.Fatalf("repeated Close() = %v", err)
			}
			if pool.puts != 1 || pool.value == nil {
				t.Fatalf("pool after Close: puts=%d value=%T", pool.puts, pool.value)
			}
			// Growth belongs to this message; retaining its writer must not pin
			// an arbitrarily large payload in the reusable pool.
			if len(pool.value.(writePoolData).buf) != 16+maxFrameHeaderSize {
				t.Fatal("message growth replaced the configured pool buffer")
			}
			if err = conn.WriteMessage(TextMessage, []byte("pooled")); err != nil {
				t.Fatal(err)
			}
			wantGets := 2
			if suppliedBuffer {
				wantGets = 1
			}
			if pool.gets != wantGets || pool.puts != 2 {
				t.Fatalf("pool calls = (%d gets, %d puts), want (%d, 2)", pool.gets, pool.puts, wantGets)
			}
			messageType, data, err := conn.ReadMessage()
			if err != nil || messageType != BinaryMessage || !bytes.Equal(data, payload) {
				t.Fatalf("buffered message = (%d, %d bytes, %v)", messageType, len(data), err)
			}
			messageType, data, err = conn.ReadMessage()
			if err != nil || messageType != TextMessage || string(data) != "pooled" {
				t.Fatalf("reused buffer message = (%d, %q, %v)", messageType, data, err)
			}
		})
	}
}

type failingMessageStream struct {
	prepareConn
	err       error
	failWrite int
	writes    int
	deadlines int
	deadline  time.Time
}

func (s *failingMessageStream) Write(p []byte) (int, error) {
	s.writes++
	if s.writes == s.failWrite {
		return 0, s.err
	}
	return s.prepareConn.Write(p)
}

func (s *failingMessageStream) SetWriteDeadline(deadline time.Time) error {
	s.deadlines++
	s.deadline = deadline
	if s.failWrite == 0 {
		return s.err
	}
	return nil
}

func TestMessageWriteFailureIsPermanent(t *testing.T) {
	for _, mode := range []string{"message", "writer", "prepared"} {
		for _, failWrite := range []int{0, 1, 2} {
			if failWrite == 2 && mode != "message" {
				continue // Buffered and prepared messages are single stream writes.
			}
			t.Run(fmt.Sprintf("%s/failWrite=%d", mode, failWrite), func(t *testing.T) {
				wantErr := errors.New("stream failed")
				stream := &failingMessageStream{err: wantErr, failWrite: failWrite}
				pool := &messageBufferPool{}
				conn := NewConn(nil, stream, true, 0, 0, pool, nil, nil)
				deadline := time.Now().Add(time.Minute)
				if err := conn.SetWriteDeadline(deadline); err != nil {
					t.Fatal(err)
				}
				data := make([]byte, 9000)
				prepared, err := NewPreparedMessage(BinaryMessage, data)
				if err != nil {
					t.Fatal(err)
				}
				var writer io.WriteCloser
				switch mode {
				case "message":
					err = conn.WriteMessage(BinaryMessage, data)
				case "prepared":
					err = conn.WritePreparedMessage(prepared)
				case "writer":
					writer, err = conn.NextWriter(BinaryMessage)
					if err != nil {
						t.Fatal(err)
					}
					if _, err = writer.Write(data); err != nil {
						t.Fatal(err)
					}
					err = writer.Close()
				}
				if err != wantErr {
					t.Fatalf("write = %v, want original stream error", err)
				}
				if stream.deadline != deadline {
					t.Fatal("write deadline was not applied")
				}
				if writer != nil {
					if err = writer.Close(); err != wantErr {
						t.Fatalf("repeated failed Close() = %v", err)
					}
				}
				if err = conn.WriteMessage(TextMessage, []byte("later")); err != wantErr {
					t.Fatalf("later WriteMessage = %v", err)
				}
				if _, err = conn.NextWriter(TextMessage); err != wantErr {
					t.Fatalf("later NextWriter = %v", err)
				}
				if err = conn.WritePreparedMessage(prepared); err != wantErr {
					t.Fatalf("later WritePreparedMessage = %v", err)
				}
				if stream.deadlines != 1 || stream.writes != failWrite {
					t.Fatalf("stream retried after failure: deadlines=%d writes=%d", stream.deadlines, stream.writes)
				}
				wantPoolCalls := 1
				if mode == "prepared" {
					wantPoolCalls = 0
				}
				if pool.gets != wantPoolCalls || pool.puts != wantPoolCalls {
					t.Fatalf("pool calls after failure: gets=%d puts=%d", pool.gets, pool.puts)
				}
			})
		}
	}
}

type messageDataErrorReader struct {
	data []byte
	err  error
}

func (r *messageDataErrorReader) Read(p []byte) (int, error) {
	n := copy(p, r.data)
	r.data = r.data[n:]
	if len(r.data) == 0 {
		return n, r.err
	}
	return n, nil
}

func TestMessageWriterReadFromSourceError(t *testing.T) {
	conn, _ := newTestConn(true)
	writer, err := conn.NextWriter(TextMessage)
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("source failed")
	payload := bytes.Repeat([]byte("x"), 4900)
	n, err := writer.(io.ReaderFrom).ReadFrom(&messageDataErrorReader{data: payload, err: wantErr})
	if n != int64(len(payload)) || err != wantErr {
		t.Fatalf("ReadFrom = (%d, %v)", n, err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	messageType, got, err := conn.ReadMessage()
	if err != nil || messageType != TextMessage || !bytes.Equal(got, payload) {
		t.Fatalf("partial source message = (%d, %d bytes, %v)", messageType, len(got), err)
	}
}

func BenchmarkWriteMessage(b *testing.B) {
	for _, size := range []int{128, 4096, 65536} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			conn, stream := newTestConn(true)
			data := make([]byte, size)
			b.ReportAllocs()
			for b.Loop() {
				stream.buf.Reset()
				if err := conn.WriteMessage(BinaryMessage, data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkNextWriter(b *testing.B) {
	for _, size := range []int{128, 4096, 65536} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			conn, stream := newTestConn(true)
			data := make([]byte, size)
			b.ReportAllocs()
			for b.Loop() {
				stream.buf.Reset()
				writer, err := conn.NextWriter(BinaryMessage)
				if err != nil {
					b.Fatal(err)
				}
				if _, err = writer.Write(data); err != nil {
					b.Fatal(err)
				}
				if err = writer.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
