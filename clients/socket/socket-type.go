package socket

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
)

type (
	Flags struct {
		packet.Options

		Volatile  bool           `json:"volatile" msgpack:"volatile"`
		Timeout   *time.Duration `json:"timeout,omitempty" msgpack:"timeout,omitempty"`
		FromQueue bool           `json:"fromQueue" msgpack:"fromQueue"`
	}

	QueuedPacket struct {
		// Only used for debugging purposes. To allow deduplication on the server side, one should include a unique offset in
		// the packet, for example with crypto.randomUUID().
		//
		// @see https://developer.mozilla.org/en-US/docs/Web/API/Crypto/randomUUID
		Id       uint64
		Args     []any
		Flags    *Flags
		Pending  atomic.Bool
		TryCount atomic.Int64
	}
)
