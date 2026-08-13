// Package adapter implements a Redis sharded Pub/Sub adapter for Socket.IO clustering.
//
// This adapter uses Redis 7.0 sharded Pub/Sub, which distributes channels across
// Redis Cluster slots for improved horizontal scalability.
//
// See: https://redis.io/docs/manual/pubsub/#sharded-pubsub
package adapter

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/slices"
)

// ShardedRedisAdapterBuilder creates sharded Redis adapters for Socket.IO namespaces.
type ShardedRedisAdapterBuilder struct {
	// Redis is the Redis client used for sharded Pub/Sub communication.
	Redis *redis.RedisClient
	// Opts contains configuration options for the adapter.
	Opts ShardedRedisAdapterOptionsInterface
}

// New creates a new sharded Redis adapter for the given namespace.
// It implements the socket.AdapterBuilder interface.
func (sb *ShardedRedisAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewShardedRedisAdapter(nsp, sb.Redis, sb.Opts)
}

type nodePubSubEntry struct {
	pubSub   *rds.PubSub
	refCount int
}

const dynamicSubscriptionRetryDelay = time.Second

type shardedRedisAdapter struct {
	adapter.ClusterAdapter

	channelPubSub  *rds.PubSub
	responsePubSub *rds.PubSub

	dynamicMu       sync.Mutex
	nodePubSubs     map[string]*nodePubSubEntry
	chanToAddr      map[string]string
	dynamicChannels map[string]struct{}
	recovering      bool
	closed          bool
	closeOnce       sync.Once

	redisClient     *redis.RedisClient
	opts            *ShardedRedisAdapterOptions
	channel         string
	responseChannel string

	ctx    context.Context
	cancel context.CancelFunc
}

// MakeShardedRedisAdapter creates a new uninitialized shardedRedisAdapter.
// Call Construct to complete initialization.
func MakeShardedRedisAdapter() ShardedRedisAdapter {
	c := &shardedRedisAdapter{
		ClusterAdapter:  adapter.MakeClusterAdapter(),
		opts:            DefaultShardedRedisAdapterOptions(),
		nodePubSubs:     make(map[string]*nodePubSubEntry),
		chanToAddr:      make(map[string]string),
		dynamicChannels: make(map[string]struct{}),
	}
	c.Prototype(c)
	return c
}

// NewShardedRedisAdapter creates and fully initializes a sharded Redis adapter.
func NewShardedRedisAdapter(nsp socket.Namespace, redisClient *redis.RedisClient, opts any) ShardedRedisAdapter {
	c := MakeShardedRedisAdapter()
	c.SetRedis(redisClient)
	c.SetOpts(opts)
	c.Construct(nsp)
	return c
}

// SetRedis sets the Redis client for this adapter.
func (s *shardedRedisAdapter) SetRedis(redisClient *redis.RedisClient) {
	s.redisClient = redisClient
}

// SetOpts applies configuration options to this adapter.
// Non-ShardedRedisAdapterOptionsInterface values are silently ignored.
func (s *shardedRedisAdapter) SetOpts(opts any) {
	if options, ok := opts.(ShardedRedisAdapterOptionsInterface); ok {
		s.opts.Assign(options)
	}
}

// Construct initializes the adapter for the given namespace.
// It applies defaults, builds channel names, subscribes to static channels,
// registers dynamic subscription handlers, and starts message-receiving goroutines.
func (s *shardedRedisAdapter) Construct(nsp socket.Namespace) {
	s.ClusterAdapter.Construct(nsp)

	if s.opts.GetRawChannelPrefix() == nil {
		s.opts.SetChannelPrefix(DefaultShardedChannelPrefix)
	}
	if s.opts.GetRawSubscriptionMode() == nil {
		s.opts.SetSubscriptionMode(DefaultShardedSubscriptionMode)
	}

	s.ctx, s.cancel = context.WithCancel(s.redisClient.Context)

	s.channel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	s.responseChannel = s.channel + string(s.Uid()) + "#"

	// Subscribe to static channels using SubClient for read/write separation.
	s.channelPubSub = s.redisClient.Sub().SSubscribe(s.ctx, s.channel)
	s.responsePubSub = s.redisClient.Sub().SSubscribe(s.ctx, s.responseChannel)

	if s.isDynamicMode() {
		s.setupDynamicSubscriptions()
	}

	go s.receiveMessages(s.channelPubSub)
	go s.receiveMessages(s.responsePubSub)
}

