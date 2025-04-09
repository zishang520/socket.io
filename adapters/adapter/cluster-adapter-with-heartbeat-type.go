package adapter

import (
	"sync/atomic"

	"github.com/zishang520/socket.io/servers/engine/v3/types"
	"github.com/zishang520/socket.io/servers/engine/v3/utils"
)

type (
	CustomClusterRequest struct {
		Type        MessageType
		Resolve     func(*types.Slice[any])
		Timeout     *atomic.Pointer[utils.Timer]
		MissingUids *types.Set[ServerId]
		Responses   *types.Slice[any]
	}

	ClusterAdapterWithHeartbeat interface {
		ClusterAdapter

		SetOpts(any)
	}
)
