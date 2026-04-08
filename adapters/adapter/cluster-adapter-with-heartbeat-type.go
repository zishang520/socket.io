package adapter

import (
	"sync"
	"sync/atomic"

	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// CustomClusterRequest represents a custom request in the cluster with tracking for missing responses.
type (
	CustomClusterRequest struct {
		Type        MessageType
		Resolve     func(*types.Slice[any])
		Timeout     *atomic.Pointer[utils.Timer]
		MissingUids *types.Set[ServerId]
		Responses   *types.Slice[any]
		Once        sync.Once // guards against double callback invocation (timeout vs response race)
	}

	// ClusterAdapterWithHeartbeat extends ClusterAdapter with heartbeat and custom options support.
	ClusterAdapterWithHeartbeat interface {
		ClusterAdapter

		SetOpts(any)
	}
)
