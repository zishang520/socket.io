package adapter

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/engine.io/v2/utils"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
	"github.com/zishang520/socket.io/v2/socket"
)

type (
	// The unique ID of a server
	ServerId string

	// The unique ID of a message (for the connection state recovery feature)
	Offset string

	MessageType int

	// Common fields for all messages
	ClusterMessage struct {
		Uid  ServerId    `json:"uid,omitempty" msgpack:"uid,omitempty"`
		Nsp  string      `json:"nsp,omitempty" msgpack:"nsp,omitempty"`
		Type MessageType `json:"type,omitempty" msgpack:"type,omitempty"`
		Data any         `json:"data,omitempty" msgpack:"data,omitempty"` // Data will hold the specific message data for different types
	}

	// PacketOptions represents the options for broadcasting messages.
	PacketOptions struct {
		Rooms  []socket.Room          `json:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Except []socket.Room          `json:"except,omitempty" msgpack:"except,omitempty"`
		Flags  *socket.BroadcastFlags `json:"flags,omitempty" msgpack:"flags,omitempty"`
	}

	// Message for BROADCAST
	BroadcastMessage struct {
		Opts      *PacketOptions `json:"opts,omitempty" msgpack:"opts,omitempty"`
		Packet    *parser.Packet `json:"packet,omitempty" msgpack:"packet,omitempty"`
		RequestId *string        `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
	}

	// Message for SOCKETS_JOIN, SOCKETS_LEAVE
	SocketsJoinLeaveMessage struct {
		Opts  *PacketOptions `json:"opts,omitempty" msgpack:"opts,omitempty"`
		Rooms []socket.Room  `json:"rooms,omitempty" msgpack:"rooms,omitempty"`
	}

	// Message for DISCONNECT_SOCKETS
	DisconnectSocketsMessage struct {
		Opts  *PacketOptions `json:"opts,omitempty" msgpack:"opts,omitempty"`
		Close bool           `json:"close,omitempty" msgpack:"close,omitempty"`
	}

	// Message for FETCH_SOCKETS
	FetchSocketsMessage struct {
		Opts      *PacketOptions `json:"opts,omitempty" msgpack:"opts,omitempty"`
		RequestId string         `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
	}

	// Message for SERVER_SIDE_EMIT
	ServerSideEmitMessage struct {
		RequestId *string `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Packet    []any   `json:"packet,omitempty" msgpack:"packet,omitempty"`
	}

	// ClusterRequest equivalent
	ClusterRequest struct {
		Type      MessageType
		Resolve   func(*types.Slice[any])
		Timeout   *atomic.Pointer[utils.Timer]
		Expected  int64
		Current   *atomic.Int64
		Responses *types.Slice[any]
	}

	ClusterResponse = ClusterMessage

	SocketResponse struct {
		Id        socket.SocketId   `json:"id,omitempty" msgpack:"id,omitempty"`
		Handshake *socket.Handshake `json:"handshake,omitempty" msgpack:"handshake,omitempty"`
		Rooms     []socket.Room     `json:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Data      any               `json:"data,omitempty" msgpack:"data,omitempty"`
	}

	FetchSocketsResponse struct {
		RequestId string            `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Sockets   []*SocketResponse `json:"sockets,omitempty" msgpack:"sockets,omitempty"`
	}

	ServerSideEmitResponse struct {
		RequestId string `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Packet    []any  `json:"packet,omitempty" msgpack:"packet,omitempty"`
	}

	BroadcastClientCountResponse struct {
		RequestId   string `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		ClientCount uint64 `json:"clientCount,omitempty" msgpack:"clientCount,omitempty"`
	}

	BroadcastAck struct {
		RequestId string `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Packet    []any  `json:"packet,omitempty" msgpack:"packet,omitempty"`
	}

	ClusterAckRequest struct {
		ClientCountCallback func(uint64)
		Ack                 socket.Ack
	}
)

const (
	EMITTER_UID     ServerId      = "emitter"
	DEFAULT_TIMEOUT time.Duration = 5_000 * time.Millisecond
)

const (
	INITIAL_HEARTBEAT MessageType = iota + 1
	HEARTBEAT
	BROADCAST
	SOCKETS_JOIN
	SOCKETS_LEAVE
	DISCONNECT_SOCKETS
	FETCH_SOCKETS
	FETCH_SOCKETS_RESPONSE
	SERVER_SIDE_EMIT
	SERVER_SIDE_EMIT_RESPONSE
	BROADCAST_CLIENT_COUNT
	BROADCAST_ACK
	ADAPTER_CLOSE
)
