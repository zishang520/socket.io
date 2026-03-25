// Package valkey provides a Valkey client wrapper that implements the
// cache.CacheClient interface, enabling the Socket.IO cache adapters to run
// against a Valkey (standalone or cluster) backend using the valkey-go library.
//
// Valkey is an open-source, Redis-compatible data store forked from Redis 7.2.
// This package translates the valkey-go command-builder API to the generic
// cache.CacheClient interface so that existing adapter and emitter code works
// without modification.
package valkey

import (
	"context"
	"fmt"
	"strconv"
	"time"

	vk "github.com/valkey-io/valkey-go"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// ValkeyClient wraps a valkey-go Client and implements cache.CacheClient.
type ValkeyClient struct {
	types.EventEmitter

	client vk.Client
	ctx    context.Context
}

// NewValkeyClient creates a new ValkeyClient.
//
// Parameters:
//   - ctx: Lifecycle context. Cancellation terminates all subscriptions.
//     Defaults to context.Background() if nil.
//   - client: A valkey.Client (supports both standalone and cluster modes).
//
// Example:
//
//	vc, _ := valkey.NewClient(valkey.ClientOption{InitAddress: []string{"127.0.0.1:6379"}})
//	c := valkeyimpl.NewValkeyClient(context.Background(), vc)
//	io.Adapter(&cacheadapter.CacheAdapterBuilder{Cache: c})
func NewValkeyClient(ctx context.Context, client vk.Client) *ValkeyClient {
	if ctx == nil {
		ctx = context.Background()
	}
	return &ValkeyClient{
		EventEmitter: types.NewEventEmitter(),
		client:       client,
		ctx:          ctx,
	}
}

// Context returns the lifecycle context for this client.
func (v *ValkeyClient) Context() context.Context { return v.ctx }

// --- Classic Pub/Sub ---

// Subscribe creates a channel subscription.
func (v *ValkeyClient) Subscribe(ctx context.Context, channels ...string) cache.CacheSubscription {
	cmd := v.client.B().Subscribe().Channel(channels...).Build()
	return newValkeySubscription(ctx, v.client, cmd)
}

// PSubscribe creates a pattern subscription.
func (v *ValkeyClient) PSubscribe(ctx context.Context, patterns ...string) cache.CacheSubscription {
	cmd := v.client.B().Psubscribe().Pattern(patterns...).Build()
	return newValkeySubscription(ctx, v.client, cmd)
}

// Publish publishes a message to a channel.
func (v *ValkeyClient) Publish(ctx context.Context, channel string, message any) error {
	return v.client.Do(ctx,
		v.client.B().Publish().Channel(channel).Message(toStr(message)).Build(),
	).Error()
}

// PubSubNumSub returns the subscriber count per channel.
// The PUBSUB NUMSUB command returns a flat alternating [channel, count, …] list.
func (v *ValkeyClient) PubSubNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	pairs, err := v.client.Do(ctx,
		v.client.B().PubsubNumsub().Channel(channels...).Build(),
	).AsStrSlice()
	if err != nil {
		return nil, err
	}
	return parseNumSubPairs(pairs), nil
}

// --- Sharded Pub/Sub ---

// SSubscribe creates a sharded pub/sub subscription.
func (v *ValkeyClient) SSubscribe(ctx context.Context, channels ...string) cache.CacheSubscription {
	cmd := v.client.B().Ssubscribe().Channel(channels...).Build()
	return newValkeySubscription(ctx, v.client, cmd)
}

// SPublish publishes a message to a sharded pub/sub channel.
func (v *ValkeyClient) SPublish(ctx context.Context, channel string, message any) error {
	return v.client.Do(ctx,
		v.client.B().Spublish().Channel(channel).Message(toStr(message)).Build(),
	).Error()
}

// PubSubShardNumSub returns the subscriber count per sharded channel.
func (v *ValkeyClient) PubSubShardNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	pairs, err := v.client.Do(ctx,
		v.client.B().PubsubShardnumsub().Channel(channels...).Build(),
	).AsStrSlice()
	if err != nil {
		return nil, err
	}
	return parseNumSubPairs(pairs), nil
}

// --- Streams ---

// XAdd appends an entry to a stream and returns the auto-generated entry ID.
// When maxLen > 0 the stream is capped; approx enables "~" approximate trimming.
//
// The valkey-go builder uses separate code paths per trimming option because the
// intermediate builder types live in an internal package and cannot be stored in
// named variables.  The three branches (no trim, approx, exact) are therefore
// written out explicitly.
func (v *ValkeyClient) XAdd(ctx context.Context, stream string, maxLen int64, approx bool, values map[string]any) (string, error) {
	if maxLen <= 0 {
		b := v.client.B().Xadd().Key(stream).Id("*").FieldValue()
		for k, val := range values {
			b = b.FieldValue(k, toStr(val))
		}
		return v.client.Do(ctx, b.Build()).ToString()
	}

	threshold := strconv.FormatInt(maxLen, 10)

	if approx {
		b := v.client.B().Xadd().Key(stream).Maxlen().Almost().Threshold(threshold).Id("*").FieldValue()
		for k, val := range values {
			b = b.FieldValue(k, toStr(val))
		}
		return v.client.Do(ctx, b.Build()).ToString()
	}

	b := v.client.B().Xadd().Key(stream).Maxlen().Threshold(threshold).Id("*").FieldValue()
	for k, val := range values {
		b = b.FieldValue(k, toStr(val))
	}
	return v.client.Do(ctx, b.Build()).ToString()
}

