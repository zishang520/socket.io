// Package redis provides a Redis client wrapper that implements the
// cache.CacheClient interface, enabling the Socket.IO cache adapters to run
// against a Redis (standalone, sentinel, or cluster) backend.
package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	rds "github.com/redis/go-redis/v9"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// RedisClient wraps a go-redis UniversalClient and implements cache.CacheClient.
// It provides context management, event emission for error propagation,
// and transparent translation of go-redis API types to the cache abstraction.
type RedisClient struct {
	types.EventEmitter

	// client is the underlying go-redis universal client.
	client rds.UniversalClient

	// ctx controls the lifecycle of this client's subscriptions and operations.
	ctx context.Context
}

// NewRedisClient creates a new RedisClient with the given context and go-redis client.
//
// Parameters:
//   - ctx: Lifecycle context. When canceled, all subscriptions are terminated.
//     Defaults to context.Background() if nil.
//   - client: A go-redis UniversalClient (supports standalone, sentinel, and cluster).
//
// Example:
//
//	rdb := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
//	c := redis.NewRedisClient(context.Background(), rdb)
//	io.Adapter(&cacheadapter.CacheAdapterBuilder{Cache: c})
func NewRedisClient(ctx context.Context, client rds.UniversalClient) *RedisClient {
	if ctx == nil {
		ctx = context.Background()
	}
	return &RedisClient{
		EventEmitter: types.NewEventEmitter(),
		client:       client,
		ctx:          ctx,
	}
}

// Context returns the lifecycle context for this client.
func (r *RedisClient) Context() context.Context { return r.ctx }

// --- Classic pub/sub ---

// Subscribe creates a channel subscription and returns a CacheSubscription.
func (r *RedisClient) Subscribe(ctx context.Context, channels ...string) cache.CacheSubscription {
	pubsub := r.client.Subscribe(ctx, channels...)
	return newRedisSubscription(ctx, pubsub)
}

// PSubscribe creates a pattern subscription and returns a CacheSubscription.
func (r *RedisClient) PSubscribe(ctx context.Context, patterns ...string) cache.CacheSubscription {
	pubsub := r.client.PSubscribe(ctx, patterns...)
	return newRedisSubscription(ctx, pubsub)
}

// Publish publishes message to channel.
func (r *RedisClient) Publish(ctx context.Context, channel string, message any) error {
	return r.client.Publish(ctx, channel, message).Err()
}

// PubSubNumSub returns the number of subscribers per channel.
func (r *RedisClient) PubSubNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	return r.client.PubSubNumSub(ctx, channels...).Result()
}

// --- Sharded pub/sub ---

// SSubscribe creates a sharded pub/sub subscription.
func (r *RedisClient) SSubscribe(ctx context.Context, channels ...string) cache.CacheSubscription {
	pubsub := r.client.SSubscribe(ctx, channels...)
	return newRedisSubscription(ctx, pubsub)
}

// SPublish publishes message to a sharded pub/sub channel.
func (r *RedisClient) SPublish(ctx context.Context, channel string, message any) error {
	return r.client.SPublish(ctx, channel, message).Err()
}

// PubSubShardNumSub returns the number of sharded-sub subscribers per channel.
func (r *RedisClient) PubSubShardNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	return r.client.PubSubShardNumSub(ctx, channels...).Result()
}

// --- Streams ---

// XAdd appends an entry to a stream.
func (r *RedisClient) XAdd(ctx context.Context, stream string, maxLen int64, approx bool, values map[string]any) (string, error) {
	return r.client.XAdd(ctx, &rds.XAddArgs{
		Stream: stream,
		MaxLen: maxLen,
		Approx: approx,
		ID:     "*",
		Values: values,
	}).Result()
}

// XRead reads entries from one or more streams.
func (r *RedisClient) XRead(ctx context.Context, streams []string, id string, count int64, block time.Duration) ([]cache.CacheStream, error) {
	args := make([]string, len(streams)*2)
	copy(args[:len(streams)], streams)
	for i := range streams {
		args[len(streams)+i] = id
	}

	result, err := r.client.XRead(ctx, &rds.XReadArgs{
		Streams: args,
		ID:      id,
		Count:   count,
		Block:   block,
	}).Result()
	if err != nil {
		if errors.Is(err, rds.Nil) {
			return nil, cache.ErrNil
		}
		return nil, err
	}

	out := make([]cache.CacheStream, len(result))
	for i, s := range result {
		entries := make([]cache.CacheStreamEntry, len(s.Messages))
		for j, m := range s.Messages {
			entries[j] = cache.CacheStreamEntry{ID: m.ID, Values: m.Values}
		}
		out[i] = cache.CacheStream{Name: s.Stream, Messages: entries}
	}
	return out, nil
}

