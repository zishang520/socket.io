package webtransport

import (
	"bytes"
	"fmt"
	"sync"
	"testing"
)

func TestPreparedMessageWireCompatibility(t *testing.T) {
	for _, isServer := range []bool{true, false} {
		for _, size := range []int{0, 1, 125, 126, 127, 4095, 4096, 4097, 8192, 65535, 65536, 70000} {
			for _, messageType := range []int{TextMessage, BinaryMessage} {
				t.Run(fmt.Sprintf("server=%t/type=%d/size=%d", isServer, messageType, size), func(t *testing.T) {
					data := bytes.Repeat([]byte("x"), size)
					plain, plainStream := newTestConn(isServer)
					if err := plain.WriteMessage(messageType, data); err != nil {
						t.Fatal(err)
					}
					pm, err := NewPreparedMessage(messageType, data)
					if err != nil {
						t.Fatal(err)
					}
					// Prepared messages must own their payload for both endpoints.
					clear(data)
					prepared, preparedStream := newTestConn(isServer)
					for range 2 {
						preparedStream.buf.Reset()
						if err := prepared.WritePreparedMessage(pm); err != nil {
							t.Fatal(err)
						}
						if !bytes.Equal(preparedStream.buf.Bytes(), plainStream.buf.Bytes()) {
							t.Fatal("prepared and ordinary messages have different wire bytes")
						}
					}
				})
			}
		}
	}
}

func TestPreparedMessageConcurrentReuse(t *testing.T) {
	data := bytes.Repeat([]byte("payload"), 10000)
	pm, err := NewPreparedMessage(TextMessage, data)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for range 8 {
		for _, isServer := range []bool{true, false} {
			group.Go(func() {
				plain, plainStream := newTestConn(isServer)
				if err := plain.WriteMessage(TextMessage, data); err != nil {
					t.Error(err)
					return
				}
				conn, stream := newTestConn(isServer)
				if err := conn.WritePreparedMessage(pm); err != nil {
					t.Error(err)
					return
				}
				if !bytes.Equal(stream.buf.Bytes(), plainStream.buf.Bytes()) {
					t.Error("shared prepared message changed its wire representation")
				}
			})
		}
	}
	group.Wait()
}

func TestWriteMessageFrameHeaders(t *testing.T) {
	for _, tt := range []struct {
		size   int
		header []byte
	}{
		{0, []byte{0}},
		{125, []byte{125}},
		{126, []byte{126, 0, 126}},
		{65535, []byte{126, 255, 255}},
		{65536, []byte{127, 0, 0, 0, 0, 0, 1, 0, 0}},
	} {
		for _, messageType := range []int{TextMessage, BinaryMessage} {
			t.Run(fmt.Sprintf("type=%d/size=%d", messageType, tt.size), func(t *testing.T) {
				conn, stream := newTestConn(true)
				payload := bytes.Repeat([]byte("x"), tt.size)
				if err := conn.WriteMessage(messageType, payload); err != nil {
					t.Fatal(err)
				}
				header := bytes.Clone(tt.header)
				if messageType == BinaryMessage {
					header[0] |= 0x80
				}
				if !bytes.Equal(stream.buf.Bytes(), append(header, payload...)) {
					t.Fatal("unexpected frame encoding")
				}
			})
		}
	}
}

func BenchmarkPreparedMessage(b *testing.B) {
	for _, size := range []int{128, 4096, 65536} {
		b.Run(fmt.Sprintf("construct/%d", size), func(b *testing.B) {
			data := make([]byte, size)
			b.ReportAllocs()
			for b.Loop() {
				if _, err := NewPreparedMessage(BinaryMessage, data); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprintf("reuse/%d", size), func(b *testing.B) {
			pm, err := NewPreparedMessage(BinaryMessage, make([]byte, size))
			if err != nil {
				b.Fatal(err)
			}
			conn, stream := newTestConn(true)
			b.ReportAllocs()
			for b.Loop() {
				stream.buf.Reset()
				if err := conn.WritePreparedMessage(pm); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
