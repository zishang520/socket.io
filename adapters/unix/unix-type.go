// Package unix provides Unix Domain Socket-based adapter types and interfaces for Socket.IO clustering.
// These types define the message structures used for inter-node communication via Unix Domain Sockets.
package unix

import (
	"errors"
)

// ErrNilUnixPacket indicates an attempt to unmarshal into a nil UnixPacket.
var ErrNilUnixPacket = errors.New("cannot unmarshal into nil UnixPacket")

// Parser defines the interface for encoding and decoding data for Unix Domain Socket communication.
// Implementations must be thread-safe as they may be called from multiple goroutines.
type Parser interface {
	// Encode serializes the given value into a byte slice.
	Encode(any) ([]byte, error)

	// Decode deserializes the byte slice into the given value.
	Decode([]byte, any) error
}
