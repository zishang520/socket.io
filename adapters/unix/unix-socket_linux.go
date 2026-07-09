//go:build linux

package unix

import (
	"fmt"
	"io"
	"net"

	"golang.org/x/sys/unix"
)

func listenUnix(path string) (net.Listener, unixSocketMode, error) {
	packetAddr := &net.UnixAddr{Name: path, Net: "unixpacket"}
	listener, err := net.ListenUnix("unixpacket", packetAddr)
	if err == nil {
		listener.SetUnlinkOnClose(true)
		return listener, unixSocketPacket, nil
	}

	streamAddr := &net.UnixAddr{Name: path, Net: "unix"}
	listener, err = net.ListenUnix("unix", streamAddr)
	if err != nil {
		return nil, unixSocketStream, err
	}
	listener.SetUnlinkOnClose(true)
	return listener, unixSocketStream, nil
}

func dialUnix(path string) (net.Conn, unixSocketMode, error) {
	conn, err := net.DialUnix("unixpacket", nil, &net.UnixAddr{Name: path, Net: "unixpacket"})
	if err == nil {
		return conn, unixSocketPacket, nil
	}

	conn, err = net.DialUnix("unix", nil, &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, unixSocketStream, err
	}
	return conn, unixSocketStream, nil
}

func writeUnixMessage(conn net.Conn, payload []byte, mode unixSocketMode) error {
	if mode == unixSocketPacket {
		return writeUnixPacket(conn, payload)
	}
	return writeUnixStreamFrame(conn, payload)
}

func readUnixMessage(conn net.Conn, mode unixSocketMode) ([]byte, error) {
	if mode == unixSocketPacket {
		return readUnixPacket(conn)
	}
	return readUnixStreamFrame(conn)
}

func writeUnixPacket(conn net.Conn, payload []byte) error {
	if len(payload) == 0 || len(payload) > maxMessageSize {
		return fmt.Errorf("invalid Unix packet length: %d", len(payload))
	}
	n, err := conn.Write(payload)
	if err != nil {
		return err
	}
	if n != len(payload) {
		return io.ErrShortWrite
	}
	return nil
}

func readUnixPacket(conn net.Conn) ([]byte, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("expected *net.UnixConn, got %T", conn)
	}

	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		return nil, err
	}

	var packetLen int
	var recvErr error
	peek := []byte{0}
	if err := rawConn.Read(func(fd uintptr) bool {
		packetLen, _, _, _, recvErr = unix.Recvmsg(int(fd), peek, nil, unix.MSG_PEEK|unix.MSG_TRUNC)
		return recvErr != unix.EAGAIN && recvErr != unix.EWOULDBLOCK
	}); err != nil {
		return nil, err
	}
	if recvErr != nil {
		return nil, recvErr
	}
	if packetLen == 0 || packetLen > maxMessageSize {
		return nil, fmt.Errorf("invalid Unix packet length: %d", packetLen)
	}

	data := make([]byte, packetLen)
	var n int
	if err := rawConn.Read(func(fd uintptr) bool {
		n, _, _, _, recvErr = unix.Recvmsg(int(fd), data, nil, 0)
		return recvErr != unix.EAGAIN && recvErr != unix.EWOULDBLOCK
	}); err != nil {
		return nil, err
	}
	if recvErr != nil {
		return nil, recvErr
	}
	if n != packetLen {
		return nil, fmt.Errorf("short Unix packet read: got %d, want %d", n, packetLen)
	}
	return data[:n], nil
}
