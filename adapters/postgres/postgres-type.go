// Package postgres provides PostgreSQL-based adapter types and interfaces for Socket.IO clustering.
// These types define the message structures used for inter-node communication via PostgreSQL LISTEN/NOTIFY.
package postgres

import (
	"errors"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
)

// ErrNilPostgresPacket indicates an attempt to unmarshal into a nil PostgresPacket.
var ErrNilPostgresPacket = errors.New("cannot unmarshal into nil PostgresPacket")

type (
	// NotificationMessage represents a message received via PostgreSQL LISTEN/NOTIFY.
	// It can either contain the full payload or a reference to an attachment.
	NotificationMessage struct {
		Uid          adapter.ServerId    `json:"uid,omitempty"  msgpack:"uid,omitempty"`
		Type         adapter.MessageType `json:"type,omitempty"  msgpack:"type,omitempty"`
		AttachmentId string              `json:"attachmentId,omitempty"  msgpack:"attachmentId,omitempty"`
	}

	// Parser defines the interface for encoding and decoding data for PostgreSQL communication.
	// Implementations must be thread-safe as they may be called from multiple goroutines.
	Parser interface {
		// Encode serializes the given value into a byte slice.
		Encode(any) ([]byte, error)

		// Decode deserializes the byte slice into the given value.
		Decode([]byte, any) error
	}
)
