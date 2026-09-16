package adapter

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type (
	// ServerId is the unique ID of a server.
	//
	ServerId string

	// Offset is the unique ID of a message (for the connection state recovery feature).
	Offset string

	// MessageType represents the type of cluster message.
	MessageType int

	// ClusterMessage contains common fields for all cluster messages.
	ClusterMessage struct {
		Uid  ServerId    `json:"uid" msgpack:"uid"`
		Nsp  string      `json:"nsp" msgpack:"nsp"`
		Type MessageType `json:"type" msgpack:"type"`
		Data any         `json:"data,omitzero" msgpack:"data,omitempty"`
	}

	// PacketOptions represents the options for broadcasting messages.
	PacketOptions struct {
		Rooms  []socket.Room          `json:"rooms" msgpack:"rooms" bson:"rooms"`
		Except []socket.Room          `json:"except" msgpack:"except" bson:"except"`
		Flags  *socket.BroadcastFlags `json:"flags,omitempty" msgpack:"flags,omitempty" bson:"flags,omitempty"`
	}

	// BroadcastMessage is a message for broadcasting.
	BroadcastMessage struct {
		Opts      *PacketOptions `json:"opts" msgpack:"opts"`
		Packet    *parser.Packet `json:"packet" msgpack:"packet"`
		RequestId *string        `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
	}

	// SocketsJoinLeaveMessage is a message for joining or leaving sockets.
	SocketsJoinLeaveMessage struct {
		Opts  *PacketOptions `json:"opts" msgpack:"opts"`
		Rooms []socket.Room  `json:"rooms" msgpack:"rooms"`
	}

	// DisconnectSocketsMessage is a message for disconnecting sockets.
	DisconnectSocketsMessage struct {
		Opts  *PacketOptions `json:"opts" msgpack:"opts"`
		Close bool           `json:"close" msgpack:"close"`
	}

	// FetchSocketsMessage is a message for fetching sockets.
	FetchSocketsMessage struct {
		Opts      *PacketOptions `json:"opts" msgpack:"opts"`
		RequestId string         `json:"requestId" msgpack:"requestId"`
	}

	// ServerSideEmitMessage is a message for server-side emit.
	ServerSideEmitMessage struct {
		RequestId *string `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Packet    []any   `json:"packet" msgpack:"packet"`
	}

	// ClusterRequest represents a cluster request.
	// It must not be copied after first use.
	ClusterRequest struct {
		Type      MessageType
		Resolve   func(*types.Slice[any])
		Timeout   *atomic.Pointer[utils.Timer]
		Expected  int64
		Current   *atomic.Int64
		Responses *types.Slice[any]

		// Sockets tracks responding server IDs, not client socket IDs.
		// Update it inside Responses.DoWrite together with result collection.
		Sockets types.Set[ServerId]
	}

	ClusterResponse = ClusterMessage

	// SocketResponse represents a socket response.
	SocketResponse struct {
		Id        socket.SocketId   `json:"id" msgpack:"id"`
		Handshake *socket.Handshake `json:"handshake" msgpack:"handshake"`
		Rooms     []socket.Room     `json:"rooms" msgpack:"rooms"`
		Data      any               `json:"data" msgpack:"data"`
	}

	// FetchSocketsResponse represents a response for fetching sockets.
	FetchSocketsResponse struct {
		RequestId string           `json:"requestId" msgpack:"requestId"`
		Sockets   []SocketResponse `json:"sockets" msgpack:"sockets"`
	}

	// ServerSideEmitResponse represents a response for server-side emit.
	// A nil Packet is encoded as null.
	ServerSideEmitResponse struct {
		RequestId string `json:"requestId" msgpack:"requestId"`
		Packet    any    `json:"packet" msgpack:"packet"`
	}

	// BroadcastClientCount represents a broadcast client count.
	BroadcastClientCount struct {
		RequestId   string `json:"requestId" msgpack:"requestId"`
		ClientCount uint64 `json:"clientCount" msgpack:"clientCount"`
	}

	// BroadcastAck represents a broadcast acknowledgment.
	// A nil Packet is encoded as null.
	BroadcastAck struct {
		RequestId string `json:"requestId" msgpack:"requestId"`
		Packet    any    `json:"packet" msgpack:"packet"`
	}

	// ClusterAckRequest represents a cluster acknowledgment request.
	// It must not be copied after first use.
	ClusterAckRequest struct {
		ClientCountCallback func(uint64)
		Ack                 socket.Ack

		// Sockets tracks servers that reported their client count, not client ACKs.
		Sockets types.Set[ServerId]
	}

	// PublishFunc sends a prepared message. It owns encoded data and routing
	// values, and must not retain the caller's mutable ClusterMessage.
	PublishFunc func() (Offset, error)

	// ClusterAdapter is an interface for a cluster-ready adapter.
	// Any implementation must provide methods for publishing messages and responses across the cluster.
	ClusterAdapter interface {
		Adapter

		// Uid returns the unique server ID.
		Uid() ServerId
		// OnMessage handles an incoming cluster message with its offset.
		OnMessage(*ClusterMessage, Offset)
		// OnResponse handles an incoming cluster response.
		OnResponse(*ClusterResponse)
		// Publish sends a cluster message to other nodes.
		Publish(*ClusterMessage)
		// PublishAndReturnOffset sends a message and returns its offset.
		PublishAndReturnOffset(*ClusterMessage) (Offset, error)
		// PublishAndClose queues a final message after accepted messages and responses,
		// then closes publishing. It returns without waiting for delivery.
		// This is a publishing-layer operation used by lifecycle implementations;
		// applications should call the outer adapter's Close for its lifecycle cleanup.
		PublishAndClose(*ClusterMessage)
		// PreparePublish encodes and selects a destination without publishing.
		// The returned function is executed by the publisher queue.
		PreparePublish(*ClusterMessage) (PublishFunc, error)
		// PublishResponse sends a response to a specific server.
		PublishResponse(ServerId, *ClusterResponse)
		// PreparePublishResponse prepares a response for the independent response queue.
		// The returned function's offset is ignored.
		PreparePublishResponse(ServerId, *ClusterResponse) (PublishFunc, error)
	}
)

// IsValid reports whether all required packet option fields are present.
func (p *PacketOptions) IsValid() bool {
	return p != nil && p.Rooms != nil && p.Except != nil
}

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

// IsValid performs a fast bounds check to ensure the MessageType is within defined enum constants.
func (m MessageType) IsValid() bool {
	return m >= INITIAL_HEARTBEAT && m <= ADAPTER_CLOSE
}
