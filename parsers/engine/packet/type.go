package packet

import (
	"io"

	"github.com/zishang520/socket.io/v3/types"
)

type (
	Type string

	Options struct {
		Compress          *bool                 `json:"compress,omitempty" msgpack:"compress,omitempty"`
		WsPreEncodedFrame types.BufferInterface `json:"wsPreEncodedFrame,omitempty" msgpack:"wsPreEncodedFrame,omitempty"`
	}

	Packet struct {
		Type    Type      `json:"type" msgpack:"type"`
		Data    io.Reader `json:"data,omitempty" msgpack:"data,omitempty"`
		Options *Options  `json:"options,omitempty" msgpack:"options,omitempty"`
	}
)

// Packet types.
const (
	OPEN    Type = "open"
	CLOSE   Type = "close"
	PING    Type = "ping"
	PONG    Type = "pong"
	MESSAGE Type = "message"
	UPGRADE Type = "upgrade"
	NOOP    Type = "noop"
	ERROR   Type = "error"
)
