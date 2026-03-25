// Package adapter implements a sharded pub/sub adapter for Socket.IO clustering.
//
// This adapter uses sharded pub/sub (Redis 7+ SSUBSCRIBE / Valkey SSubscribe),
// which distributes channels across cluster slots for improved horizontal scalability.
//
// The implementation is simplified relative to the Redis-only variant: each
// channel subscription is managed independently rather than pooled per cluster
// master node. Backends such as valkey-go already perform cluster-aware routing
// internally, so no additional pooling is required.
package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// ShardedCacheAdapterBuilder creates sharded cache adapters for Socket.IO namespaces.
type ShardedCacheAdapterBuilder struct {
	// Cache is the cache client used for sharded pub/sub communication.
	Cache cache.CacheClient
	// Opts contains configuration options for the adapter.
	Opts ShardedCacheAdapterOptionsInterface
}

// New creates a new sharded cache adapter for the given namespace.
// Implements the socket.AdapterBuilder interface.
func (sb *ShardedCacheAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewShardedCacheAdapter(nsp, sb.Cache, sb.Opts)
}

type shardedCacheAdapter struct {
	adapter.ClusterAdapter

	// staticSubs holds the two fixed subscriptions: main broadcast channel and per-server response channel.
	staticSubs *types.Map[string, cache.CacheSubscription]

	// dynamicSubs holds per-room subscriptions, guarded by dynamicMu.
	dynamicSubs *types.Map[string, cache.CacheSubscription]
	dynamicMu   *types.Map[string, *sync.Mutex]

	cacheClient cache.CacheClient
	opts        *ShardedCacheAdapterOptions
	channel     string
	responseChannel string
}

// MakeShardedCacheAdapter returns an uninitialized shardedCacheAdapter.
func MakeShardedCacheAdapter() ShardedCacheAdapter {
	c := &shardedCacheAdapter{
		ClusterAdapter: adapter.MakeClusterAdapter(),
		opts:           DefaultShardedCacheAdapterOptions(),
		staticSubs:     &types.Map[string, cache.CacheSubscription]{},
		dynamicSubs:    &types.Map[string, cache.CacheSubscription]{},
		dynamicMu:      &types.Map[string, *sync.Mutex]{},
	}
	c.Prototype(c)
	return c
}

// NewShardedCacheAdapter creates and fully initialises a sharded cache adapter.
func NewShardedCacheAdapter(nsp socket.Namespace, client cache.CacheClient, opts any) ShardedCacheAdapter {
	c := MakeShardedCacheAdapter()
	c.SetCache(client)
	c.SetOpts(opts)
	c.Construct(nsp)
	return c
}

// SetCache sets the cache client.
func (s *shardedCacheAdapter) SetCache(client cache.CacheClient) { s.cacheClient = client }

// SetOpts applies options; non-ShardedCacheAdapterOptionsInterface values are ignored.
func (s *shardedCacheAdapter) SetOpts(opts any) {
	if o, ok := opts.(ShardedCacheAdapterOptionsInterface); ok {
		s.opts.Assign(o)
	}
}

// Construct initialises the adapter for the given namespace.
func (s *shardedCacheAdapter) Construct(nsp socket.Namespace) {
	s.ClusterAdapter.Construct(nsp)

	if s.opts.GetRawChannelPrefix() == nil {
		s.opts.SetChannelPrefix(DefaultShardedChannelPrefix)
	}
	if s.opts.GetRawSubscriptionMode() == nil {
		s.opts.SetSubscriptionMode(DefaultShardedSubscriptionMode)
	}

	s.channel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	s.responseChannel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#" + string(s.Uid()) + "#"

	ctx := s.cacheClient.Context()

	channelSub := s.cacheClient.SSubscribe(ctx, s.channel)
	responseSub := s.cacheClient.SSubscribe(ctx, s.responseChannel)

	s.staticSubs.Store(s.channel, channelSub)
	s.staticSubs.Store(s.responseChannel, responseSub)

	if s.isDynamicMode() {
		s.setupDynamicSubscriptions()
	}

	go s.receiveMessages(channelSub)
	go s.receiveMessages(responseSub)
}

func (s *shardedCacheAdapter) isDynamicMode() bool {
	mode := s.opts.SubscriptionMode()
	return mode == cache.DynamicSubscriptionMode || mode == cache.DynamicPrivateSubscriptionMode
}

// setupDynamicSubscriptions registers room create/delete handlers for per-room channels.
func (s *shardedCacheAdapter) setupDynamicSubscriptions() {
	_ = s.On("create-room", func(rooms ...any) {
		room := slices.TryGetAny[socket.Room](rooms, 0)
		if !s.shouldUseASeparateNamespace(room) {
			return
		}
		s.subscribeRoom(s.dynamicChannel(room))
	})

	_ = s.On("delete-room", func(rooms ...any) {
		room := slices.TryGetAny[socket.Room](rooms, 0)
		if !s.shouldUseASeparateNamespace(room) {
			return
		}
		s.unsubscribeRoom(s.dynamicChannel(room))
	})
}

