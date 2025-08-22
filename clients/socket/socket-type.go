package socket

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
)

// Flags represents emission flags for a socket event, such as volatile, timeout, and queue status.
type (
	Flags struct {
		packet.Options

		Volatile  bool           `json:"volatile" msgpack:"volatile"`
		Timeout   *time.Duration `json:"timeout,omitempty" msgpack:"timeout,omitempty"`
		FromQueue bool           `json:"fromQueue" msgpack:"fromQueue"`
	}

	// QueuedPacket represents a packet that is queued for guaranteed delivery with retry support.
	// Id is for debugging; deduplication should use a unique offset.
	QueuedPacket struct {
		Id       uint64
		Args     []any
		Flags    *Flags
		Pending  atomic.Bool
		TryCount atomic.Int64
	}
)
