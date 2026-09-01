package unix

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"time"
)

// ErrUnixMessageSize is returned when a Unix message is empty or exceeds the
// maximum frame size.
var ErrUnixMessageSize = errors.New("unix: message size must be between 1 byte and 10 MiB")

func listenUnix(path string) (net.Listener, error) {
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(true)

	// Windows does not implement Unix permission bits for AF_UNIX socket files.
	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o600); err != nil {
			_ = listener.Close()
			return nil, fmt.Errorf("failed to set Unix socket permissions: %w", err)
		}
	}

	return listener, nil
}

func dialUnix(ctx context.Context, path string, timeout time.Duration) (net.Conn, error) {
	dialer := net.Dialer{Timeout: timeout}
	return dialer.DialContext(ctx, "unix", path)
}

func validateUnixMessage(payload []byte) error {
	if len(payload) == 0 || len(payload) > maxMessageSize {
		return fmt.Errorf("%w: %d", ErrUnixMessageSize, len(payload))
	}
	return nil
}

func writeUnixMessage(conn net.Conn, payload []byte) error {
	if err := validateUnixMessage(payload); err != nil {
		return err
	}

	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	bufs := net.Buffers{header[:], payload}
	n, err := bufs.WriteTo(conn)
	if err == nil && n != int64(len(header)+len(payload)) {
		return io.ErrShortWrite
	}
	return err
}

func readUnixMessage(conn net.Conn) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(header[:])
	if msgLen == 0 || msgLen > maxMessageSize {
		return nil, fmt.Errorf("%w: %d", ErrUnixMessageSize, msgLen)
	}

	data := make([]byte, msgLen)
	_, err := io.ReadFull(conn, data)
	return data, err
}