// XRange returns entries in [start, stop] from stream.
func (r *RedisClient) XRange(ctx context.Context, stream, start, stop string) ([]cache.CacheStreamEntry, error) {
	result, err := r.client.XRange(ctx, stream, start, stop).Result()
	if err != nil {
		return nil, err
	}
	out := make([]cache.CacheStreamEntry, len(result))
	for i, m := range result {
		out[i] = cache.CacheStreamEntry{ID: m.ID, Values: m.Values}
	}
	return out, nil
}

// XRangeN is like XRange but limited to count entries.
func (r *RedisClient) XRangeN(ctx context.Context, stream, start, stop string, count int64) ([]cache.CacheStreamEntry, error) {
	result, err := r.client.XRangeN(ctx, stream, start, stop, count).Result()
	if err != nil {
		return nil, err
	}
	out := make([]cache.CacheStreamEntry, len(result))
	for i, m := range result {
		out[i] = cache.CacheStreamEntry{ID: m.ID, Values: m.Values}
	}
	return out, nil
}

// --- Key-Value ---

// Set stores value at key with an optional TTL.
func (r *RedisClient) Set(ctx context.Context, key string, value any, expiry time.Duration) error {
	return r.client.Set(ctx, key, value, expiry).Err()
}

// GetDel atomically gets and deletes key.
// Returns ("", cache.ErrNil) when the key does not exist.
func (r *RedisClient) GetDel(ctx context.Context, key string) (string, error) {
	val, err := r.client.GetDel(ctx, key).Result()
	if err != nil {
		if errors.Is(err, rds.Nil) {
			return "", cache.ErrNil
		}
		return "", err
	}
	return val, nil
}

// ---------------------------------------------------------------------------
// RedisSubscription wraps a *rds.PubSub and satisfies cache.CacheSubscription.
// ---------------------------------------------------------------------------

// redisSubscription wraps a go-redis PubSub and exposes messages via a Go channel.
type redisSubscription struct {
	pubsub *rds.PubSub
	ch     chan *cache.CacheMessage
	ctx    context.Context
}

// newRedisSubscription creates a subscription and starts a goroutine that pumps
// messages from pubsub.ReceiveMessage into the returned channel.
func newRedisSubscription(ctx context.Context, pubsub *rds.PubSub) *redisSubscription {
	s := &redisSubscription{
		pubsub: pubsub,
		ch:     make(chan *cache.CacheMessage, 256),
		ctx:    ctx,
	}
	go s.pump()
	return s
}

// pump reads from the go-redis PubSub and forwards messages to ch.
func (s *redisSubscription) pump() {
	defer close(s.ch)
	for {
		msg, err := s.pubsub.ReceiveMessage(s.ctx)
		if err != nil {
			if errors.Is(err, rds.ErrClosed) || s.ctx.Err() != nil {
				return
			}
			// Non-fatal: log and continue.  Callers observe the closed channel
			// on context cancellation.
			continue
		}
		select {
		case s.ch <- &cache.CacheMessage{
			Pattern: msg.Pattern,
			Channel: msg.Channel,
			Payload: []byte(msg.Payload),
		}:
		case <-s.ctx.Done():
			return
		}
	}
}

// C returns the read channel for incoming messages.
func (s *redisSubscription) C() <-chan *cache.CacheMessage { return s.ch }

// PUnsubscribe removes pattern subscriptions.
func (s *redisSubscription) PUnsubscribe(ctx context.Context, patterns ...string) error {
	if err := s.pubsub.PUnsubscribe(ctx, patterns...); err != nil {
		return fmt.Errorf("redis PUnsubscribe: %w", err)
	}
	return nil
}

// Unsubscribe removes channel subscriptions.
func (s *redisSubscription) Unsubscribe(ctx context.Context, channels ...string) error {
	if err := s.pubsub.Unsubscribe(ctx, channels...); err != nil {
		return fmt.Errorf("redis Unsubscribe: %w", err)
	}
	return nil
}

// SUnsubscribe removes sharded pub/sub channel subscriptions.
func (s *redisSubscription) SUnsubscribe(ctx context.Context, channels ...string) error {
	if err := s.pubsub.SUnsubscribe(ctx, channels...); err != nil {
		return fmt.Errorf("redis SUnsubscribe: %w", err)
	}
	return nil
}

// Close terminates the subscription.
func (s *redisSubscription) Close() error {
	return s.pubsub.Close()
}