// setupDynamicSubscriptions registers create-room and delete-room event handlers
// that subscribe/unsubscribe per-room channels on demand.
func (s *shardedRedisAdapter) setupDynamicSubscriptions() {
	_ = s.On("create-room", func(rooms ...any) {
		room := slices.TryGetAny[socket.Room](rooms, 0)
		if !s.shouldUseASeparateNamespace(room) {
			return
		}
		s.subscribeNode(s.dynamicChannel(room))
	})

	_ = s.On("delete-room", func(rooms ...any) {
		room := slices.TryGetAny[socket.Room](rooms, 0)
		if !s.shouldUseASeparateNamespace(room) {
			return
		}
		s.unsubscribeNode(s.dynamicChannel(room))
	})
}

// subscribeNode subscribes to a dynamic channel. Cluster and Ring clients
// reuse one Pub/Sub connection per Redis node; standalone clients reuse the
// namespace's main subscription.
func (s *shardedRedisAdapter) subscribeNode(channel string) {
	s.dynamicMu.Lock()
	if s.closed || s.ctx == nil || s.ctx.Err() != nil {
		s.dynamicMu.Unlock()
		return
	}
	if _, exists := s.dynamicChannels[channel]; exists {
		s.dynamicMu.Unlock()
		return
	}
	s.dynamicChannels[channel] = struct{}{}
	if s.recovering {
		s.dynamicMu.Unlock()
		return
	}

	switch s.redisClient.Sub().(type) {
	case *rds.ClusterClient, *rds.Ring:
	default:
		err := s.channelPubSub.SSubscribe(s.ctx, channel)
		if err != nil {
			err = errors.Join(err, s.channelPubSub.SUnsubscribe(s.ctx, channel))
			delete(s.dynamicChannels, channel)
		}
		s.dynamicMu.Unlock()
		s.emitError(err)
		return
	}

	entry, created, err := s.subscribeDynamicLocked(channel, s.nodePubSubs, s.chanToAddr)
	if err != nil {
		if entry != nil && !created {
			s.recovering = true
			pubSubs := s.takeDynamicPubSubsLocked()
			closeErrors := s.closePubSubs(pubSubs)
			s.dynamicMu.Unlock()
			s.emitErrors(closeErrors)
			go s.recoverDynamicSubscriptions()
			return
		}
		delete(s.dynamicChannels, channel)
		s.dynamicMu.Unlock()
		s.emitError(fmt.Errorf("subscribeNode: SSubscribe(%q): %w", channel, err))
		return
	}

	s.dynamicMu.Unlock()
	if created {
		go s.receiveDynamicMessages(entry.pubSub)
	}
}

func (s *shardedRedisAdapter) dynamicNode(channel string) (*rds.Client, error) {
	switch client := s.redisClient.Sub().(type) {
	case *rds.ClusterClient:
		nodeClient, err := client.MasterForKey(s.ctx, channel)
		return nodeClient, err
	case *rds.Ring:
		nodeClient, err := client.GetShardClientForKey(channel)
		return nodeClient, err
	default:
		return nil, errors.New("dynamic subscription pooling is unavailable")
	}
}

