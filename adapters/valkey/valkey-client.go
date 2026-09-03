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
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/queue"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	valkeyClientLog = log.NewLog("socket.io-valkey")

	// ErrValkeyClientRequired is returned when no primary Valkey client is provided.
	ErrValkeyClientRequired = errors.New("valkey: client is required")

	// ErrValkeyPubSubClosed is returned when receiving from a closed ValkeyPubSub.
	ErrValkeyPubSubClosed = errors.New("valkey: pubsub closed")
)

const (
	valkeyPubSubRetryDelay         = time.Second
	valkeyPubSubCleanupRetryDelay  = 25 * time.Millisecond
	valkeyPubSubCleanupMaxAttempts = 3
)

// ValkeyPubSub owns one fixed logical subscription. Matching subscriptions on
// the same ValkeyClient share a physical topic subscription while retaining
// independent delivery queues.
type ValkeyPubSub struct {
	ctx      context.Context
	cancel   context.CancelFunc
	ch       chan vk.PubSubMessage
	messages *queue.Queue
	done     chan struct{}
	closeErr error
}

// ReceiveMessage blocks until a valkey-go PubSubMessage is available or the context is done.
func (p *ValkeyPubSub) ReceiveMessage(ctx context.Context) (vk.PubSubMessage, error) {
	select {
	case msg, ok := <-p.ch:
		if !ok {
			return vk.PubSubMessage{}, ErrValkeyPubSubClosed
		}
		return msg, nil
	case <-ctx.Done():
		return vk.PubSubMessage{}, ctx.Err()
	}
}

// Close stops the fixed subscription and waits for message delivery to end.
func (p *ValkeyPubSub) Close() error {
	p.cancel()
	<-p.done
	return p.closeErr
}

func (p *ValkeyPubSub) enqueue(message vk.PubSubMessage) {
	if p.ctx.Err() != nil {
		return
	}
	p.messages.Enqueue(func() {
		select {
		case p.ch <- message:
		case <-p.ctx.Done():
		}
	})
}

type valkeyPubSubKey struct {
	kind  string
	topic string
}

type valkeyPubSubEntry struct {
	hub    *valkeyPubSubHub
	key    valkeyPubSubKey
	ctx    context.Context
	cancel context.CancelFunc

	subscribers map[*ValkeyPubSub]struct{}
	ready       chan struct{}
	done        chan struct{}
	readyOnce   sync.Once
	closeErr    error
}

func (e *valkeyPubSubEntry) command(client vk.CommandClient) vk.Completed {
	switch e.key.kind {
	case "subscribe":
		return client.B().Subscribe().Channel(e.key.topic).Build()
	case "psubscribe":
		return client.B().Psubscribe().Pattern(e.key.topic).Build()
	default:
		return client.B().Ssubscribe().Channel(e.key.topic).Build()
	}
}

func (e *valkeyPubSubEntry) unsubscribeCommand(client vk.CommandClient) vk.Completed {
	switch e.key.kind {
	case "subscribe":
		return client.B().Unsubscribe().Channel(e.key.topic).Build()
	case "psubscribe":
		return client.B().Punsubscribe().Pattern(e.key.topic).Build()
	default:
		return client.B().Sunsubscribe().Channel(e.key.topic).Build()
	}
}

func (e *valkeyPubSubEntry) unsubscribe(client vk.CommandClient) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	for attempt := 1; attempt <= valkeyPubSubCleanupMaxAttempts; attempt++ {
		err := client.Do(ctx, e.unsubscribeCommand(client)).Error()
		valkeyErr, isValkeyErr := vk.IsValkeyErr(err)
		if err == nil || !isValkeyErr || !valkeyErr.IsTryAgain() || attempt == valkeyPubSubCleanupMaxAttempts {
			return err
		}

		timer := time.NewTimer(valkeyPubSubCleanupRetryDelay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		}
	}
	return nil
}

func (e *valkeyPubSubEntry) signalReady() {
	e.readyOnce.Do(func() { close(e.ready) })
}

