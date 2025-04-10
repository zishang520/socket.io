package adapter

import (
	"sync/atomic"

	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
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
