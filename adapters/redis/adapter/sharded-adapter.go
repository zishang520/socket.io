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
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

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

// nodePubSubEntry is one pooled Pub/Sub connection and the set of dynamic
// channels currently subscribed on it. Entries are owned exclusively by the
// subscription manager goroutine, so no locking is needed.
type nodePubSubEntry struct {
	pubSub   *rds.PubSub
	channels map[string]struct{}
}

type shardedRedisAdapter struct {
	adapter.ClusterAdapter

	// desiredChannels is the level-triggered desired subscription state. It
	// always contains the two static channels (main broadcast and per-server
	// response); in dynamic mode create-room adds a room channel and
	// delete-room removes it. The subscription manager goroutine reconciles
	// the actual Redis subscriptions to this set, so a delete-room that races
	// an in-flight subscribe can never leak a subscription, and failed
	// subscribes are retried until the channel is subscribed or no longer
	// desired.
	desiredChannels *types.Map[string, bool]
	// reconcileWake nudges the subscription manager after a desired-state change.
	reconcileWake chan struct{}
	// deadPubSubs carries Pub/Sub connections whose receive loop terminated
	// unexpectedly — e.g. go-redis closed the node client after the node
	// disappeared from the cluster topology. The manager forgets their
	// channels so the next reconcile pass re-subscribes them on a live
	// connection; without this, a node that comes back under the same address
	// would never be re-subscribed (no placement change to detect).
	deadPubSubs chan *rds.PubSub
	// topologyStale is set when a MOVED/ASK error is observed on a Pub/Sub
	// connection, asking the manager's next pass to re-resolve channel
	// placement immediately instead of waiting for the periodic tick.
	topologyStale atomic.Bool
	// managerWg tracks the subscription manager goroutine so Close can wait
	// for every Pub/Sub connection to be released.
	managerWg sync.WaitGroup

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
		desiredChannels: &types.Map[string, bool]{},
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
// It applies defaults, builds channel names, records the static channels as
// desired subscriptions, registers dynamic subscription handlers, and starts
// the subscription manager goroutine that owns all Pub/Sub connections.
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

	s.reconcileWake = make(chan struct{}, 1)
	s.deadPubSubs = make(chan *rds.PubSub, 64)

	// Every subscription — the two static channels and any dynamic room
	// channels — is level-triggered desired state reconciled by the
	// subscription manager. Placement is re-resolved periodically, which is
	// what lets subscriptions survive Redis Cluster failovers and slot
	// migrations.
	s.desiredChannels.Store(s.channel, true)
	s.desiredChannels.Store(s.responseChannel, true)

	if s.isDynamicMode() {
		s.setupDynamicSubscriptions()
	}

	s.managerWg.Add(1)
	go s.subscriptionManagerLoop()
}

// setupDynamicSubscriptions registers create-room and delete-room event handlers.
//
// The handlers only record the desired state and nudge the manager: the actual
// SSUBSCRIBE/SUNSUBSCRIBE round-trips happen on the manager goroutine, so Join
// and Leave never block on Redis I/O (subscriptions are established
// asynchronously, typically within a millisecond).
func (s *shardedRedisAdapter) setupDynamicSubscriptions() {
	_ = s.On("create-room", func(rooms ...any) {
		room := slices.TryGetAny[socket.Room](rooms, 0)
		if !s.shouldUseASeparateNamespace(room) {
			return
		}
		if channel := s.dynamicChannel(room); channel != s.channel && channel != s.responseChannel {
			s.desiredChannels.Store(channel, true)
			s.nudgeSubscriptionManager()
		}
	})

	_ = s.On("delete-room", func(rooms ...any) {
		room := slices.TryGetAny[socket.Room](rooms, 0)
		if !s.shouldUseASeparateNamespace(room) {
			return
		}
		// Never drop the static channels, even for a room whose name collides
		// with them (e.g. a room named after this server's uid).
		if channel := s.dynamicChannel(room); channel != s.channel && channel != s.responseChannel {
			s.desiredChannels.Delete(channel)
			s.nudgeSubscriptionManager()
		}
	})
}

// nudgeSubscriptionManager wakes the subscription manager without blocking.
func (s *shardedRedisAdapter) nudgeSubscriptionManager() {
	select {
	case s.reconcileWake <- struct{}{}:
	default:
	}
}

