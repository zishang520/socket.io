package unix

import (
	"bytes"
	"context"
	"errors"
	"testing"
)

func TestNewUnixClient(t *testing.T) {
	t.Run("with valid context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		uc, err := NewUnixClient(ctx, "/tmp/test.sock")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = uc.Close() })

		if uc.SocketPath() != "/tmp/test.sock" {
			t.Fatal("SocketPath mismatch")
		}
		cancel()
		<-uc.Context().Done()
	})

	t.Run("with nil context defaults to background", func(t *testing.T) {
		uc, err := NewUnixClient(nil, "/tmp/test.sock") //nolint:staticcheck // Verify the nil-context fallback.
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = uc.Close() })

		if uc == nil {
			t.Fatal("Expected non-nil UnixClient")
		}
		if uc.Context() == nil {
			t.Fatal("Expected non-nil Context (should default to Background)")
		}
	})

	t.Run("requires socket path", func(t *testing.T) {
		uc, err := NewUnixClient(context.Background(), "")
		if uc != nil || !errors.Is(err, ErrUnixSocketPathRequired) {
			t.Fatalf("NewUnixClient() = (%v, %v), want (nil, ErrUnixSocketPathRequired)", uc, err)
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
