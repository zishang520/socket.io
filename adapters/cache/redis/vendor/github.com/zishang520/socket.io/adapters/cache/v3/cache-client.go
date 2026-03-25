// Package cache provides a backend-agnostic pub/sub and stream interface for
// Socket.IO clustering. It defines the CacheClient and CacheSubscription
// abstractions that Redis and Valkey implementations satisfy.
package cache

import (
	"context"
	"errors"
	"time"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// Sentinel errors for cache operations.
var (
	// ErrNil is returned when a cache operation produces a nil/not-found result.
	// Client wrappers (Redis, Valkey, …) must translate their own nil-result
	// errors to this value so adapter code can use a single errors.Is check.
	ErrNil = errors.New("cache: nil result")

	// ErrClosed is returned when an operation is attempted on a closed subscription.
	ErrClosed = errors.New("cache: subscription closed")
)

// CacheMessage is a pub/sub message received from the cache backend.
type CacheMessage struct {
	// Pattern is the glob pattern that matched this message.
	// Non-empty only for pattern subscriptions (PSubscribe).
	Pattern string

	// Channel is the channel on which the message was published.
	Channel string

	// Payload is the raw message bytes.
	Payload []byte
}

// CacheStreamEntry is a single entry in a cache stream (e.g. a Redis XREAD row).
type CacheStreamEntry struct {
	// ID is the stream entry identifier (e.g. "1700000000000-0").
	ID string

	// Values contains the field-value pairs for this entry.
	Values map[string]any
}

// CacheStream is the result for one stream key returned by XRead.
type CacheStream struct {
	// Name is the stream key.
	Name string

	// Messages contains the entries read from the stream.
	Messages []CacheStreamEntry
}

// CacheSubscription represents an active pub/sub subscription.
// All implementations must be safe for concurrent use.
type CacheSubscription interface {
	// C returns the read channel for incoming messages.
	// The channel is closed when Close is called or when the backing context
	// is cancelled, whichever happens first.
	C() <-chan *CacheMessage

	// PUnsubscribe removes the given glob patterns from a pattern subscription.
	PUnsubscribe(ctx context.Context, patterns ...string) error

	// Unsubscribe removes the given channels from a regular subscription.
	Unsubscribe(ctx context.Context, channels ...string) error

	// SUnsubscribe removes the given channels from a sharded pub/sub subscription.
	SUnsubscribe(ctx context.Context, channels ...string) error

	// Close terminates the subscription and closes the message channel.
	Close() error
}

// CacheClient is the unified interface for all cache backend operations used by
// the Socket.IO cluster adapters and emitters.
//
// It covers:
//   - Classic pub/sub (Subscribe / PSubscribe / Publish)
//   - Sharded pub/sub for Redis 7+ and Valkey (SSubscribe / SPublish)
//   - Stream operations (XAdd / XRead / XRange / XRangeN)
//   - Key-value storage for session persistence (Set / GetDel)
//
// All implementations must be safe for concurrent use from multiple goroutines.
type CacheClient interface {
	types.EventEmitter

	// Context returns the lifecycle context for this client.
	// The context governs the lifetime of subscriptions and polling loops.
	Context() context.Context

	// --- Classic Pub/Sub ---

	// Subscribe creates a subscription to one or more channels.
	Subscribe(ctx context.Context, channels ...string) CacheSubscription

	// PSubscribe creates a pattern-based subscription matching one or more glob patterns.
	PSubscribe(ctx context.Context, patterns ...string) CacheSubscription

	// Publish publishes a message to a channel, returning the first error encountered.
	Publish(ctx context.Context, channel string, message any) error

	// PubSubNumSub returns the number of subscribers for each of the given channels.
	PubSubNumSub(ctx context.Context, channels ...string) (map[string]int64, error)

	// --- Sharded Pub/Sub ---

	// SSubscribe creates a sharded pub/sub subscription to one or more channels.
	// Requires Redis 7+ or Valkey with cluster mode.
	SSubscribe(ctx context.Context, channels ...string) CacheSubscription

	// SPublish publishes a message to a sharded pub/sub channel.
	SPublish(ctx context.Context, channel string, message any) error

	// PubSubShardNumSub returns the number of sharded-pub/sub subscribers per channel.
	PubSubShardNumSub(ctx context.Context, channels ...string) (map[string]int64, error)

	// --- Streams ---

	// XAdd appends a new entry to a stream, returning the auto-generated entry ID.
	// When maxLen > 0 the stream is trimmed to approximately that length.
	// Set approx true (recommended) to allow Redis/Valkey to use "~" approximate trimming.
	XAdd(ctx context.Context, stream string, maxLen int64, approx bool, values map[string]any) (string, error)

	// XRead reads entries from one or more streams starting at id.
	// Use "$" for id to receive only new entries; count limits entries per stream.
	// block specifies how long to wait for new messages (0 = non-blocking).
	XRead(ctx context.Context, streams []string, id string, count int64, block time.Duration) ([]CacheStream, error)

	// XRange returns entries from stream in the inclusive range [start, stop].
	XRange(ctx context.Context, stream, start, stop string) ([]CacheStreamEntry, error)

	// XRangeN is like XRange but limits the result to at most count entries.
	XRangeN(ctx context.Context, stream, start, stop string, count int64) ([]CacheStreamEntry, error)

	// --- Key-Value (session persistence) ---

	// Set stores value at key with an optional TTL (0 = no expiry).
	Set(ctx context.Context, key string, value any, expiry time.Duration) error

	// GetDel atomically retrieves and removes the key.
	// Returns ("", ErrNil) when the key does not exist.
	GetDel(ctx context.Context, key string) (string, error)
}
