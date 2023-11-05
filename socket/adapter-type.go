package socket

import (
	"time"

	"github.com/zishang520/engine.io-go-parser/packet"
	"github.com/zishang520/engine.io/v2/types"
)

type (
	// A public ID, sent by the server at the beginning of the Socket.IO session and which can be used for private messaging
	SocketId string

	// A private ID, sent by the server at the beginning of the Socket.IO session and used for connection state recovery
	// upon reconnection
	PrivateSessionId string

	// we could extend the Room type to "string", but that would be a breaking change
	// Related: https://github.com/socketio/socket.io-redis-adapter/issues/418
	Room string

	WriteOptions struct {
		packet.Options

		Volatile   bool `json:"volatile" mapstructure:"volatile" msgpack:"volatile"`
		PreEncoded bool `json:"preEncoded" mapstructure:"preEncoded" msgpack:"preEncoded"`
	}

	BroadcastFlags struct {
		WriteOptions

		Local     bool           `json:"local" mapstructure:"local" msgpack:"local"`
		Broadcast bool           `json:"broadcast" mapstructure:"broadcast" msgpack:"broadcast"`
		Binary    bool           `json:"binary" mapstructure:"binary" msgpack:"binary"`
		Timeout   *time.Duration `json:"timeout,omitempty" mapstructure:"timeout,omitempty" msgpack:"timeout,omitempty"`

		ExpectSingleResponse bool `json:"expectSingleResponse" mapstructure:"expectSingleResponse" msgpack:"expectSingleResponse"`
	}

	BroadcastOptions struct {
		Rooms  *types.Set[Room] `json:"rooms,omitempty" mapstructure:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Except *types.Set[Room] `json:"except,omitempty" mapstructure:"except,omitempty" msgpack:"except,omitempty"`
		Flags  *BroadcastFlags  `json:"flags,omitempty" mapstructure:"flags,omitempty" msgpack:"flags,omitempty"`
	}

	SessionToPersist struct {
		Sid   SocketId         `json:"sid" mapstructure:"sid" msgpack:"sid"`
		Pid   PrivateSessionId `json:"pid" mapstructure:"pid" msgpack:"pid"`
		Rooms *types.Set[Room] `json:"rooms,omitempty" mapstructure:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Data  any              `json:"data" mapstructure:"data" msgpack:"data"`
	}

	Session struct {
		*SessionToPersist

		MissedPackets []any `json:"missedPackets" mapstructure:"missedPackets" msgpack:"missedPackets"`
	}

	PersistedPacket struct {
		Id        string            `json:"id" mapstructure:"id" msgpack:"id"`
		EmittedAt int64             `json:"emittedAt" mapstructure:"emittedAt" msgpack:"emittedAt"`
		Data      any               `json:"data" mapstructure:"data" msgpack:"data"`
		Opts      *BroadcastOptions `json:"opts,omitempty" mapstructure:"opts,omitempty" msgpack:"opts,omitempty"`
	}

	SessionWithTimestamp struct {
		*SessionToPersist

		DisconnectedAt int64 `json:"disconnectedAt" mapstructure:"disconnectedAt" msgpack:"disconnectedAt"`
	}
)
