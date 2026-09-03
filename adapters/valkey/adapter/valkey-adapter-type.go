// Package adapter defines types and interfaces for the Valkey-based Socket.IO adapter implementation.
package adapter

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type (
	Packet   = valkey.ValkeyPacket
	Request  = valkey.ValkeyRequest
	Response = valkey.ValkeyResponse

	AckRequest = adapter.ClusterAckRequest

	// ValkeyRequest tracks the lifecycle and accumulated state of a pending request.
	ValkeyRequest struct {
		Type      valkey.RequestType
		Resolve   func(*types.Slice[any])
		Timeout   atomic.Pointer[utils.Timer]
		NumSub    int64
		MsgCount  atomic.Int64
		Rooms     *types.Set[socket.Room]
		Sockets   *types.Set[socket.SocketId]
		Responses *types.Slice[any]
	}

	// ValkeyAdapter defines the public classic Valkey adapter contract.
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
