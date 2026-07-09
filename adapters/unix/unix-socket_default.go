//go:build !linux

package unix

import (
	"net"
)

func listenUnix(path string) (net.Listener, unixSocketMode, error) {
	listener, err := net.Listen("unix", path)
	return listener, unixSocketStream, err
}

func dialUnix(path string) (net.Conn, unixSocketMode, error) {
	conn, err := net.Dial("unix", path)
	return conn, unixSocketStream, err
}

func writeUnixMessage(conn net.Conn, payload []byte, mode unixSocketMode) error {
	return writeUnixStreamFrame(conn, payload)
}

func readUnixMessage(conn net.Conn, mode unixSocketMode) ([]byte, error) {
	return readUnixStreamFrame(conn)
}