func (s *shardedRedisAdapter) subscribeDynamicLocked(
	channel string,
	nodePubSubs map[string]*nodePubSubEntry,
	chanToAddr map[string]string,
) (*nodePubSubEntry, bool, error) {
	nodeClient, err := s.dynamicNode(channel)
	if err != nil {
		return nil, false, err
	}

	addr := nodeClient.Options().Addr
	entry, exists := nodePubSubs[addr]
	if !exists {
		entry = &nodePubSubEntry{pubSub: nodeClient.SSubscribe(s.ctx)}
		nodePubSubs[addr] = entry
	}
	if err = entry.pubSub.SSubscribe(s.ctx, channel); err == nil {
		err = s.ctx.Err()
	}
	if err != nil {
		if !exists {
			delete(nodePubSubs, addr)
			err = errors.Join(err, entry.pubSub.Close())
		}
		return entry, !exists, err
	}

	entry.refCount++
	chanToAddr[channel] = addr
	return entry, !exists, nil
}

// unsubscribeNode removes a dynamic channel subscription. The pooled
// connection is closed when its last channel is removed.
func (s *shardedRedisAdapter) unsubscribeNode(channel string) {
	s.dynamicMu.Lock()
	if s.closed {
		s.dynamicMu.Unlock()
		return
	}
	if _, exists := s.dynamicChannels[channel]; !exists {
		s.dynamicMu.Unlock()
		return
	}
	delete(s.dynamicChannels, channel)
	if s.recovering {
		s.dynamicMu.Unlock()
		return
	}

	switch s.redisClient.Sub().(type) {
	case *rds.ClusterClient, *rds.Ring:
	default:
		err := s.channelPubSub.SUnsubscribe(s.ctx, channel)
		s.dynamicMu.Unlock()
		s.emitError(err)
		return
	}

	addr, exists := s.chanToAddr[channel]
	if !exists {
		s.dynamicMu.Unlock()
		return
	}
	delete(s.chanToAddr, channel)

	entry, exists := s.nodePubSubs[addr]
	if !exists {
		s.dynamicMu.Unlock()
		return
	}
	entry.refCount--
	if entry.refCount == 0 {
		delete(s.nodePubSubs, addr)
		err := entry.pubSub.Close()
		s.dynamicMu.Unlock()
		s.emitError(err)
		return
	}

	err := entry.pubSub.SUnsubscribe(s.ctx, channel)
	s.dynamicMu.Unlock()
	if err != nil {
		s.beginDynamicRecovery(entry.pubSub, err)
	}
}

// beginDynamicRecovery replaces the dynamic connection pool after a transport
// error. go-redis resubscribes all shard channels in one command, which Redis
// Cluster rejects when those channels belong to different slots. Rebuilding
// the pool one channel at a time preserves node-level connection reuse.
func (s *shardedRedisAdapter) beginDynamicRecovery(pubSub *rds.PubSub, cause error) {
	s.dynamicMu.Lock()
	if s.closed || s.recovering {
		s.dynamicMu.Unlock()
		return
	}

	current := false
	for _, entry := range s.nodePubSubs {
		if entry.pubSub == pubSub {
			current = true
			break
		}
	}
	if !current {
		s.dynamicMu.Unlock()
		return
	}

	s.recovering = true
	pubSubs := s.takeDynamicPubSubsLocked()
	closeErrors := s.closePubSubs(pubSubs)
	s.dynamicMu.Unlock()
	s.emitErrors(closeErrors)

	var networkError net.Error
	if !rds.HasErrorPrefix(cause, "CROSSSLOT") &&
		!errors.Is(cause, io.EOF) &&
		!errors.Is(cause, io.ErrUnexpectedEOF) &&
		!errors.As(cause, &networkError) {
		s.emitError(cause)
	}
	if _, moved := rds.IsMovedError(cause); moved {
		if client, ok := s.redisClient.Sub().(*rds.ClusterClient); ok {
			client.ReloadState(s.ctx)
		}
	}
	go s.recoverDynamicSubscriptions()
}

