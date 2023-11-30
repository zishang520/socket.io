package socket

import (
	"github.com/zishang520/engine.io/v2/types"
)

type (
	SocketDetails interface {
		Id() SocketId
		Handshake() *Handshake
		Rooms() *types.Set[Room]
		Data() any
	}
)
