package engine

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
	webtrans "github.com/zishang520/socket.io/v3/pkg/webtransport"
)

func TestWebTransportPreservesLargeMessageBoundaries(t *testing.T) {
	opts := config.DefaultServerOptions()
	opts.SetTransports(types.NewSet[transports.TransportCtor](WebTransport))
	s := NewServer(opts)
	t.Cleanup(func() { s.Close() })
	_ = s.On("connection", func(args ...any) {
		socket := args[0].(Socket)
		_ = socket.On("message", func(args ...any) {
			socket.Send(args[0].(io.Reader), nil, nil)
		})
	})
	conn := readinessWebTransport(t, s)
	readinessWrite(t, conn, "0")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, data, err := conn.ReadMessage(); err != nil || !bytes.HasPrefix(data, []byte(`0{"`)) {
		t.Fatalf("OPEN=%q error=%v", data, err)
	}
	for _, tc := range []struct {
		name string
		kind int
		data []byte
	}{
		{"text crossing buffer", webtrans.TextMessage, []byte("4" + strings.Repeat("x", 4900))},
		{"unicode", webtrans.TextMessage, []byte("4" + strings.Repeat("🙂汉字", 900))},
		{"binary extended length", webtrans.BinaryMessage, bytes.Repeat([]byte{0, 1, 255}, 24000)},
		{"following message", webtrans.TextMessage, []byte("4next")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writer, err := conn.NextWriter(tc.kind)
			if err != nil {
				t.Fatal(err)
			}
			for offset := 0; offset < len(tc.data); offset += 37 {
				if _, err = writer.Write(tc.data[offset:min(offset+37, len(tc.data))]); err != nil {
					t.Fatal(err)
				}
			}
			if err = writer.Close(); err != nil {
				t.Fatal(err)
			}
			_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
			kind, data, err := conn.ReadMessage()
			if err != nil || kind != tc.kind || !bytes.Equal(data, tc.data) {
				t.Fatalf("echo type=%d length=%d error=%v, want type=%d length=%d", kind, len(data), err, tc.kind, len(tc.data))
			}
		})
		if t.Failed() {
			return
		}
	}
}