func (s *shardedRedisAdapter) recoverDynamicSubscriptions() {
	for {
		timer := time.NewTimer(dynamicSubscriptionRetryDelay)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			s.dynamicMu.Lock()
			s.recovering = false
			s.dynamicMu.Unlock()
			return
		case <-timer.C:
		}

		s.dynamicMu.Lock()
		if s.closed || s.ctx.Err() != nil {
			s.recovering = false
			s.dynamicMu.Unlock()
			return
		}
		if len(s.dynamicChannels) == 0 {
			s.recovering = false
			s.dynamicMu.Unlock()
			return
		}

		nodePubSubs := make(map[string]*nodePubSubEntry)
		chanToAddr := make(map[string]string, len(s.dynamicChannels))
		var recoveryErr error
		for channel := range s.dynamicChannels {
			if _, _, err := s.subscribeDynamicLocked(channel, nodePubSubs, chanToAddr); err != nil {
				recoveryErr = fmt.Errorf("SSubscribe(%q): %w", channel, err)
				break
			}
		}

		if recoveryErr == nil {
			s.nodePubSubs = nodePubSubs
			s.chanToAddr = chanToAddr
			s.recovering = false
			pubSubs := dynamicPubSubs(nodePubSubs)
			s.dynamicMu.Unlock()
			for _, pubSub := range pubSubs {
				go s.receiveDynamicMessages(pubSub)
			}
			return
		}

		closeErrors := s.closePubSubs(dynamicPubSubs(nodePubSubs))
		s.dynamicMu.Unlock()
		s.emitError(fmt.Errorf("recover dynamic subscriptions: %w", recoveryErr))
		s.emitErrors(closeErrors)
	}
}

// Close cancels all subscriptions and closes every Pub/Sub connection. It is
// safe to call more than once.
func (s *shardedRedisAdapter) Close() {
	var closeErrors []error
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}

		s.dynamicMu.Lock()
		s.closed = true
		s.recovering = false
		pubSubs := make([]*rds.PubSub, 0, 2+len(s.nodePubSubs))
		pubSubs = append(pubSubs, s.channelPubSub, s.responsePubSub)
		pubSubs = append(pubSubs, s.takeDynamicPubSubsLocked()...)
		s.channelPubSub = nil
		s.responsePubSub = nil
		clear(s.dynamicChannels)
		s.dynamicMu.Unlock()

		closeErrors = s.closePubSubs(pubSubs)
		s.ClusterAdapter.Close()
	})
	for _, err := range closeErrors {
		s.emitError(err)
	}
}

// receiveMessages continuously reads messages from a Pub/Sub connection and
// dispatches them to onRawMessage. It exits when the context is canceled or
// the Pub/Sub is closed.
func (s *shardedRedisAdapter) receiveMessages(pubSub *rds.PubSub) {
	for {
		msg, err := pubSub.ReceiveMessage(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil || errors.Is(err, rds.ErrClosed) {
				return
			}
			s.emitError(err)
			continue
		}
		s.onRawMessage([]byte(msg.Payload), msg.Channel)
	}
}

func (s *shardedRedisAdapter) receiveDynamicMessages(pubSub *rds.PubSub) {
	for {
		msg, err := pubSub.ReceiveMessage(s.ctx)
		if err == nil {
			s.onRawMessage([]byte(msg.Payload), msg.Channel)
			continue
		}
		if s.ctx.Err() != nil || errors.Is(err, rds.ErrClosed) {
			return
		}
		s.beginDynamicRecovery(pubSub, err)
		return
	}
}

func (s *shardedRedisAdapter) closePubSubs(pubSubs []*rds.PubSub) []error {
	var closeErrors []error
	for _, pubSub := range pubSubs {
		if pubSub == nil {
			continue
		}
		if err := pubSub.Close(); err != nil && !errors.Is(err, rds.ErrClosed) {
			closeErrors = append(closeErrors, err)
		}
	}
	return closeErrors
}

func (s *shardedRedisAdapter) takeDynamicPubSubsLocked() []*rds.PubSub {
	pubSubs := dynamicPubSubs(s.nodePubSubs)
	clear(s.nodePubSubs)
	clear(s.chanToAddr)
	return pubSubs
}

func dynamicPubSubs(entries map[string]*nodePubSubEntry) []*rds.PubSub {
	pubSubs := make([]*rds.PubSub, 0, len(entries))
	for _, entry := range entries {
		pubSubs = append(pubSubs, entry.pubSub)
	}
	return pubSubs
}

