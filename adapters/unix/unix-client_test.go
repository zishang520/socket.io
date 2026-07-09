package unix

import (
	"bytes"
	"context"
	"testing"
)

func TestNewUnixClient(t *testing.T) {
	t.Run("with valid context", func(t *testing.T) {
		ctx := context.Background()
		uc := &UnixClient{
			Context:    ctx,
			SocketPath: "/tmp/test.sock",
		}

		if uc.Context != ctx {
			t.Fatal("Context mismatch")
		}
		if uc.SocketPath != "/tmp/test.sock" {
			t.Fatal("SocketPath mismatch")
		}
	})

	t.Run("with nil context defaults to background", func(t *testing.T) {
		uc := NewUnixClient(context.Background(), "/tmp/test.sock")

		if uc == nil {
			t.Fatal("Expected non-nil UnixClient")
		}
		if uc.Context == nil {
			t.Fatal("Expected non-nil Context (should default to Background)")
		}
	})
}

func TestUnixSocketPlatformOps(t *testing.T) {
	listenerPath := t.TempDir() + "/socket.io.sock"

	listener, mode, err := listenUnix(listenerPath)
	if err != nil {
		t.Fatalf("listenUnix failed: %v", err)
	}
	defer func() { _ = listener.Close() }()

	accepted := make(chan []byte, 1)
	acceptErr := make(chan error, 1)
	go func() {
		acceptedConn, acceptConnErr := listener.Accept()
		if acceptConnErr != nil {
			acceptErr <- acceptConnErr
			return
		}
		defer func() { _ = acceptedConn.Close() }()

		data, readErr := readUnixMessage(acceptedConn, mode)
		if readErr != nil {
			acceptErr <- readErr
			return
		}

		accepted <- data
	}()

	conn, connMode, err := dialUnix(listenerPath)
	if err != nil {
		t.Fatalf("dialUnix failed: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if connMode != mode {
		t.Fatalf("Expected dial mode %v to match listen mode %v", connMode, mode)
	}

	payload := []byte("platform-frame")
	if err := writeUnixMessage(conn, payload, connMode); err != nil {
		t.Fatalf("writeUnixMessage failed: %v", err)
	}

	select {
	case err := <-acceptErr:
		t.Fatalf("accept/read failed: %v", err)
	case got := <-accepted:
		if !bytes.Equal(got, payload) {
			t.Fatalf("Expected payload %q, got %q", payload, got)
		}
	}
}