func (s *shardedCacheAdapter) shouldUseASeparateNamespace(room socket.Room) bool {
	return cache.ShouldUseDynamicChannel(s.opts.SubscriptionMode(), room)
}

// subscribeRoom subscribes to a dynamic per-room channel.
func (s *shardedCacheAdapter) subscribeRoom(channel string) {
	mu, _ := s.dynamicMu.LoadOrStore(channel, &sync.Mutex{})
	mu.Lock()
	defer mu.Unlock()

	if _, exists := s.dynamicSubs.Load(channel); exists {
		return
	}

	sub := s.cacheClient.SSubscribe(s.cacheClient.Context(), channel)
	s.dynamicSubs.Store(channel, sub)
	go s.receiveMessages(sub)
}

// unsubscribeRoom removes a dynamic per-room channel subscription.
func (s *shardedCacheAdapter) unsubscribeRoom(channel string) {
	mu, exists := s.dynamicMu.Load(channel)
	if !exists {
		return
	}
	mu.Lock()
	defer mu.Unlock()

	if sub, ok := s.dynamicSubs.LoadAndDelete(channel); ok {
		if err := sub.SUnsubscribe(s.cacheClient.Context(), channel); err != nil {
			s.cacheClient.Emit("error", err)
		}
		if err := sub.Close(); err != nil {
			s.cacheClient.Emit("error", err)
		}
	}
	s.dynamicMu.Delete(channel)
}

// Close unsubscribes from all channels and releases resources.
func (s *shardedCacheAdapter) Close() {
	ctx := s.cacheClient.Context()

	s.staticSubs.Range(func(channel string, sub cache.CacheSubscription) bool {
		if err := sub.SUnsubscribe(ctx, channel); err != nil {
			s.cacheClient.Emit("error", err)
		}
		if err := sub.Close(); err != nil {
			s.cacheClient.Emit("error", err)
		}
		return true
	})
	s.staticSubs.Clear()

	s.dynamicSubs.Range(func(_ string, sub cache.CacheSubscription) bool {
		if err := sub.Close(); err != nil {
			s.cacheClient.Emit("error", err)
		}
		return true
	})
	s.dynamicSubs.Clear()
	s.dynamicMu.Clear()
}

// receiveMessages pumps messages from a CacheSubscription to onRawMessage.
func (s *shardedCacheAdapter) receiveMessages(sub cache.CacheSubscription) {
	ctx := s.cacheClient.Context()
	for {
		select {
		case msg, ok := <-sub.C():
			if !ok {
				return
			}
			s.onRawMessage(msg.Payload, msg.Channel)
		case <-ctx.Done():
			return
		}
	}
}

// DoPublish publishes a cluster message to the appropriate sharded channel.
func (s *shardedCacheAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	channel := s.computeChannel(message)
	cacheLog.Debug("publishing message of type %v to %s", message.Type, channel)

	msg, err := s.encode(message)
	if err != nil {
		return "", fmt.Errorf("failed to encode message: %w", err)
	}

	return "", s.cacheClient.SPublish(s.cacheClient.Context(), channel, msg)
}

// computeChannel selects the channel to publish on: room-specific or namespace-level.
func (s *shardedCacheAdapter) computeChannel(message *adapter.ClusterMessage) string {
	if message.Type != adapter.BROADCAST {
		return s.channel
	}
	data, ok := message.Data.(*adapter.BroadcastMessage)
	if !ok || data.RequestId != nil {
		return s.channel
	}
	if len(data.Opts.Rooms) == 1 {
		room := data.Opts.Rooms[0]
		if cache.ShouldUseDynamicChannel(s.opts.SubscriptionMode(), room) {
			return s.dynamicChannel(room)
		}
	}
	return s.channel
}

func (s *shardedCacheAdapter) dynamicChannel(room socket.Room) string {
	roomStr := string(room)
	var b strings.Builder
	b.Grow(len(s.channel) + len(roomStr) + 1)
	b.WriteString(s.channel)
	b.WriteString(roomStr)
	b.WriteByte('#')
	return b.String()
}

// DoPublishResponse publishes a response to the requester's per-server channel.
func (s *shardedCacheAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	cacheLog.Debug("publishing response of type %d to %s", response.Type, requesterUid)
	message, err := s.encode(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}
	return s.cacheClient.SPublish(s.cacheClient.Context(), s.channel+string(requesterUid)+"#", message)
}

// encode serialises a cluster message using JSON, or MessagePack when binary data is present.
func (s *shardedCacheAdapter) encode(message *adapter.ClusterMessage) ([]byte, error) {
	switch message.Type {
	case adapter.BROADCAST, adapter.BROADCAST_ACK, adapter.FETCH_SOCKETS_RESPONSE,
		adapter.SERVER_SIDE_EMIT, adapter.SERVER_SIDE_EMIT_RESPONSE:
		if parser.HasBinary(message.Data) {
			return utils.MsgPack().Encode(message)
		}
	}
	return json.Marshal(message)
}

