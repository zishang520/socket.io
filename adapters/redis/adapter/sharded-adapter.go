// Package adapter implements a Redis sharded Pub/Sub adapter for Socket.IO clustering.
//
// This adapter uses Redis 7.0 sharded Pub/Sub, which distributes channels across
// Redis Cluster slots for improved horizontal scalability.
//
// See: https://redis.io/docs/manual/pubsub/#sharded-pubsub
package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	rds "github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
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

// nodePubSubEntry represents a pooled Pub/Sub connection to a single Redis Cluster master node.
//
// All dynamic channels whose slots map to the same master share one TCP connection.
// mu protects pubSub initialization and refCount updates, preventing duplicate
// SSubscribe calls when multiple goroutines race to subscribe the first channel
// for a given master (double-checked locking pattern).
type nodePubSubEntry struct {
	mu       sync.Mutex
	pubSub   *rds.PubSub
	refCount atomic.Int64 // number of active dynamic channels on this connection
}

type shardedRedisAdapter struct {
	adapter.ClusterAdapter

	// pubSubClients holds the two static Pub/Sub connections: one for the main
	// broadcast channel and one for the per-server response channel.
	pubSubClients *types.Map[string, *rds.PubSub]

	// nodePubSubs holds one nodePubSubEntry per Redis Cluster master node,
	// keyed by node address. Channels whose slots land on the same master
	// share a single TCP connection instead of opening one per channel.
	nodePubSubs *types.Map[string, *nodePubSubEntry]
	// chanToAddr maps each subscribed dynamic channel to its master node address,
	// enabling O(1) lookup in unsubscribeNode.
	chanToAddr *types.Map[string, string]

	// ncDynamicPubSubs holds dynamic channel subscriptions for non-ClusterClient
	// backends (standalone Redis, Redis Ring). Each channel gets its own Pub/Sub.
	ncDynamicPubSubs *types.Map[string, *rds.PubSub]
	// ncDynamicMutexes provides per-channel mutual exclusion for non-cluster
	// subscribe/unsubscribe pairs to prevent duplicate connections.
	ncDynamicMutexes *types.Map[string, *sync.Mutex]

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
		ClusterAdapter:   adapter.MakeClusterAdapter(),
		opts:             DefaultShardedRedisAdapterOptions(),
		pubSubClients:    &types.Map[string, *rds.PubSub]{},
		nodePubSubs:      &types.Map[string, *nodePubSubEntry]{},
		chanToAddr:       &types.Map[string, string]{},
		ncDynamicPubSubs: &types.Map[string, *rds.PubSub]{},
		ncDynamicMutexes: &types.Map[string, *sync.Mutex]{},
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
	s.responseChannel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#" + string(s.Uid()) + "#"

	// Subscribe to static channels using SubClient for read/write separation.
	channelPubSub := s.redisClient.Sub().SSubscribe(s.ctx, s.channel)
	responsePubSub := s.redisClient.Sub().SSubscribe(s.ctx, s.responseChannel)

	s.pubSubClients.Store(s.channel, channelPubSub)
	s.pubSubClients.Store(s.responseChannel, responsePubSub)

	if s.isDynamicMode() {
		s.setupDynamicSubscriptions()
	}

	go s.receiveMessages(channelPubSub)
	go s.receiveMessages(responsePubSub)
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

// subscribeNode subscribes to a dynamic channel, pooling connections by master node.
//
// For ClusterClient backends, all channels that hash to the same master share one
// TCP connection. The first subscriber for a given master opens an SSubscribe
// connection; subsequent subscribers reuse it by issuing additional SSUBSCRIBE
// commands on the same Pub/Sub object (double-checked locking via nodePubSubEntry.mu).
//
// For non-ClusterClient backends, each channel gets its own Pub/Sub, guarded by
// a per-channel mutex stored in ncDynamicMutexes.
func (s *shardedRedisAdapter) subscribeNode(channel string) {
	clusterClient, isCluster := s.redisClient.Sub().(*rds.ClusterClient)
	if !isCluster {
		// Non-cluster path: ensure exactly one SSubscribe per channel.
		mu, _ := s.ncDynamicMutexes.LoadOrStore(channel, &sync.Mutex{})
		mu.Lock()
		defer mu.Unlock()

		if _, exists := s.ncDynamicPubSubs.Load(channel); exists {
			return // idempotency guard
		}
		pubSub := s.redisClient.Sub().SSubscribe(s.ctx, channel)
		s.ncDynamicPubSubs.Store(channel, pubSub)
		go s.receiveMessages(pubSub)
		return
	}

	nodeClient, err := clusterClient.MasterForKey(s.ctx, channel)
	if err != nil {
		s.redisClient.Emit("error", fmt.Errorf("subscribeNode: MasterForKey(%q): %w", channel, err))
		return
	}
	addr := nodeClient.Options().Addr

	// Early idempotency check without acquiring the entry lock.
	if _, alreadyTracked := s.chanToAddr.Load(channel); alreadyTracked {
		return
	}

	// LoadOrStore atomically claims the entry for this master node.
	// The first caller creates it; all concurrent callers receive the same entry.
	entry, _ := s.nodePubSubs.LoadOrStore(addr, &nodePubSubEntry{})

	entry.mu.Lock()
	defer entry.mu.Unlock()

	// Double-check: another goroutine may have finished subscribing between the
	// chanToAddr check above and acquiring entry.mu.
	if entry.pubSub == nil {
		// First goroutine for this master: open one new TCP connection.
		entry.pubSub = nodeClient.SSubscribe(s.ctx, channel)
		go s.receiveMessages(entry.pubSub)
	} else {
		// Subsequent goroutines reuse the existing connection by sending an
		// additional SSUBSCRIBE command on the open socket.
		if err := entry.pubSub.SSubscribe(s.ctx, channel); err != nil {
			s.redisClient.Emit("error", fmt.Errorf("subscribeNode: SSubscribe(%q): %w", channel, err))
			return
		}
	}

	entry.refCount.Add(1)
	s.chanToAddr.Store(channel, addr)
}

// unsubscribeNode removes a dynamic channel subscription.
// When the reference count for a master node reaches zero, its shared Pub/Sub
// connection is closed and the pool entry is deleted.
func (s *shardedRedisAdapter) unsubscribeNode(channel string) {
	if _, isCluster := s.redisClient.Sub().(*rds.ClusterClient); !isCluster {
		// Non-cluster path: unsubscribe under the per-channel mutex.
		if mu, exists := s.ncDynamicMutexes.Load(channel); exists {
			mu.Lock()
			if pubSub, ok := s.ncDynamicPubSubs.LoadAndDelete(channel); ok {
				if err := pubSub.SUnsubscribe(s.ctx, channel); err != nil {
					s.redisClient.Emit("error", err)
				}
				if err := pubSub.Close(); err != nil {
					s.redisClient.Emit("error", err)
				}
			}
			mu.Unlock()
			s.ncDynamicMutexes.Delete(channel)
		}
		return
	}

	addr, ok := s.chanToAddr.LoadAndDelete(channel)
	if !ok {
		return
	}

	entry, exists := s.nodePubSubs.Load(addr)
	if !exists {
		return
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	if entry.pubSub != nil {
		if err := entry.pubSub.SUnsubscribe(s.ctx, channel); err != nil {
			s.redisClient.Emit("error", err)
		}
	}

	// Close and remove the pool entry when no channels remain on this connection.
	if entry.refCount.Add(-1) <= 0 {
		if entry.pubSub != nil {
			if err := entry.pubSub.Close(); err != nil {
				s.redisClient.Emit("error", err)
			}
		}
		s.nodePubSubs.Delete(addr)
	}
}

// Close unsubscribes from all channels and shuts down every Pub/Sub connection.
func (s *shardedRedisAdapter) Close() {
	defer s.cancel()

	s.pubSubClients.Range(func(channel string, pubSub *rds.PubSub) bool {
		if err := pubSub.SUnsubscribe(s.ctx, channel); err != nil {
			s.redisClient.Emit("error", err)
		}
		if err := pubSub.Close(); err != nil {
			s.redisClient.Emit("error", err)
		}
		return true
	})

	s.nodePubSubs.Range(func(addr string, entry *nodePubSubEntry) bool {
		entry.mu.Lock()
		if entry.pubSub != nil {
			if err := entry.pubSub.Close(); err != nil {
				s.redisClient.Emit("error", err)
			}
		}
		entry.mu.Unlock()
		return true
	})

	s.nodePubSubs.Clear()
	s.chanToAddr.Clear()

	s.ncDynamicPubSubs.Range(func(_ string, pubSub *rds.PubSub) bool {
		if err := pubSub.Close(); err != nil {
			s.redisClient.Emit("error", err)
		}
		return true
	})

	s.ncDynamicPubSubs.Clear()
	s.ncDynamicMutexes.Clear()

	s.ClusterAdapter.Close()
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
			s.redisClient.Emit("error", err)
			continue
		}
		s.onRawMessage([]byte(msg.Payload), msg.Channel)
	}
}