// sSubscriber is the subset of a Redis client needed to open a new sharded
// subscription connection.
type sSubscriber interface {
	SSubscribe(ctx context.Context, channels ...string) *rds.PubSub
}

// resolveNode maps a channel to its connection pool key and the client used
// to open new subscriptions for it:
//
//   - Redis Cluster: the key is the address of the master that owns the
//     channel's slot, so all dynamic channels on one master share a single
//     TCP connection. The placement is re-evaluated on every periodic
//     reconcile, which is what lets subscriptions follow failovers and slot
//     migrations.
//   - Standalone / Sentinel (*rds.Client): a single shared connection carries
//     every dynamic channel — there are no slot constraints.
//   - Anything else (e.g. rds.Ring): one connection per channel, because
//     channel-to-shard routing is internal to the client.
//
// The two static channels always get a dedicated connection so that request/
// response handling is not serialized behind dynamic room traffic.
func (s *shardedRedisAdapter) resolveNode(channel string) (string, sSubscriber, error) {
	static := ""
	if channel == s.channel || channel == s.responseChannel {
		static = "#static#" + channel
	}

	switch client := s.redisClient.Sub().(type) {
	case *rds.ClusterClient:
		nodeClient, err := client.MasterForKey(s.ctx, channel)
		if err != nil {
			return "", nil, err
		}
		return nodeClient.Options().Addr + static, nodeClient, nil
	case *rds.Client:
		return static, client, nil
	default:
		return "#" + channel, client, nil
	}
}

// subscriptionManagerLoop owns every Pub/Sub connection of this adapter. It
// reconciles the actual Redis subscriptions with desiredChannels whenever
// nudged, and on a periodic tick (or after a MOVED error flagged the topology
// as stale) additionally re-resolves channel placement so that subscriptions
// survive Redis Cluster failovers and slot migrations.
//
// Single-goroutine ownership of the connection pool removes the
// subscribe/unsubscribe races that a shared pool would need locks for.
func (s *shardedRedisAdapter) subscriptionManagerLoop() {
	defer s.managerWg.Done()

	entries := map[string]*nodePubSubEntry{} // pool key -> shared connection
	actual := map[string]string{}            // channel -> pool key

	ticker := time.NewTicker(DefaultSubscriptionReconcileInterval)
	defer ticker.Stop()

	// Initial pass: subscribe what is already desired — at minimum the two
	// static channels stored by Construct.
	s.reconcileSubscriptions(entries, actual, false)

	for {
		select {
		case <-s.ctx.Done():
			for _, entry := range entries {
				if err := entry.pubSub.Close(); err != nil {
					s.redisClient.Emit("error", err)
				}
			}
			return
		case <-s.reconcileWake:
			s.reconcileSubscriptions(entries, actual, s.topologyStale.Swap(false))
		case <-ticker.C:
			s.reconcileSubscriptions(entries, actual, true)
		}
	}
}

