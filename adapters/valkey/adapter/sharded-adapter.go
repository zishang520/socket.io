// Package adapter implements a Valkey sharded Pub/Sub adapter for Socket.IO clustering.
package adapter

import (
	"context"
	"fmt"
	"sync"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/slices"
)

// ShardedValkeyAdapterBuilder creates sharded Valkey adapters for Socket.IO namespaces.
type ShardedValkeyAdapterBuilder struct {
	Valkey *valkey.ValkeyClient
	Opts   ShardedValkeyAdapterOptionsInterface
}

// New creates a new sharded Valkey adapter for the given namespace.
func (sb *ShardedValkeyAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewShardedValkeyAdapter(nsp, sb.Valkey, sb.Opts)
}

// A valkey-go cluster subscription is bound to the node selected by its first
// keyed command. Each sharded channel therefore owns a fixed subscription.
type shardedValkeyAdapter struct {
	adapter.ClusterAdapter

	valkeyClient *valkey.ValkeyClient
	opts         *ShardedValkeyAdapterOptions
	channel      string

	channelPubSub  *valkey.ValkeyPubSub
	responsePubSub *valkey.ValkeyPubSub
	dynamicPubSubs map[string]*valkey.ValkeyPubSub
	dynamicMu      sync.Mutex

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

func MakeShardedValkeyAdapter() ShardedValkeyAdapter {
	a := &shardedValkeyAdapter{
		ClusterAdapter: adapter.MakeClusterAdapter(),
		opts:           DefaultShardedValkeyAdapterOptions(),
		dynamicPubSubs: make(map[string]*valkey.ValkeyPubSub),
	}
	a.Prototype(a)
	return a
}

func NewShardedValkeyAdapter(nsp socket.Namespace, client *valkey.ValkeyClient, opts any) ShardedValkeyAdapter {
	a := MakeShardedValkeyAdapter()
	a.SetValkey(client)
	a.SetOpts(opts)
	a.Construct(nsp)
	return a
}

func (s *shardedValkeyAdapter) SetValkey(client *valkey.ValkeyClient) {
	s.valkeyClient = client
}

func (s *shardedValkeyAdapter) SetOpts(opts any) {
	if options, ok := opts.(ShardedValkeyAdapterOptionsInterface); ok {
		s.opts.Assign(options)
	}
}

func (s *shardedValkeyAdapter) Construct(nsp socket.Namespace) {
	s.ClusterAdapter.Construct(nsp)
	if s.opts.GetRawChannelPrefix() == nil {
		s.opts.SetChannelPrefix(DefaultShardedChannelPrefix)
	}
	if s.opts.GetRawSubscriptionMode() == nil {
		s.opts.SetSubscriptionMode(DefaultShardedSubscriptionMode)
	}

	s.ctx, s.cancel = context.WithCancel(s.valkeyClient.Context())
	s.channel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	responseChannel := s.channel + string(s.Uid()) + "#"

	if s.isDynamicMode() {
		s.setupDynamicSubscriptions()
	}
	// A subscription error handler may close the adapter before Subscribe returns.
	s.dynamicMu.Lock()
	s.channelPubSub = s.valkeyClient.SSubscribe(s.ctx, s.channel)
	s.responsePubSub = s.valkeyClient.SSubscribe(s.ctx, responseChannel)
	s.dynamicMu.Unlock()
	go s.receiveMessages(s.channelPubSub)
	go s.receiveMessages(s.responsePubSub)

	context.AfterFunc(s.ctx, s.Close)
	if s.ctx.Err() != nil {
		s.Close()
	}
}

func (s *shardedValkeyAdapter) setupDynamicSubscriptions() {
	_ = s.On("create-room", func(args ...any) {
		room := slices.TryGetAny[socket.Room](args, 0)
		if s.shouldUseASeparateNamespace(room) {
			s.subscribeChannel(s.dynamicChannel(room))
		}
	})
	_ = s.On("delete-room", func(args ...any) {
		room := slices.TryGetAny[socket.Room](args, 0)
		if s.shouldUseASeparateNamespace(room) {
			s.unsubscribeChannel(s.dynamicChannel(room))
		}
	})
}

func (s *shardedValkeyAdapter) subscribeChannel(channel string) {
	s.dynamicMu.Lock()
	defer s.dynamicMu.Unlock()
	if s.ctx.Err() != nil {
		return
	}
	if _, exists := s.dynamicPubSubs[channel]; exists {
		return
	}
	pubSub := s.valkeyClient.SSubscribe(s.ctx, channel)
	s.dynamicPubSubs[channel] = pubSub
	go s.receiveMessages(pubSub)
}

func (s *shardedValkeyAdapter) unsubscribeChannel(channel string) {
	s.dynamicMu.Lock()
	pubSub := s.dynamicPubSubs[channel]
	delete(s.dynamicPubSubs, channel)
	s.dynamicMu.Unlock()
	if pubSub != nil {
		_ = pubSub.Close()
	}
}

func (s *shardedValkeyAdapter) Close() {
	s.closeOnce.Do(func() {
		s.ClusterAdapter.Close()
		if s.cancel != nil {
			s.cancel()
		}

		s.dynamicMu.Lock()
		dynamic := make([]*valkey.ValkeyPubSub, 0, len(s.dynamicPubSubs))
		for channel, pubSub := range s.dynamicPubSubs {
			dynamic = append(dynamic, pubSub)
			delete(s.dynamicPubSubs, channel)
		}
		s.dynamicMu.Unlock()

		if s.channelPubSub != nil {
			_ = s.channelPubSub.Close()
		}
		if s.responsePubSub != nil {
			_ = s.responsePubSub.Close()
		}
		for _, pubSub := range dynamic {
			_ = pubSub.Close()
		}
	})
}

func (s *shardedValkeyAdapter) receiveMessages(pubSub *valkey.ValkeyPubSub) {
	for {
		message, err := pubSub.ReceiveMessage(s.ctx)
		if err != nil {
			return
		}
		s.onRawMessage([]byte(message.Message))
	}
}

func (s *shardedValkeyAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	channel := s.computeChannel(message)
	valkeyLog.Debug("publishing message of type %v to %s", message.Type, channel)
	payload, err := adapter.EncodeClusterMessage(message)
	if err != nil {
		return "", fmt.Errorf("failed to encode message: %w", err)
	}
	return "", s.valkeyClient.SPublish(s.valkeyClient.Context(), channel, payload)
}

func (s *shardedValkeyAdapter) computeChannel(message *adapter.ClusterMessage) string {
	if message.Type != adapter.BROADCAST {
		return s.channel
	}
	data, ok := message.Data.(*adapter.BroadcastMessage)
	if !ok || data.RequestId != nil || len(data.Opts.Rooms) != 1 {
		return s.channel
	}
	room := data.Opts.Rooms[0]
	if valkey.ShouldUseDynamicChannel(s.opts.SubscriptionMode(), room) {
		return s.dynamicChannel(room)
	}
	return s.channel
}

func (s *shardedValkeyAdapter) dynamicChannel(room socket.Room) string {
	return s.channel + string(room) + "#"
}

func (s *shardedValkeyAdapter) DoPublishResponse(requester adapter.ServerId, response *adapter.ClusterResponse) error {
	payload, err := adapter.EncodeClusterMessage(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}
	return s.valkeyClient.SPublish(s.valkeyClient.Context(), s.channel+string(requester)+"#", payload)
}

func (s *shardedValkeyAdapter) onRawMessage(raw []byte) {
	if len(raw) == 0 || s.ctx.Err() != nil {
		return
	}
	message, err := adapter.DecodeClusterMessage(raw)
	if err != nil {
		valkeyLog.Debug("invalid message format: %s", err.Error())
		return
	}
	s.OnMessage(message, "")
}

func (s *shardedValkeyAdapter) ServerCount() (int64, error) {
	result, err := s.valkeyClient.PubSubShardNumSub(s.ctx, s.channel)
	if err != nil {
		return 0, err
	}
	return result[s.channel], nil
}

func (s *shardedValkeyAdapter) isDynamicMode() bool {
	mode := s.opts.SubscriptionMode()
	return mode == valkey.DynamicSubscriptionMode || mode == valkey.DynamicPrivateSubscriptionMode
}

func (s *shardedValkeyAdapter) shouldUseASeparateNamespace(room socket.Room) bool {
	if room == socket.Room(s.Uid()) {
		return false
	}
	return valkey.ShouldUseDynamicChannel(s.opts.SubscriptionMode(), room)
}
