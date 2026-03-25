// Package adapter defines the interface for the cache-backed sharded pub/sub adapter.
package adapter

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
)

// ShardedCacheAdapter is the interface for a sharded pub/sub Socket.IO adapter.
// It extends ClusterAdapter with cache-specific configuration methods.
type ShardedCacheAdapter interface {
	adapter.ClusterAdapter

	// SetCache configures the cache client for the adapter.
	SetCache(cache.CacheClient)

	// SetOpts configures adapter options.
	SetOpts(any)
}