// XRead reads entries from one or more streams starting at id.
// block specifies the BLOCK timeout in milliseconds (0 = non-blocking).
func (v *ValkeyClient) XRead(ctx context.Context, streams []string, id string, count int64, block time.Duration) ([]cache.CacheStream, error) {
	// Build one ID per stream (all the same).
	ids := make([]string, len(streams))
	for i := range ids {
		ids[i] = id
	}

	res, err := v.client.Do(ctx,
		v.client.B().Xread().Count(count).Block(block.Milliseconds()).Streams().Key(streams...).Id(ids...).Build(),
	).AsXRead()
	if err != nil {
		if vk.IsValkeyNil(err) {
			return nil, cache.ErrNil
		}
		return nil, err
	}

	out := make([]cache.CacheStream, 0, len(res))
	for name, entries := range res {
		out = append(out, cache.CacheStream{
			Name:     name,
			Messages: toStreamEntries(entries),
		})
	}
	return out, nil
}

// XRange returns entries in the inclusive range [start, stop] from stream.
func (v *ValkeyClient) XRange(ctx context.Context, stream, start, stop string) ([]cache.CacheStreamEntry, error) {
	res, err := v.client.Do(ctx,
		v.client.B().Xrange().Key(stream).Start(start).End(stop).Build(),
	).AsXRange()
	if err != nil {
		return nil, err
	}
	return toStreamEntries(res), nil
}

// XRangeN is like XRange but limits the result to count entries.
func (v *ValkeyClient) XRangeN(ctx context.Context, stream, start, stop string, count int64) ([]cache.CacheStreamEntry, error) {
	res, err := v.client.Do(ctx,
		v.client.B().Xrange().Key(stream).Start(start).End(stop).Count(count).Build(),
	).AsXRange()
	if err != nil {
		return nil, err
	}
	return toStreamEntries(res), nil
}

// --- Key-Value ---

// Set stores value at key with an optional TTL (0 = no expiry).
func (v *ValkeyClient) Set(ctx context.Context, key string, value any, expiry time.Duration) error {
	if expiry > 0 {
		return v.client.Do(ctx,
			v.client.B().Set().Key(key).Value(toStr(value)).Ex(expiry).Build(),
		).Error()
	}
	return v.client.Do(ctx,
		v.client.B().Set().Key(key).Value(toStr(value)).Build(),
	).Error()
}

// GetDel atomically gets and deletes key.
// Returns ("", cache.ErrNil) when the key does not exist.
func (v *ValkeyClient) GetDel(ctx context.Context, key string) (string, error) {
	val, err := v.client.Do(ctx,
		v.client.B().Getdel().Key(key).Build(),
	).ToString()
	if err != nil {
		if vk.IsValkeyNil(err) {
			return "", cache.ErrNil
		}
		return "", err
	}
	return val, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// toStr converts a value to a string suitable for use in Valkey commands.
func toStr(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprint(v)
	}
}

// parseNumSubPairs parses the flat [channel, count, …] response from
// PUBSUB NUMSUB and PUBSUB SHARDNUMSUB.
func parseNumSubPairs(pairs []string) map[string]int64 {
	result := make(map[string]int64, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		count, _ := strconv.ParseInt(pairs[i+1], 10, 64)
		result[pairs[i]] = count
	}
	return result
}

// toStreamEntries converts valkey-go XRangeEntry slice to the cache abstraction.
func toStreamEntries(entries []vk.XRangeEntry) []cache.CacheStreamEntry {
	out := make([]cache.CacheStreamEntry, len(entries))
	for i, e := range entries {
		vals := make(map[string]any, len(e.FieldValues))
		for k, fv := range e.FieldValues {
			vals[k] = fv
		}
		out[i] = cache.CacheStreamEntry{ID: e.ID, Values: vals}
	}
	return out
}

// ---------------------------------------------------------------------------
// valkeySubscription implements cache.CacheSubscription.
// ---------------------------------------------------------------------------

// valkeySubscription wraps a valkey-go Receive call and exposes messages via
// a buffered Go channel.  The subscription terminates when Close is called or
// when the context passed to the factory is cancelled.
type valkeySubscription struct {
	cancel context.CancelFunc
	ch     chan *cache.CacheMessage
}

// newValkeySubscription starts a Receive goroutine and returns the subscription.
func newValkeySubscription(ctx context.Context, client vk.Client, cmd vk.Completed) *valkeySubscription {
	subCtx, cancel := context.WithCancel(ctx)
	s := &valkeySubscription{
		cancel: cancel,
		ch:     make(chan *cache.CacheMessage, 256),
	}
	go func() {
		defer close(s.ch)
		_ = client.Receive(subCtx, cmd, func(msg vk.PubSubMessage) {
			select {
			case s.ch <- &cache.CacheMessage{
				Pattern: msg.Pattern,
				Channel: msg.Channel,
				Payload: []byte(msg.Message),
			}:
			case <-subCtx.Done():
			}
		})
	}()
	return s
}

func (s *valkeySubscription) C() <-chan *cache.CacheMessage { return s.ch }

// PUnsubscribe terminates the subscription (valkey-go uses context cancellation).
func (s *valkeySubscription) PUnsubscribe(_ context.Context, _ ...string) error {
	s.cancel()
	return nil
}

// Unsubscribe terminates the subscription.
func (s *valkeySubscription) Unsubscribe(_ context.Context, _ ...string) error {
	s.cancel()
	return nil
}

// SUnsubscribe terminates a sharded subscription.
func (s *valkeySubscription) SUnsubscribe(_ context.Context, _ ...string) error {
	s.cancel()
	return nil
}

// Close terminates the subscription and drains the internal context.
func (s *valkeySubscription) Close() error {
	s.cancel()
	return nil
}

// compile-time interface assertions.
var (
	_ cache.CacheClient       = (*ValkeyClient)(nil)
	_ cache.CacheSubscription = (*valkeySubscription)(nil)
)
