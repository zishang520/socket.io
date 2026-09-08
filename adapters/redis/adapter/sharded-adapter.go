// Package adapter implements Redis adapters for Socket.IO clustering.
package adapter

import (
	"context"
	"fmt"
	"sync"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/slices"
)

// ShardedRedisAdapterBuilder creates adapters that share Redis subscriber
// connections across namespaces.
type ShardedRedisAdapterBuilder struct {
	Redis *redis.RedisClient
	Opts  ShardedRedisAdapterOptionsInterface
}

// New creates a new sharded Redis adapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (sb *ShardedRedisAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewShardedRedisAdapter(nsp, sb.Redis, sb.Opts)
}

type shardedRedisAdapter struct {
	adapter.ClusterAdapter

	redisClient *redis.RedisClient
	opts        *ShardedRedisAdapterOptions
	channel     string

	pubSub       *shardedPubSub
	subscription *shardedSubscription
	server       *socket.Server

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func MakeShardedRedisAdapter() ShardedRedisAdapter {
	a := &shardedRedisAdapter{
		ClusterAdapter: adapter.MakeClusterAdapter(),
		opts:           DefaultShardedRedisAdapterOptions(),
	}
	a.Prototype(a)
	return a
}

func NewShardedRedisAdapter(nsp socket.Namespace, client *redis.RedisClient, opts any) ShardedRedisAdapter {
	a := MakeShardedRedisAdapter()
	a.SetRedis(client)
	a.SetOpts(opts)
	a.Construct(nsp)
	return a
}

func (s *shardedRedisAdapter) SetRedis(client *redis.RedisClient) {
	s.redisClient = client
}

func (s *shardedRedisAdapter) SetOpts(opts any) {
	if options, ok := opts.(ShardedRedisAdapterOptionsInterface); ok {
		s.opts.Assign(options)
	}
}

func (s *shardedRedisAdapter) Construct(nsp socket.Namespace) {
	s.ClusterAdapter.Construct(nsp)
	if s.opts.GetRawChannelPrefix() == nil {
		s.opts.SetChannelPrefix(DefaultShardedChannelPrefix)
	}
	if s.opts.GetRawSubscriptionMode() == nil {
		s.opts.SetSubscriptionMode(DefaultShardedSubscriptionMode)
	}

	s.ctx, s.cancel = context.WithCancel(s.redisClient.Context())
	s.server = nsp.Server()
	s.channel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	responseChannel := s.channel + string(s.Uid()) + "#"

	s.pubSub = acquireShardedPubSub(s.server, s.redisClient)
	s.subscription = s.pubSub.newSubscription(s.onRawMessage)
	if s.isDynamicMode() {
		s.setupDynamicSubscriptions()
	}
	s.subscription.Subscribe(s.channel)
	s.subscription.Subscribe(responseChannel)
	context.AfterFunc(s.ctx, s.Close)
	if s.ctx.Err() != nil {
		s.Close()
		return
	}
	if err := s.pubSub.flush(s.ctx); err != nil && s.ctx.Err() == nil {
		s.redisClient.Emit("error", err)
	}
}

func (s *shardedRedisAdapter) setupDynamicSubscriptions() {
	_ = s.On("create-room", func(args ...any) {
		room := slices.TryGetAny[socket.Room](args, 0)
		if s.shouldUseASeparateNamespace(room) {
			s.subscribeNode(s.dynamicChannel(room))
		}
	})
	_ = s.On("delete-room", func(args ...any) {
		room := slices.TryGetAny[socket.Room](args, 0)
		if s.shouldUseASeparateNamespace(room) {
			s.unsubscribeNode(s.dynamicChannel(room))
		}
	})
}

func (s *shardedRedisAdapter) subscribeNode(channel string) {
	if s.subscription != nil {
		s.subscription.Subscribe(channel)
	}
}

func (s *shardedRedisAdapter) unsubscribeNode(channel string) {
	if s.subscription != nil {
		s.subscription.Unsubscribe(channel)
	}
}

func (s *shardedRedisAdapter) Close() {
	s.closeOnce.Do(func() {
		s.ClusterAdapter.Close()
		if s.cancel != nil {
			s.cancel()
		}
		if s.subscription != nil {
			s.subscription.Close()
		}
		if s.pubSub != nil {
			releaseShardedPubSub(s.server, s.redisClient, s.pubSub)
		}
	})
}

func (s *shardedRedisAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	channel := s.computeChannel(message)
	redisLog.Debug("publishing message of type %v to %s", message.Type, channel)
	payload, err := adapter.EncodeClusterMessage(message)
	if err != nil {
		return "", fmt.Errorf("failed to encode message: %w", err)
	}
	return "", s.redisClient.Client().SPublish(s.redisClient.Context(), channel, payload).Err()
}

func (s *shardedRedisAdapter) computeChannel(message *adapter.ClusterMessage) string {
	if message.Type != adapter.BROADCAST {
		return s.channel
	}
	data, ok := message.Data.(*adapter.BroadcastMessage)
	if !ok || data.RequestId != nil || len(data.Opts.Rooms) != 1 {
		return s.channel
	}
	room := data.Opts.Rooms[0]
	if redis.ShouldUseDynamicChannel(s.opts.SubscriptionMode(), room) {
		return s.dynamicChannel(room)
	}
	return s.channel
}

func (s *shardedRedisAdapter) dynamicChannel(room socket.Room) string {
	return s.channel + string(room) + "#"
}

func (s *shardedRedisAdapter) DoPublishResponse(requester adapter.ServerId, response *adapter.ClusterResponse) error {
	payload, err := adapter.EncodeClusterMessage(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}
	return s.redisClient.Client().SPublish(s.redisClient.Context(), s.channel+string(requester)+"#", payload).Err()
}

func (s *shardedRedisAdapter) onRawMessage(raw []byte, _ string) {
	if len(raw) == 0 || s.ctx != nil && s.ctx.Err() != nil {
		return
	}
	message, err := adapter.DecodeClusterMessage(raw)
	if err != nil {
		redisLog.Debug("invalid message format: %s", err.Error())
		return
	}
	// ClusterAdapter.OnMessage also routes response message types. Dispatching by
	// type avoids a room name colliding with this server's response channel.
	s.OnMessage(message, "")
}

func (s *shardedRedisAdapter) ServerCount() (int64, error) {
	return pubSubNumSub(s.ctx, s.redisClient.Sub(), true, s.channel)
}

func (s *shardedRedisAdapter) isDynamicMode() bool {
	mode := s.opts.SubscriptionMode()
	return mode == redis.DynamicSubscriptionMode || mode == redis.DynamicPrivateSubscriptionMode
}

func (s *shardedRedisAdapter) shouldUseASeparateNamespace(room socket.Room) bool {
	if room == socket.Room(s.Uid()) {
		return false
	}
	return redis.ShouldUseDynamicChannel(s.opts.SubscriptionMode(), room)
}
