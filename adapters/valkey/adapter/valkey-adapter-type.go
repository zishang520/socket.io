// Package adapter defines types and interfaces for the Valkey-based Socket.IO adapter implementation.
package adapter

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type (
	// Packet is an alias for valkey.ValkeyPacket, representing a broadcast packet sent via Valkey.
	Packet = valkey.ValkeyPacket

	// Request is an alias for valkey.ValkeyRequest, representing an inter-node request.
	Request = valkey.ValkeyRequest

	// Response is an alias for valkey.ValkeyResponse, representing an inter-node response.
	Response = valkey.ValkeyResponse

	// AckRequest is an alias for adapter.ClusterAckRequest, used for acknowledgement tracking.
	AckRequest = adapter.ClusterAckRequest

	// ValkeyRequest represents an internal request tracker with state management.
	ValkeyRequest struct {
		Type      adapter.MessageType
		Resolve   func(*types.Slice[any])
		Timeout   *atomic.Pointer[utils.Timer]
		NumSub    int64
		MsgCount  *atomic.Int64
		Rooms     *types.Set[socket.Room]
		Sockets   *types.Slice[*adapter.SocketResponse]
		Responses *types.Slice[any]
	}

	// ValkeyAdapter defines the interface for a Valkey-based Socket.IO adapter.
	ValkeyAdapter interface {
		socket.Adapter

		SetValkey(*valkey.ValkeyClient)
		SetOpts(any)
		Uid() adapter.ServerId
		RequestsTimeout() time.Duration
		PublishOnSpecificResponseChannel() bool
		Parser() valkey.Parser
		AllRooms() func(func(*types.Set[socket.Room], error))
	}
)