func (e *valkeyPubSubEntry) run() {
	defer close(e.done)
	defer e.signalReady()

	for e.ctx.Err() == nil {
		receiveCtx := vk.WithOnSubscriptionHook(e.ctx, func(subscription vk.PubSubSubscription) {
			if subscription.Kind != e.key.kind || subscription.Channel != e.key.topic {
				return
			}
			e.signalReady()
		})
		receiveCtx = vk.WithOnReceiveReturnHook(receiveCtx, func(err error, client vk.CommandClient) error {
			if e.ctx.Err() == nil {
				return err
			}
			cleanupErr := e.unsubscribe(client)
			e.closeErr = cleanupErr
			if cleanupErr != nil {
				go e.hub.owner.Emit("error", cleanupErr)
			}
			if err != nil {
				return err
			}
			return cleanupErr
		})
		client := e.hub.owner.Sub()
		err := client.Receive(receiveCtx, e.command(client), func(message vk.PubSubMessage) {
			e.hub.dispatch(e, message)
		})
		if e.ctx.Err() != nil {
			return
		}
		e.signalReady()
		if err == nil {
			continue
		}
		// An error listener may close the adapter and this subscription. Run it
		// outside the receive loop so Close can wait for run to exit.
		go e.hub.owner.Emit("error", err)

		timer := time.NewTimer(valkeyPubSubRetryDelay)
		select {
		case <-e.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

type valkeyPubSubHub struct {
	owner   *ValkeyClient
	mu      sync.RWMutex
	entries map[valkeyPubSubKey]*valkeyPubSubEntry
}

// acquire shares a topic within one ValkeyClient so one unsubscribe cannot
// remove another local receiver from valkey-go's multiplexed connection.
func (h *valkeyPubSubHub) acquire(pubSub *ValkeyPubSub, kind string, topics []string) []*valkeyPubSubEntry {
	entries := make([]*valkeyPubSubEntry, 0, len(topics))
	for _, topic := range topics {
		key := valkeyPubSubKey{kind: kind, topic: topic}
		for {
			h.mu.Lock()
			entry := h.entries[key]
			if entry != nil && entry.ctx.Err() != nil {
				done := entry.done
				h.mu.Unlock()
				<-done

				h.mu.Lock()
				if h.entries[key] == entry {
					delete(h.entries, key)
				}
				h.mu.Unlock()
				continue
			}
			if entry == nil {
				ctx, cancel := context.WithCancel(h.owner.ctx)
				entry = &valkeyPubSubEntry{
					hub:         h,
					key:         key,
					ctx:         ctx,
					cancel:      cancel,
					subscribers: make(map[*ValkeyPubSub]struct{}),
					ready:       make(chan struct{}),
					done:        make(chan struct{}),
				}
				h.entries[key] = entry
				go entry.run()
			}
			if _, exists := entry.subscribers[pubSub]; !exists {
				entry.subscribers[pubSub] = struct{}{}
				entries = append(entries, entry)
			}
			h.mu.Unlock()
			break
		}
	}
	return entries
}

func (h *valkeyPubSubHub) dispatch(entry *valkeyPubSubEntry, message vk.PubSubMessage) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for pubSub := range entry.subscribers {
		pubSub.enqueue(message)
	}
}

func (h *valkeyPubSubHub) release(pubSub *ValkeyPubSub, entries []*valkeyPubSubEntry) error {
	closing := make([]*valkeyPubSubEntry, 0, len(entries))
	h.mu.Lock()
	for _, entry := range entries {
		delete(entry.subscribers, pubSub)
		if len(entry.subscribers) == 0 {
			entry.cancel()
			closing = append(closing, entry)
		}
	}
	h.mu.Unlock()

	var firstErr error
	for _, entry := range closing {
		<-entry.done
		if firstErr == nil {
			firstErr = entry.closeErr
		}
	}

	h.mu.Lock()
	for _, entry := range closing {
		if h.entries[entry.key] == entry {
			delete(h.entries, entry.key)
		}
	}
	h.mu.Unlock()
	return firstErr
}

// ValkeyClient wraps a valkey-go client and provides context management
// and event emitting capabilities for the Socket.IO Valkey adapter.
//
// Client() is the primary-consistent client used for writes and recovery reads.
// Sub() is used for Pub/Sub, blocking XREAD and subscriber counts. Without a
// separate subscription client, both methods return the primary client.
// One wrapper and its Sub() client represent one Socket.IO server and may be
// shared by that server's namespace adapters.
type ValkeyClient struct {
	types.EventEmitter

	client    vk.Client
	subClient vk.Client
	ctx       context.Context
	pubSubs   valkeyPubSubHub
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

func (c *ValkeyClient) onError(...any) {
	if c.ListenerCount("error") == 1 {
		valkeyClientLog.Warning("missing 'error' handler on this Valkey client")
	}
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
//   - client: The primary Valkey client for writes and consistency-sensitive reads.
//   - subClient: The Valkey client for SUBSCRIBE, XREAD, and subscriber counts.
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
	valkeyClient := &ValkeyClient{
		EventEmitter: types.NewEventEmitter(),
		client:       client,
		subClient:    subClient,
		ctx:          ctx,
	}
	valkeyClient.pubSubs.owner = valkeyClient
	valkeyClient.pubSubs.entries = make(map[valkeyPubSubKey]*valkeyPubSubEntry)
	_ = valkeyClient.On("error", valkeyClient.onError)
	return valkeyClient, nil
}

func (c *ValkeyClient) newPubSub(ctx context.Context, kind string, topics []string) *ValkeyPubSub {
	subCtx, cancel := context.WithCancel(ctx)
	stopOwner := context.AfterFunc(c.ctx, cancel)
	p := &ValkeyPubSub{
		ctx:      subCtx,
		cancel:   cancel,
		ch:       make(chan vk.PubSubMessage, 64),
		messages: queue.New(),
		done:     make(chan struct{}),
	}
	if len(topics) == 0 {
		cancel()
		stopOwner()
		p.messages.TryClose()
		close(p.ch)
		close(p.done)
		return p
	}

	entries := c.pubSubs.acquire(p, kind, topics)
	go func() {
		<-subCtx.Done()
		stopOwner()
		p.closeErr = c.pubSubs.release(p, entries)
		p.messages.Close()
		close(p.ch)
		close(p.done)
	}()
	for _, entry := range entries {
		select {
		case <-entry.ready:
		case <-subCtx.Done():
			return p
		}
	}
	return p
}

// Subscribe creates a channel-subscription on one or more Valkey channels.
// The returned ValkeyPubSub delivers messages via ReceiveMessage.
func (c *ValkeyClient) Subscribe(ctx context.Context, channels ...string) *ValkeyPubSub {
	return c.newPubSub(ctx, "subscribe", channels)
}

// PSubscribe creates a pattern-subscription on one or more Valkey patterns.
// The returned ValkeyPubSub delivers messages with Pattern and Channel set.
func (c *ValkeyClient) PSubscribe(ctx context.Context, patterns ...string) *ValkeyPubSub {
	return c.newPubSub(ctx, "psubscribe", patterns)
}

// SSubscribe creates a sharded Pub/Sub subscription on one channel (SSUBSCRIBE).
// The returned ValkeyPubSub delivers messages via ReceiveMessage.
func (c *ValkeyClient) SSubscribe(ctx context.Context, channel string) *ValkeyPubSub {
	return c.newPubSub(ctx, "ssubscribe", []string{channel})
}

// Publish publishes a message to a Valkey channel.
func (c *ValkeyClient) Publish(ctx context.Context, channel string, message []byte) error {
	return c.client.Do(ctx,
		c.client.B().Publish().Channel(channel).Message(vk.BinaryString(message)).Build(),
	).Error()
}

// SPublish publishes a message to a sharded Valkey channel (SPUBLISH).
func (c *ValkeyClient) SPublish(ctx context.Context, channel string, message []byte) error {
	return c.client.Do(ctx,
		c.client.B().Spublish().Channel(channel).Message(vk.BinaryString(message)).Build(),
	).Error()
}

// PubSubNumSub returns the number of subscribers for each channel using PUBSUB NUMSUB.
func (c *ValkeyClient) PubSubNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	counts := make(map[string]int64, len(channels))
	for _, node := range c.Sub().Nodes() {
		response, err := node.Do(ctx,
			node.B().PubsubNumsub().Channel(channels...).Build(),
		).AsIntMap()
		if err != nil {
			return nil, err
		}
		for channel, count := range response {
			counts[channel] += count
		}
	}
	return counts, nil
}

// PubSubShardNumSub returns the subscriber count for sharded channels using PUBSUB SHARDNUMSUB.
func (c *ValkeyClient) PubSubShardNumSub(ctx context.Context, channels ...string) (map[string]int64, error) {
	sub := c.Sub()
	resp, err := sub.Do(ctx,
		sub.B().PubsubShardnumsub().Channel(channels...).Build(),
	).AsIntMap()
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// XAdd appends a message to a Valkey stream and returns the generated entry ID.
// maxLen is used with the approximate trimming operator (~) for performance.
func (c *ValkeyClient) XAdd(ctx context.Context, stream string, message RawClusterMessage, maxLen int64) (string, error) {
	command := c.client.B().Xadd().Key(stream).
		Maxlen().Almost().Threshold(strconv.FormatInt(maxLen, 10)).
		Id("*").FieldValue().
		FieldValue("uid", message.Uid()).
		FieldValue("nsp", message.Nsp()).
		FieldValue("type", message.Type())
	if data := message.Data(); data != "" {
		command = command.FieldValue("data", data)
	}
	return c.client.Do(ctx, command.Build()).ToString()
}

// XRead reads messages from a Valkey stream, blocking up to the given duration.
// Returns nil if the timeout is reached.
// Uses the read client (SubClient if set) for read/write separation.
func (c *ValkeyClient) XRead(ctx context.Context, stream, id string, count int64, block time.Duration) ([]vk.XRangeEntry, error) {
	sub := c.Sub()
	result, err := sub.Do(ctx,
		sub.B().Xread().Count(count).Block(block.Milliseconds()).Streams().Key(stream).Id(id).Build(),
	).AsXRead()
	if err != nil {
		if vk.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	return result[stream], nil
}

// XRangeN reads a limited range of entries from the primary-consistent client.
func (c *ValkeyClient) XRangeN(ctx context.Context, stream, start, stop string, count int64) ([]vk.XRangeEntry, error) {
	return c.client.Do(ctx,
		c.client.B().Xrange().Key(stream).Start(start).End(stop).Count(count).Build(),
	).AsXRange()
}

// XRevRangeN reads a limited reverse range from the primary-consistent client.
func (c *ValkeyClient) XRevRangeN(ctx context.Context, stream, start, stop string, count int64) ([]vk.XRangeEntry, error) {
	return c.client.Do(ctx,
		c.client.B().Xrevrange().Key(stream).End(start).Start(stop).Count(count).Build(),
	).AsXRange()
}

// Set stores a string value at key with an expiry duration.
func (c *ValkeyClient) Set(ctx context.Context, key, value string, expiry time.Duration) error {
	return c.client.Do(ctx,
		c.client.B().Set().Key(key).Value(value).Px(expiry).Build(),
	).Error()
}

// GetDel atomically gets and deletes a key. Returns ("", nil) if the key does not exist.
func (c *ValkeyClient) GetDel(ctx context.Context, key string) (string, error) {
	val, err := c.client.Do(ctx,
		c.client.B().Getdel().Key(key).Build(),
	).ToString()
	if err != nil {
		if vk.IsValkeyNil(err) {
			return "", nil
		}
		return "", err
	}
	return val, nil
}
