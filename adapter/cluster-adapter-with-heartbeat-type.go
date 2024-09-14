package adapter

import (
	"sync/atomic"

	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/engine.io/v2/utils"
)

type (
	CustomClusterRequest struct {
		Type        MessageType
		Resolve     func(*types.Slice[any])
		Timeout     *atomic.Pointer[utils.Timer]
		MissingUids *types.Set[ServerId]
		Responses   *types.Slice[any]
	}
)