// reconcileSubscriptions brings the actual subscriptions in line with the
// desired set. With refreshTopology it also re-resolves the placement of every
// subscribed channel against the current cluster topology: a channel whose
// master changed (failover, slot migration) is dropped from its old connection
// and re-subscribed on the node that now owns it.
func (s *shardedRedisAdapter) reconcileSubscriptions(entries map[string]*nodePubSubEntry, actual map[string]string, refreshTopology bool) {
	// Forget connections that died underneath us so their channels are
	// re-subscribed below on a fresh connection. Closing is a no-op for
	// connections that are already closed (node client garbage-collected);
	// for CROSSSLOT casualties it tears down the broken connection.
	for drained := false; !drained; {
		select {
		case dead := <-s.deadPubSubs:
			for key, entry := range entries {
				if entry.pubSub != dead {
					continue
				}
				_ = entry.pubSub.Close()
				for channel := range entry.channels {
					delete(actual, channel)
				}
				delete(entries, key)
			}
		default:
			drained = true
		}
	}

	// Drop subscriptions that are no longer desired.
	for channel, key := range actual {
		if _, ok := s.desiredChannels.Load(channel); !ok {
			s.dropSubscription(entries, actual, channel, key, false)
		}
	}

	if refreshTopology && len(actual) > 0 {
		if clusterClient, ok := s.redisClient.Sub().(*rds.ClusterClient); ok {
			// Force a topology refresh: subscribers may never issue regular
			// commands, so nothing else would invalidate a stale slot map.
			clusterClient.ReloadState(s.ctx)
			for channel, key := range actual {
				newKey, _, err := s.resolveNode(channel)
				if err != nil || newKey == key {
					continue
				}
				// The old node already dropped this subscription when the
				// slot moved (or the node is gone entirely), so skip the
				// SUNSUBSCRIBE round-trip — against a failed-over node it
				// would block on a redial.
				s.dropSubscription(entries, actual, channel, key, true)
			}

			// Re-assert every remaining subscription, one SSUBSCRIBE per
			// channel. This is the authoritative self-heal for server-side
			// subscription loss that produces no client-side signal (e.g. a
			// rejected batch resubscribe): re-subscribing an already-active
			// channel is a no-op for the server, and a channel whose slot
			// moved surfaces as a MOVED error handled by the receive loop.
			for channel, key := range actual {
				if entry, ok := entries[key]; ok {
					if err := entry.pubSub.SSubscribe(s.ctx, channel); err != nil {
						s.redisClient.Emit("error", fmt.Errorf("reconcile: SSubscribe(%q): %w", channel, err))
					}
				}
			}
		}
	}

	// Subscribe desired channels that are missing. Failures leave the channel
	// out of `actual`, so it is retried on the next nudge or tick; failedKeys
	// stops one unreachable node from stalling the pass on every one of its
	// channels.
	failedKeys := map[string]bool{}
	s.desiredChannels.Range(func(channel string, _ bool) bool {
		if _, ok := actual[channel]; !ok {
			s.addSubscription(entries, actual, failedKeys, channel)
		}
		return true
	})
}

// addSubscription subscribes a channel on its node's shared connection,
// opening the connection if it is the first channel for that node.
func (s *shardedRedisAdapter) addSubscription(entries map[string]*nodePubSubEntry, actual map[string]string, failedKeys map[string]bool, channel string) {
	key, client, err := s.resolveNode(channel)
	if err != nil {
		s.redisClient.Emit("error", fmt.Errorf("addSubscription: resolveNode(%q): %w", channel, err))
		return
	}
	if failedKeys[key] {
		return
	}

	entry, ok := entries[key]
	if !ok {
		entry = &nodePubSubEntry{
			pubSub:   client.SSubscribe(s.ctx, channel),
			channels: map[string]struct{}{},
		}
		entries[key] = entry
		go s.receiveMessages(entry.pubSub)
	} else if err := entry.pubSub.SSubscribe(s.ctx, channel); err != nil {
		failedKeys[key] = true
		s.redisClient.Emit("error", fmt.Errorf("addSubscription: SSubscribe(%q): %w", channel, err))
		return
	}

	entry.channels[channel] = struct{}{}
	actual[channel] = key
}

// dropSubscription unsubscribes a channel and closes its node's shared
// connection when no channels remain on it. With serverSideGone the
// SUNSUBSCRIBE round-trip is skipped: the server already removed the
// subscription (slot migration or node failure).
func (s *shardedRedisAdapter) dropSubscription(entries map[string]*nodePubSubEntry, actual map[string]string, channel string, key string, serverSideGone bool) {
	delete(actual, channel)

	entry, ok := entries[key]
	if !ok {
		return
	}
	delete(entry.channels, channel)

	if len(entry.channels) == 0 {
		// Closing the connection unsubscribes everything server-side, so no
		// SUNSUBSCRIBE round-trip (which could block redialing a dead node)
		// is needed.
		if err := entry.pubSub.Close(); err != nil {
			s.redisClient.Emit("error", err)
		}
		delete(entries, key)
		return
	}

	if !serverSideGone {
		if err := entry.pubSub.SUnsubscribe(s.ctx, channel); err != nil {
			s.redisClient.Emit("error", err)
		}
	}
}

// Close shuts down the subscription manager and every Pub/Sub connection it
// owns; closing the connections implicitly unsubscribes all channels.
func (s *shardedRedisAdapter) Close() {
	// Cancel before any connection is closed: a receive loop woken by its
	// connection closing then observes the canceled context and exits quietly.
	s.cancel()
	s.managerWg.Wait()
	s.desiredChannels.Clear()

	s.ClusterAdapter.Close()
}