// onRawMessage decodes an incoming sharded message and routes it.
func (s *shardedCacheAdapter) onRawMessage(rawMessage []byte, channel string) {
	if len(rawMessage) == 0 {
		cacheLog.Debug("received empty message")
		return
	}

	message, err := s.decodeClusterMessage(rawMessage)
	if err != nil {
		cacheLog.Debug("invalid message format: %s", err.Error())
		return
	}

	if channel == s.responseChannel {
		s.OnResponse(message)
	} else {
		s.OnMessage(message, "")
	}
}

func (s *shardedCacheAdapter) decodeClusterMessage(rawMessage []byte) (*adapter.ClusterResponse, error) {
	var uid adapter.ServerId
	var nsp string
	var messageType adapter.MessageType
	var rawData any

	if rawMessage[0] == '{' {
		var rawMsg struct {
			Uid  adapter.ServerId    `json:"uid,omitempty"`
			Nsp  string              `json:"nsp,omitempty"`
			Type adapter.MessageType `json:"type,omitempty"`
			Data json.RawMessage     `json:"data,omitempty"`
		}
		if err := json.Unmarshal(rawMessage, &rawMsg); err != nil {
			return nil, fmt.Errorf("invalid JSON format: %w", err)
		}
		uid, nsp, messageType, rawData = rawMsg.Uid, rawMsg.Nsp, rawMsg.Type, rawMsg.Data
	} else {
		var rawMsg struct {
			Uid  adapter.ServerId    `msgpack:"uid,omitempty"`
			Nsp  string              `msgpack:"nsp,omitempty"`
			Type adapter.MessageType `msgpack:"type,omitempty"`
			Data msgpack.RawMessage  `msgpack:"data,omitempty"`
		}
		if err := utils.MsgPack().Decode(rawMessage, &rawMsg); err != nil {
			return nil, fmt.Errorf("invalid MessagePack format: %w", err)
		}
		uid, nsp, messageType, rawData = rawMsg.Uid, rawMsg.Nsp, rawMsg.Type, rawMsg.Data
	}

	data, err := s.decodeData(messageType, rawData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode data: %w", err)
	}

	return &adapter.ClusterResponse{
		Uid:  uid,
		Nsp:  nsp,
		Type: messageType,
		Data: data,
	}, nil
}

func (s *shardedCacheAdapter) decodeData(messageType adapter.MessageType, rawData any) (any, error) {
	var target any
	switch messageType {
	case adapter.INITIAL_HEARTBEAT, adapter.HEARTBEAT, adapter.ADAPTER_CLOSE:
		return nil, nil
	case adapter.BROADCAST:
		target = &adapter.BroadcastMessage{}
	case adapter.SOCKETS_JOIN, adapter.SOCKETS_LEAVE:
		target = &adapter.SocketsJoinLeaveMessage{}
	case adapter.DISCONNECT_SOCKETS:
		target = &adapter.DisconnectSocketsMessage{}
	case adapter.FETCH_SOCKETS:
		target = &adapter.FetchSocketsMessage{}
	case adapter.FETCH_SOCKETS_RESPONSE:
		target = &adapter.FetchSocketsResponse{}
	case adapter.SERVER_SIDE_EMIT:
		target = &adapter.ServerSideEmitMessage{}
	case adapter.SERVER_SIDE_EMIT_RESPONSE:
		target = &adapter.ServerSideEmitResponse{}
	case adapter.BROADCAST_CLIENT_COUNT:
		target = &adapter.BroadcastClientCount{}
	case adapter.BROADCAST_ACK:
		target = &adapter.BroadcastAck{}
	default:
		return nil, fmt.Errorf("unknown message type: %v", messageType)
	}

	switch raw := rawData.(type) {
	case json.RawMessage:
		if err := json.Unmarshal(raw, &target); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON data: %w", err)
		}
	case msgpack.RawMessage:
		if err := utils.MsgPack().Decode(raw, &target); err != nil {
			return nil, fmt.Errorf("failed to decode MessagePack data: %w", err)
		}
	default:
		if rawData == nil {
			return nil, nil
		}
		return nil, errors.New("unsupported data format: expected JSON or MessagePack")
	}

	return target, nil
}

// ServerCount returns the number of nodes subscribed to the main channel.
func (s *shardedCacheAdapter) ServerCount() int64 {
	result, err := s.cacheClient.PubSubShardNumSub(s.cacheClient.Context(), s.channel)
	if err != nil {
		s.cacheClient.Emit("error", err)
		return 0
	}
	if count, ok := result[s.channel]; ok {
		return count
	}
	return 0
}