// DoPublish publishes a cluster message to the appropriate Redis sharded channel.
func (s *shardedRedisAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	channel := s.computeChannel(message)
	redisLog.Debug("publishing message of type %v to %s", message.Type, channel)

	msg, err := s.encode(message)
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

// dynamicChannel returns the per-room channel name.
// strings.Builder with a pre-sized Grow avoids intermediate allocations.
func (s *shardedRedisAdapter) dynamicChannel(room socket.Room) string {
	roomStr := string(room)
	var b strings.Builder
	b.Grow(len(s.channel) + len(roomStr) + 1)
	b.WriteString(s.channel)
	b.WriteString(roomStr)
	b.WriteByte('#')
	return b.String()
}

// DoPublishResponse publishes a response directly to the requester's per-server channel.
func (s *shardedRedisAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	redisLog.Debug("publishing response of type %d to %s", response.Type, requesterUid)

	message, err := s.encode(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}

	return s.redisClient.Client.SPublish(s.ctx, s.channel+string(requesterUid)+"#", message).Err()
}

// encode serializes a cluster message as JSON or MessagePack.
// MessagePack is used only when the message type may contain binary data and
// the payload actually does; all other messages use JSON.
func (s *shardedRedisAdapter) encode(message *adapter.ClusterMessage) ([]byte, error) {
	switch message.Type {
	case adapter.BROADCAST, adapter.BROADCAST_ACK, adapter.FETCH_SOCKETS_RESPONSE,
		adapter.SERVER_SIDE_EMIT, adapter.SERVER_SIDE_EMIT_RESPONSE:
		if parser.HasBinary(message.Data) {
			return utils.MsgPack().Encode(message)
		}
	}
	return json.Marshal(message)
}

// onRawMessage decodes an incoming Redis message and routes it to OnResponse
// (for response-channel messages) or OnMessage (for all others).
func (s *shardedRedisAdapter) onRawMessage(rawMessage []byte, channel string) {
	if len(rawMessage) == 0 {
		redisLog.Debug("received empty message")
		return
	}

	message, err := s.decodeClusterMessage(rawMessage)
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

// decodeClusterMessage deserializes a raw Redis payload into a ClusterResponse.
// The encoding format is detected from the first byte: '{' indicates JSON,
// anything else is treated as MessagePack.
func (s *shardedRedisAdapter) decodeClusterMessage(rawMessage []byte) (*adapter.ClusterResponse, error) {
	var uid adapter.ServerId
	var nsp string
	var messageType adapter.MessageType
	var rawData any

	// Fast-path format detection by inspecting the first byte.
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

// decodeData unmarshals the raw data field into the concrete type for messageType.
func (s *shardedRedisAdapter) decodeData(messageType adapter.MessageType, rawData any) (any, error) {
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
			return nil, fmt.Errorf("JSON decoding failed: %w", err)
		}
	case msgpack.RawMessage:
		if err := utils.MsgPack().Decode(raw, &target); err != nil {
			return nil, fmt.Errorf("MessagePack decoding failed: %w", err)
		}
	default:
		return nil, errors.New("unsupported data format")
	}

	return target, nil
}

// ServerCount returns the number of servers currently subscribed to this adapter's
// main channel, as reported by Redis PUBSUBSHARDNUMSUB.
func (s *shardedRedisAdapter) ServerCount() int64 {
	result, err := s.redisClient.Client.PubSubShardNumSub(s.ctx, s.channel).Result()
	if err != nil {
		s.redisClient.Emit("error", err)
		return 0
	}

	if count, ok := result[s.channel]; ok {
		return count
	}
	return 0
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