// isMovedError reports whether err is a Redis Cluster MOVED/ASK redirection,
// which surfaces on a Pub/Sub connection when a subscribed shard channel's
// slot was migrated to another node.
func isMovedError(err error) bool {
	msg := err.Error()
	return strings.HasPrefix(msg, "MOVED ") || strings.HasPrefix(msg, "ASK ")
}

// isCrossSlotError reports whether err is a Redis Cluster CROSSSLOT rejection.
// go-redis re-subscribes all of a connection's channels in a single SSUBSCRIBE
// command after a reconnect; on a pooled connection holding channels from
// several slots that batch is rejected wholesale, leaving every subscription
// on the connection dead server-side. The receive loop treats it as a dead
// connection so the manager rebuilds the subscriptions one channel at a time.
func isCrossSlotError(err error) bool {
	return strings.HasPrefix(err.Error(), "CROSSSLOT ")
}

// receiveMessages continuously reads messages from a Pub/Sub connection and
// dispatches them to onRawMessage. It exits when the context is canceled or
// the Pub/Sub is closed.
func (s *shardedRedisAdapter) receiveMessages(pubSub *rds.PubSub) {
	for {
		msg, err := pubSub.ReceiveMessage(s.ctx)
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			if errors.Is(err, rds.ErrClosed) {
				// Closed outside adapter shutdown: either the manager dropped
				// this connection deliberately (it no longer tracks it), or
				// go-redis closed the node client underneath us — report it
				// so the manager re-subscribes the channels elsewhere.
				select {
				case s.deadPubSubs <- pubSub:
					s.nudgeSubscriptionManager()
				case <-s.ctx.Done():
				}
				return
			}
			switch {
			case errors.Is(err, net.ErrClosed):
				// A read on a connection that was closed underneath us
				// (PubSub.Close or an internal go-redis reconnect) is not
				// actionable and must not end the loop: the next
				// ReceiveMessage call either returns rds.ErrClosed or
				// proceeds on a fresh connection.
			case isCrossSlotError(err):
				// go-redis batch-resubscribed this connection's channels
				// after a reconnect and the cluster rejected the multi-slot
				// batch, so every subscription on it is dead server-side.
				// Hand the connection to the manager to close and rebuild.
				select {
				case s.deadPubSubs <- pubSub:
					s.nudgeSubscriptionManager()
				case <-s.ctx.Done():
				}
				return
			case isMovedError(err):
				// A shard channel on this connection was migrated to another
				// node; have the manager re-resolve placement now instead of
				// waiting for the next periodic tick.
				s.topologyStale.Store(true)
				s.nudgeSubscriptionManager()
			default:
				s.redisClient.Emit("error", err)
			}
			// Pause briefly so a hard-down node (instant dial failures)
			// cannot spin this loop into an error storm.
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
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
	result, err := s.pubSubShardNumSub(s.channel)
	if err != nil {
		s.redisClient.Emit("error", err)
		return 0
	}

	if count, ok := result[s.channel]; ok {
		return count
	}
	return 0
}

// pubSubShardNumSub runs PUBSUB SHARDNUMSUB for the given channel.
//
// Sharded Pub/Sub subscriber counts are tracked only by the shard that owns
// the channel's slot, while go-redis routes key-less commands such as PUBSUB
// to a random cluster node — so on cluster backends the command must be sent
// to the owning master explicitly, otherwise it usually reports 0.
func (s *shardedRedisAdapter) pubSubShardNumSub(channel string) (map[string]int64, error) {
	for _, c := range []rds.UniversalClient{s.redisClient.Sub(), s.redisClient.Client} {
		if clusterClient, ok := c.(*rds.ClusterClient); ok {
			nodeClient, err := clusterClient.MasterForKey(s.ctx, channel)
			if err != nil {
				return nil, fmt.Errorf("pubSubShardNumSub: MasterForKey(%q): %w", channel, err)
			}
			return nodeClient.PubSubShardNumSub(s.ctx, channel).Result()
		}
	}
	return s.redisClient.Client.PubSubShardNumSub(s.ctx, channel).Result()
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
