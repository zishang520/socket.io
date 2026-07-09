package unix

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

type unixSocketMode uint8

const (
	unixSocketStream unixSocketMode = iota
	unixSocketPacket
)

func writeUnixStreamFrame(conn net.Conn, payload []byte) error {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	bufs := net.Buffers{header[:], payload}
	_, err := bufs.WriteTo(conn)
	return err
}

func readUnixStreamFrame(conn net.Conn) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint32(header[:])
	if msgLen == 0 || msgLen > maxMessageSize {
		return nil, fmt.Errorf("invalid Unix message length: %d", msgLen)
	}

	data := make([]byte, msgLen)
	_, err := io.ReadFull(conn, data)
	return data, err
}
