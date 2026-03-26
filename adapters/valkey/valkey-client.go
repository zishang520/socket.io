// Package valkey provides a Valkey client wrapper for the Socket.IO Valkey adapter.
// It bridges valkey-go's callback-based Pub/Sub API to the channel-based patterns
// used by the adapter and emitter implementations.
package valkey

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	vk "github.com/valkey-io/valkey-go"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// ErrValkeyPubSubClosed is returned when receiving from a closed ValkeyPubSub.
var ErrValkeyPubSubClosed = errors.New("valkey: pubsub closed")

// ValkeyMessage represents a single Pub/Sub message received from Valkey.
type ValkeyMessage struct {
	// Pattern is non-empty for pattern-subscribed messages (PSUBSCRIBE).
	Pattern string
	// Channel is the channel the message was published on.
	Channel string
	// Payload is the raw message payload.
	Payload string
}

// ValkeyPubSub wraps a valkey-go Pub/Sub subscription into a channel-based interface
// that mirrors the go-redis *PubSub API used by the adapter and emitter.
type ValkeyPubSub struct {
	cancel context.CancelFunc
	ch     chan *ValkeyMessage
	once   sync.Once
}

// ReceiveMessage blocks until a message is available or the context is done.
func (p *ValkeyPubSub) ReceiveMessage(ctx context.Context) (*ValkeyMessage, error) {
	select {
	case msg, ok := <-p.ch:
		if !ok || msg == nil {
			return nil, ErrValkeyPubSubClosed
		}
		return msg, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Close cancels the underlying subscription goroutine.
func (p *ValkeyPubSub) Close() error {
	p.once.Do(func() {
		p.cancel()
	})
	return nil
}

// Unsubscribe cancels this Pub/Sub subscription.
func (p *ValkeyPubSub) Unsubscribe(_ context.Context, _ ...string) error {
	return p.Close()
}

// PUnsubscribe cancels this pattern Pub/Sub subscription.
func (p *ValkeyPubSub) PUnsubscribe(_ context.Context, _ ...string) error {
	return p.Close()
}

// SUnsubscribe cancels this sharded Pub/Sub subscription.
func (p *ValkeyPubSub) SUnsubscribe(_ context.Context, _ ...string) error {
	return p.Close()
}

// ValkeyClient wraps a valkey-go client and provides context management
// and event emitting capabilities for the Socket.IO Valkey adapter.
type ValkeyClient struct {
	types.EventEmitter

	// Client is the underlying Valkey client.
	Client vk.Client

	// Context is the context used for Valkey operations.
	Context context.Context
}

// NewValkeyClient creates a new ValkeyClient with the given context and valkey-go client.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of Valkey operations.
//     When canceled, all subscriptions and pending operations will be terminated.
//   - client: A valkey-go Client instance.
//
// Example:
//
//	client, _ := valkey.NewClient(valkey.ClientOption{InitAddress: []string{"localhost:6379"}})
//	valkeyClient := NewValkeyClient(context.Background(), client)
func NewValkeyClient(ctx context.Context, client vk.Client) *ValkeyClient {
	if ctx == nil {
		ctx = context.Background()
	}
	return &ValkeyClient{
		EventEmitter: types.NewEventEmitter(),
		Client:       client,
		Context:      ctx,
	}
}

// Subscribe creates a channel-subscription on one or more Valkey channels.
// The returned ValkeyPubSub delivers messages via ReceiveMessage.
func (c *ValkeyClient) Subscribe(ctx context.Context, channels ...string) *ValkeyPubSub {
	subCtx, cancel := context.WithCancel(ctx)
	p := &ValkeyPubSub{cancel: cancel, ch: make(chan *ValkeyMessage, 64)}

	go func() {
		defer close(p.ch)
		_ = c.Client.Receive(subCtx,
			c.Client.B().Subscribe().Channel(channels...).Build(),
			func(msg vk.PubSubMessage) {
				select {
				case p.ch <- &ValkeyMessage{Channel: msg.Channel, Payload: msg.Message}:
				case <-subCtx.Done():
				}
			})
	}()

	return p
}

// PSubscribe creates a pattern-subscription on one or more Valkey patterns.
// The returned ValkeyPubSub delivers messages with Pattern and Channel set.
func (c *ValkeyClient) PSubscribe(ctx context.Context, patterns ...string) *ValkeyPubSub {
	subCtx, cancel := context.WithCancel(ctx)
	p := &ValkeyPubSub{cancel: cancel, ch: make(chan *ValkeyMessage, 64)}

	go func() {
		defer close(p.ch)
		_ = c.Client.Receive(subCtx,
			c.Client.B().Psubscribe().Pattern(patterns...).Build(),
			func(msg vk.PubSubMessage) {
				select {
				case p.ch <- &ValkeyMessage{Pattern: msg.Pattern, Channel: msg.Channel, Payload: msg.Message}:
				case <-subCtx.Done():
				}
			})
	}()

	return p
}

// SSubscribe creates a sharded Pub/Sub subscription on one or more channels (SSUBSCRIBE).
// The returned ValkeyPubSub delivers messages via ReceiveMessage.
func (c *ValkeyClient) SSubscribe(ctx context.Context, channels ...string) *ValkeyPubSub {
	subCtx, cancel := context.WithCancel(ctx)
	p := &ValkeyPubSub{cancel: cancel, ch: make(chan *ValkeyMessage, 64)}

	go func() {
		defer close(p.ch)
		_ = c.Client.Receive(subCtx,
			c.Client.B().Ssubscribe().Channel(channels...).Build(),
			func(msg vk.PubSubMessage) {
				select {
				case p.ch <- &ValkeyMessage{Channel: msg.Channel, Payload: msg.Message}:
				case <-subCtx.Done():
				}
			})
	}()

	return p
}

// Publish publishes a message to a Valkey channel.
func (c *ValkeyClient) Publish(ctx context.Context, channel string, message []byte) error {
	return c.Client.Do(ctx,
		c.Client.B().Publish().Channel(channel).Message(string(message)).Build(),
	).Error()
}

// SPublish publishes a message to a sharded Valkey channel (SPUBLISH).
func (c *ValkeyClient) SPublish(ctx context.Context, channel string, message []byte) error {
	return c.Client.Do(ctx,
		c.Client.B().Spublish().Channel(channel).Message(string(message)).Build(),
	).Error()
}

// PubSubNumSub returns the number of subscribers for each channel using PUBSUB NUMSUB.
func (c *ValkeyClient) PubSubNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	resp, err := c.Client.Do(ctx,
		c.Client.B().PubsubNumsub().Channel(channels...).Build(),
	).AsIntMap()
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// PubSubShardNumSub returns the subscriber count for sharded channels using PUBSUB SHARDNUMSUB.
func (c *ValkeyClient) PubSubShardNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	resp, err := c.Client.Do(ctx,
		c.Client.B().PubsubShardnumsub().Channel(channels...).Build(),
	).AsIntMap()
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// XAdd appends a message to a Valkey stream and returns the generated entry ID.
// maxLen is used with the approximate trimming operator (~) for performance.
func (c *ValkeyClient) XAdd(ctx context.Context, stream string, maxLen int64, values map[string]any) (string, error) {
	// Build XADD using the Arbitrary command to support variable field-value pairs.
	// The fixed header is built first; field-value pairs are appended in the loop.
	// Intentionally not pre-allocating with 6+2*len(values) to avoid an integer
	// overflow in the allocation size (CodeQL: size-computation-overflow).
	//nolint:prealloc
	args := []string{"XADD", stream, "MAXLEN", "~", strconv.FormatInt(maxLen, 10), "*"}
	for k, v := range values {
		args = append(args, k, anyToString(v))
	}
	return c.Client.Do(ctx,
		c.Client.B().Arbitrary(args[0]).Args(args[1:]...).Build(),
	).ToString()
}

// XRead reads messages from one or more Valkey streams, blocking up to the given duration.
// Returns entries from the first stream that has data, or nil if the timeout is reached.
func (c *ValkeyClient) XRead(ctx context.Context, streams []string, id string, count int64, block time.Duration) ([]vk.XRangeEntry, error) {
	keys := streams
	ids := make([]string, len(streams))
	for i := range ids {
		ids[i] = id
	}

	result, err := c.Client.Do(ctx,
		c.Client.B().Xread().Count(count).Block(block.Milliseconds()).Streams().Key(keys...).Id(ids...).Build(),
	).AsXRead()
	if err != nil {
		if vk.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	for _, entries := range result {
		return entries, nil
	}
	return nil, nil
}

// XRange reads a range of entries from a Valkey stream.
func (c *ValkeyClient) XRange(ctx context.Context, stream, start, stop string) ([]vk.XRangeEntry, error) {
	return c.Client.Do(ctx,
		c.Client.B().Xrange().Key(stream).Start(start).End(stop).Build(),
	).AsXRange()
}

// XRangeN reads a limited range of entries from a Valkey stream.
func (c *ValkeyClient) XRangeN(ctx context.Context, stream, start, stop string, count int64) ([]vk.XRangeEntry, error) {
	return c.Client.Do(ctx,
		c.Client.B().Xrange().Key(stream).Start(start).End(stop).Count(count).Build(),
	).AsXRange()
}

// Set stores a string value at key with an expiry duration.
func (c *ValkeyClient) Set(ctx context.Context, key, value string, expiry time.Duration) error {
	return c.Client.Do(ctx,
		c.Client.B().Set().Key(key).Value(value).ExSeconds(int64(expiry.Seconds())).Build(),
	).Error()
}

// GetDel atomically gets and deletes a key. Returns ("", nil) if the key does not exist.
func (c *ValkeyClient) GetDel(ctx context.Context, key string) (string, error) {
	val, err := c.Client.Do(ctx,
		c.Client.B().Getdel().Key(key).Build(),
	).ToString()
	if err != nil {
		if vk.IsValkeyNil(err) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}

// anyToString converts a value to its string representation for use in Valkey commands.
func anyToString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return ""
	}
}