func (s *shardedRedisAdapter) emitErrors(errs []error) {
	for _, err := range errs {
		s.emitError(err)
	}
}

func (s *shardedRedisAdapter) emitError(err error) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, rds.ErrClosed) {
		return
	}
	if s.redisClient != nil {
		s.redisClient.Emit("error", err)
	}
}

// DoPublish publishes a cluster message to the appropriate Redis sharded channel.
func (s *shardedRedisAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	channel := s.computeChannel(message)
	redisLog.Debug("publishing message of type %v to %s", message.Type, channel)

	msg, err := redis.EncodeClusterMessage(message)
	if err != nil {
		return "", fmt.Errorf("failed to encode message: %w", err)
	}

	return "", s.redisClient.Client.SPublish(s.ctx, channel, msg).Err()
}

// computeChannel returns the Redis channel to publish a message on.
// Broadcast messages targeting a single room may use a room-specific dynamic
// channel; all others use the namespace-level main channel.
func (s *shardedRedisAdapter) computeChannel(message *adapter.ClusterMessage) string {
	if message.Type != adapter.BROADCAST {
		return s.channel
	}

	data, ok := message.Data.(*adapter.BroadcastMessage)
	if !ok || data.RequestId != nil {
		return s.channel
	}

	if len(data.Opts.Rooms) == 1 {
		room := data.Opts.Rooms[0]
		if redis.ShouldUseDynamicChannel(s.opts.SubscriptionMode(), room) {
			return s.dynamicChannel(room)
		}
	}

	return s.channel
}

func (s *shardedRedisAdapter) dynamicChannel(room socket.Room) string {
	return s.channel + string(room) + "#"
}

// DoPublishResponse publishes a response directly to the requester's per-server channel.
func (s *shardedRedisAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	redisLog.Debug("publishing response of type %d to %s", response.Type, requesterUid)

	message, err := redis.EncodeClusterMessage(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}

	return s.redisClient.Client.SPublish(s.ctx, s.channel+string(requesterUid)+"#", message).Err()
}

// onRawMessage decodes an incoming Redis message and routes it to OnResponse
// (for response-channel messages) or OnMessage (for all others).
func (s *shardedRedisAdapter) onRawMessage(rawMessage []byte, channel string) {
	if len(rawMessage) == 0 {
		redisLog.Debug("received empty message")
		return
	}

	message, err := redis.UnmarshalClusterMessage(rawMessage)
	if err != nil {
		redisLog.Debug("invalid message format: %s", err.Error())
		return
	}

	if channel == s.responseChannel {
		s.OnResponse(message)
	} else {
		s.OnMessage(message, "")
	}
}

// ServerCount returns the number of servers currently subscribed to this adapter's
// main channel, as reported by Redis PUBSUBSHARDNUMSUB.
func (s *shardedRedisAdapter) ServerCount() (int64, error) {
	return pubSubNumSub(s.ctx, s.redisClient.Client, true, s.channel)
}

// isDynamicMode reports whether the adapter is configured for dynamic channel subscriptions.
func (s *shardedRedisAdapter) isDynamicMode() bool {
	mode := s.opts.SubscriptionMode()
	return mode == redis.DynamicSubscriptionMode || mode == redis.DynamicPrivateSubscriptionMode
}

// shouldUseASeparateNamespace reports whether a room should get its own dynamic channel.
// In DynamicSubscriptionMode, only public rooms (non-socket-ID rooms) use a separate channel.
// In DynamicPrivateSubscriptionMode, all rooms do.
func (s *shardedRedisAdapter) shouldUseASeparateNamespace(room socket.Room) bool {
	_, isPrivateRoom := s.Sids().Load(socket.SocketId(room))

	switch s.opts.SubscriptionMode() {
	case redis.DynamicSubscriptionMode:
		return !isPrivateRoom
	case redis.DynamicPrivateSubscriptionMode:
		return true
	default:
		return false
	}
}
