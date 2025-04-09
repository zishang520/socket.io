package socket

import (
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

type (
	ReadyState string

	Packet struct {
		*parser.Packet

		Options *packet.Options `json:"options,omitempty" msgpack:"options,omitempty"`
	}

	Handshake struct {
		Sid string `json:"sid" msgpack:"sid"`
		Pid string `json:"pid,omitempty" msgpack:"pid,omitempty"`
	}
)

const (
	ReadyStateOpen    ReadyState = "open"
	ReadyStateOpening ReadyState = "opening"
	ReadyStateClosed  ReadyState = "closed"
)
