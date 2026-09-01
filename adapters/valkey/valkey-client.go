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
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	// ErrValkeyClientRequired is returned when no primary Valkey client is provided.
	ErrValkeyClientRequired = errors.New("valkey: client is required")

	// ErrValkeyPubSubClosed is returned when receiving from a closed ValkeyPubSub.
	ErrValkeyPubSubClosed = errors.New("valkey: pubsub closed")
)

// ValkeyMessage represents a single Pub/Sub message received from Valkey.
type ValkeyMessage struct {
	// Pattern is non-empty for pattern-subscribed messages (PSUBSCRIBE).
	Pattern string
	// Channel is the channel the message was published on.
	Channel string
	// Payload is the raw message payload.
	Payload string
}

// ValkeyPubSub wraps a valkey-go dedicated client Pub/Sub subscription into a
// channel-based interface that mirrors the go-redis *PubSub API.
//
// Unlike the previous implementation which used Client.Receive (owning the
// connection and only supporting context cancellation), this uses
// Client.Dedicate + SetPubSubHooks, giving a persistent dedicated connection
// where SUBSCRIBE/UNSUBSCRIBE commands can be issued independently.
type ValkeyPubSub struct {
	dc      vk.DedicatedClient
	release func()
	cancel  context.CancelFunc
	ch      chan *ValkeyMessage
	once    sync.Once
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

// Close releases the dedicated connection and stops message delivery.
func (p *ValkeyPubSub) Close() error {
	p.once.Do(func() {
		p.cancel()
		p.release()
	})
	return nil
}

// Unsubscribe issues an UNSUBSCRIBE command for the given channels on the
// dedicated connection, without tearing down the subscription stream.
func (p *ValkeyPubSub) Unsubscribe(ctx context.Context, channels ...string) error {
	return p.dc.Do(ctx, p.dc.B().Unsubscribe().Channel(channels...).Build()).Error()
}

// PUnsubscribe issues a PUNSUBSCRIBE command for the given patterns on the
// dedicated connection, without tearing down the subscription stream.
func (p *ValkeyPubSub) PUnsubscribe(ctx context.Context, patterns ...string) error {
	return p.dc.Do(ctx, p.dc.B().Punsubscribe().Pattern(patterns...).Build()).Error()
}

// SUnsubscribe issues a SUNSUBSCRIBE command for the given channels on the
// dedicated connection, without tearing down the subscription stream.
func (p *ValkeyPubSub) SUnsubscribe(ctx context.Context, channels ...string) error {
	return p.dc.Do(ctx, p.dc.B().Sunsubscribe().Channel(channels...).Build()).Error()
}

// ValkeyClient wraps a valkey-go client and provides context management
// and event emitting capabilities for the Socket.IO Valkey adapter.
//
// The client supports read/write separation: Client() is used for write
// operations (PUBLISH, XADD, SET, etc.) and Sub() is used for read/subscribe
// operations (SUBSCRIBE, XREAD, XRANGE, etc.). Without a separate subscription
// client, both methods return the primary client. Its clients and context are
// immutable after construction. The zero value is not usable; create clients
// with NewValkeyClient or NewValkeyClientWithSub.
type ValkeyClient struct {
	types.EventEmitter

	client    vk.Client
	subClient vk.Client
	ctx       context.Context
}

// Client returns the Valkey client used for write operations.
func (c *ValkeyClient) Client() vk.Client {
	return c.client
}

// Sub returns the Valkey client used for read and subscription operations.
func (c *ValkeyClient) Sub() vk.Client {
	return c.subClient
}

// Context returns the context controlling Valkey operations and subscriptions.
func (c *ValkeyClient) Context() context.Context {
	return c.ctx
}

// NewValkeyClient creates a new ValkeyClient with the given context and valkey-go client.
// The same client is used for both read and write operations.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of Valkey operations.
//     When canceled, all subscriptions and pending operations will be terminated.
//   - client: A valkey-go Client instance.
//
// Returns a configured ValkeyClient, or ErrValkeyClientRequired when client is nil.
//
// Example:
//
//	client, _ := valkey.NewClient(valkey.ClientOption{InitAddress: []string{"localhost:6379"}})
//	valkeyClient, err := NewValkeyClient(context.Background(), client)
func NewValkeyClient(ctx context.Context, client vk.Client) (*ValkeyClient, error) {
	return NewValkeyClientWithSub(ctx, client, nil)
}

// NewValkeyClientWithSub creates a new ValkeyClient with separate clients for read/write separation.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of Valkey operations.
//   - client: The Valkey client for write operations (PUBLISH, XADD, SET, etc.)
//     and metadata queries (PUBSUB NUMSUB).
//   - subClient: The Valkey client for read/subscribe operations (SUBSCRIBE, XREAD, etc.).
//     When nil, client is used for both roles.
//
// Returns a configured ValkeyClient, or ErrValkeyClientRequired when client is nil.
//
// Example:
//
//	pubClient, _ := valkey.NewClient(valkey.ClientOption{InitAddress: []string{"master:6379"}})
//	subClient, _ := valkey.NewClient(valkey.ClientOption{InitAddress: []string{"replica:6380"}})
//	valkeyClient, err := NewValkeyClientWithSub(context.Background(), pubClient, subClient)
func NewValkeyClientWithSub(ctx context.Context, client, subClient vk.Client) (*ValkeyClient, error) {
	if utils.IsNil(client) {
		return nil, ErrValkeyClientRequired
	}
	if utils.IsNil(subClient) {
		subClient = client
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return &ValkeyClient{
		EventEmitter: types.NewEventEmitter(),
		client:       client,
		subClient:    subClient,
		ctx:          ctx,
	}, nil
}

// newPubSub creates a ValkeyPubSub backed by a dedicated connection with
// SetPubSubHooks. The transform function converts incoming valkey-go messages
// to ValkeyMessages. The caller must send a subscribe command (SUBSCRIBE,
// PSUBSCRIBE, or SSUBSCRIBE) on the returned ValkeyPubSub's dc after this
// returns; the hooks are already installed and waiting for messages.
func (c *ValkeyClient) newPubSub(ctx context.Context, transform func(vk.PubSubMessage) *ValkeyMessage) *ValkeyPubSub {
	subCtx, cancel := context.WithCancel(ctx)
	dc, release := c.Sub().Dedicate()

	p := &ValkeyPubSub{
		dc:      dc,
		release: release,
		cancel:  cancel,
		ch:      make(chan *ValkeyMessage, 64),
	}

	wait := dc.SetPubSubHooks(vk.PubSubHooks{
		OnMessage: func(msg vk.PubSubMessage) {
			select {
			case p.ch <- transform(msg):
			case <-subCtx.Done():
			}
		},
	})

	go func() {
		defer close(p.ch)
		select {
		case <-wait:
		case <-subCtx.Done():
		}
	}()

	return p
}

// Subscribe creates a channel-subscription on one or more Valkey channels.
// The returned ValkeyPubSub delivers messages via ReceiveMessage.
func (c *ValkeyClient) Subscribe(ctx context.Context, channels ...string) *ValkeyPubSub {
	p := c.newPubSub(ctx, func(msg vk.PubSubMessage) *ValkeyMessage {
		return &ValkeyMessage{Channel: msg.Channel, Payload: msg.Message}
	})
	p.dc.Do(ctx, p.dc.B().Subscribe().Channel(channels...).Build())
	return p
}

// PSubscribe creates a pattern-subscription on one or more Valkey patterns.
// The returned ValkeyPubSub delivers messages with Pattern and Channel set.
func (c *ValkeyClient) PSubscribe(ctx context.Context, patterns ...string) *ValkeyPubSub {
	p := c.newPubSub(ctx, func(msg vk.PubSubMessage) *ValkeyMessage {
		return &ValkeyMessage{Pattern: msg.Pattern, Channel: msg.Channel, Payload: msg.Message}
	})
	p.dc.Do(ctx, p.dc.B().Psubscribe().Pattern(patterns...).Build())
	return p
}

// SSubscribe creates a sharded Pub/Sub subscription on one or more channels (SSUBSCRIBE).
// The returned ValkeyPubSub delivers messages via ReceiveMessage.
func (c *ValkeyClient) SSubscribe(ctx context.Context, channels ...string) *ValkeyPubSub {
	p := c.newPubSub(ctx, func(msg vk.PubSubMessage) *ValkeyMessage {
		return &ValkeyMessage{Channel: msg.Channel, Payload: msg.Message}
	})
	p.dc.Do(ctx, p.dc.B().Ssubscribe().Channel(channels...).Build())
	return p
}

// Publish publishes a message to a Valkey channel.
func (c *ValkeyClient) Publish(ctx context.Context, channel string, message []byte) error {
	return c.client.Do(ctx,
		c.client.B().Publish().Channel(channel).Message(string(message)).Build(),
	).Error()
}

// SPublish publishes a message to a sharded Valkey channel (SPUBLISH).
func (c *ValkeyClient) SPublish(ctx context.Context, channel string, message []byte) error {
	return c.client.Do(ctx,
		c.client.B().Spublish().Channel(channel).Message(string(message)).Build(),
	).Error()
}

// PubSubNumSub returns the number of subscribers for each channel using PUBSUB NUMSUB.
func (c *ValkeyClient) PubSubNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	resp, err := c.client.Do(ctx,
		c.client.B().PubsubNumsub().Channel(channels...).Build(),
	).AsIntMap()
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// PubSubShardNumSub returns the subscriber count for sharded channels using PUBSUB SHARDNUMSUB.
func (c *ValkeyClient) PubSubShardNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	resp, err := c.client.Do(ctx,
		c.client.B().PubsubShardnumsub().Channel(channels...).Build(),
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
	return c.client.Do(ctx,
		c.client.B().Arbitrary(args[0]).Args(args[1:]...).Build(),
	).ToString()
}

// XRead reads messages from one or more Valkey streams, blocking up to the given duration.
// Returns entries from the first stream that has data, or nil if the timeout is reached.
// Uses the read client (SubClient if set) for read/write separation.
func (c *ValkeyClient) XRead(ctx context.Context, streams []string, id string, count int64, block time.Duration) ([]vk.XRangeEntry, error) {
	keys := streams
	ids := make([]string, len(streams))
	for i := range ids {
		ids[i] = id
	}

	sub := c.Sub()
	result, err := sub.Do(ctx,
		sub.B().Xread().Count(count).Block(block.Milliseconds()).Streams().Key(keys...).Id(ids...).Build(),
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
// Uses the read client (SubClient if set) for read/write separation.
func (c *ValkeyClient) XRange(ctx context.Context, stream, start, stop string) ([]vk.XRangeEntry, error) {
	sub := c.Sub()
	return sub.Do(ctx,
		sub.B().Xrange().Key(stream).Start(start).End(stop).Build(),
	).AsXRange()
}

// XRangeN reads a limited range of entries from a Valkey stream.
// Uses the read client (SubClient if set) for read/write separation.
func (c *ValkeyClient) XRangeN(ctx context.Context, stream, start, stop string, count int64) ([]vk.XRangeEntry, error) {
	sub := c.Sub()
	return sub.Do(ctx,
		sub.B().Xrange().Key(stream).Start(start).End(stop).Count(count).Build(),
	).AsXRange()
}

// Set stores a string value at key with an expiry duration.
func (c *ValkeyClient) Set(ctx context.Context, key, value string, expiry time.Duration) error {
	return c.client.Do(ctx,
		c.client.B().Set().Key(key).Value(value).ExSeconds(int64(expiry.Seconds())).Build(),
	).Error()
}

// GetDel atomically gets and deletes a key. Returns ("", nil) if the key does not exist.
// Uses the read client (SubClient if set) for read/write separation.
func (c *ValkeyClient) GetDel(ctx context.Context, key string) (string, error) {
	sub := c.Sub()
	val, err := sub.Do(ctx,
		sub.B().Getdel().Key(key).Build(),
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
