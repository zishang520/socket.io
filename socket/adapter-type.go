package socket

import (
	"time"

	"github.com/zishang520/engine.io/types"
)

// A public ID, sent by the server at the beginning of the Socket.IO session and which can be used for private messaging
type SocketId string

// A private ID, sent by the server at the beginning of the Socket.IO session and used for connection state recovery
// upon reconnection
type PrivateSessionId string

// we could extend the Room type to "string | number", but that would be a breaking change
// Related: https://github.com/socketio/socket.io-redis-adapter/issues/418
type Room string

type WriteOptions struct {
	packet.Options

	Volatile     bool
	PreEncoded   bool
	WsPreEncoded string
}

type BroadcastFlags struct {
	WriteOptions

	Local     bool
	Broadcast bool
	Binary    bool
	Timeout   *time.Duration

	ExpectSingleResponse bool
}

type BroadcastOptions struct {
	Rooms  *types.Set[Room]
	Except *types.Set[Room]
	Flags  *BroadcastFlags
}

type SessionToPersist struct {
	Sid   SocketId
	Pid   PrivateSessionId
	Rooms *types.Set[Room]
	Data  any
}

type Session struct {
	SessionToPersist

	MissedPackets [][]any
}

type PersistedPacket struct {
	Id        string
	EmittedAt int64
	Data      []any
	Opts      *BroadcastOptions
}

type SessionWithTimestamp struct {
	SessionToPersist

	DisconnectedAt int64
}
