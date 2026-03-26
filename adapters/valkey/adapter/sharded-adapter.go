// Package adapter implements a Valkey sharded Pub/Sub adapter for Socket.IO clustering.
//
// This adapter uses Valkey's sharded Pub/Sub for improved horizontal scalability.
// Unlike the Redis sharded adapter, no manual cluster-slot pooling is required
// because the valkey-go client handles cluster routing internally.
package adapter

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// ShardedValkeyAdapterBuilder creates sharded Valkey adapters for Socket.IO namespaces.
type ShardedValkeyAdapterBuilder struct {
	// Valkey is the Valkey client used for sharded Pub/Sub communication.
	Valkey *valkey.ValkeyClient
	// Opts contains configuration options for the adapter.
	Opts ShardedValkeyAdapterOptionsInterface
}

// New creates a new sharded Valkey adapter for the given namespace.
func (sb *ShardedValkeyAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewShardedValkeyAdapter(nsp, sb.Valkey, sb.Opts)
}

type shardedValkeyAdapter struct {
	adapter.ClusterAdapter

	// pubSubClients holds the two static Pub/Sub connections (main channel + response channel).
	pubSubClients *types.Map[string, *valkey.ValkeyPubSub]

	// dynamicPubSubs holds per-room dynamic channel subscriptions.
	// valkey-go handles cluster routing internally, so no per-master pooling is needed.
	dynamicPubSubs *types.Map[string, *valkey.ValkeyPubSub]
	dynamicMutexes *types.Map[string, *sync.Mutex]

	valkeyClient    *valkey.ValkeyClient
	opts            *ShardedValkeyAdapterOptions
	channel         string
	responseChannel string
}

// MakeShardedValkeyAdapter creates a new uninitialized shardedValkeyAdapter.
func MakeShardedValkeyAdapter() ShardedValkeyAdapter {
	c := &shardedValkeyAdapter{
		ClusterAdapter: adapter.MakeClusterAdapter(),
		opts:           DefaultShardedValkeyAdapterOptions(),
		pubSubClients:  &types.Map[string, *valkey.ValkeyPubSub]{},
		dynamicPubSubs: &types.Map[string, *valkey.ValkeyPubSub]{},
		dynamicMutexes: &types.Map[string, *sync.Mutex]{},
	}
	c.Prototype(c)
	return c
}

// NewShardedValkeyAdapter creates and fully initializes a sharded Valkey adapter.
func NewShardedValkeyAdapter(nsp socket.Namespace, valkeyClient *valkey.ValkeyClient, opts any) ShardedValkeyAdapter {
	c := MakeShardedValkeyAdapter()
	c.SetValkey(valkeyClient)
	c.SetOpts(opts)
	c.Construct(nsp)
	return c
}

func (s *shardedValkeyAdapter) SetValkey(valkeyClient *valkey.ValkeyClient) {
	s.valkeyClient = valkeyClient
}

func (s *shardedValkeyAdapter) SetOpts(opts any) {
	if options, ok := opts.(ShardedValkeyAdapterOptionsInterface); ok {
		s.opts.Assign(options)
	}
}

// Construct initializes the adapter for the given namespace.
func (s *shardedValkeyAdapter) Construct(nsp socket.Namespace) {
	s.ClusterAdapter.Construct(nsp)

	if s.opts.GetRawChannelPrefix() == nil {
		s.opts.SetChannelPrefix(DefaultShardedChannelPrefix)
	}
	if s.opts.GetRawSubscriptionMode() == nil {
		s.opts.SetSubscriptionMode(DefaultShardedSubscriptionMode)
	}

	s.channel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	s.responseChannel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#" + string(s.Uid()) + "#"

	channelPubSub := s.valkeyClient.SSubscribe(s.valkeyClient.Context, s.channel)
	responsePubSub := s.valkeyClient.SSubscribe(s.valkeyClient.Context, s.responseChannel)

	s.pubSubClients.Store(s.channel, channelPubSub)
	s.pubSubClients.Store(s.responseChannel, responsePubSub)

	if s.isDynamicMode() {
		s.setupDynamicSubscriptions()
	}

	go s.receiveMessages(channelPubSub)
	go s.receiveMessages(responsePubSub)
}

func (s *shardedValkeyAdapter) setupDynamicSubscriptions() {
	_ = s.On("create-room", func(rooms ...any) {
		room := slices.TryGetAny[socket.Room](rooms, 0)
		if !s.shouldUseASeparateNamespace(room) {
			return
		}
		s.subscribeChannel(s.dynamicChannel(room))
	})

	_ = s.On("delete-room", func(rooms ...any) {
		room := slices.TryGetAny[socket.Room](rooms, 0)
		if !s.shouldUseASeparateNamespace(room) {
			return
		}
		s.unsubscribeChannel(s.dynamicChannel(room))
	})
}

// subscribeChannel subscribes to a single dynamic channel.
// A per-channel mutex prevents duplicate subscriptions.
func (s *shardedValkeyAdapter) subscribeChannel(channel string) {
	mu, _ := s.dynamicMutexes.LoadOrStore(channel, &sync.Mutex{})
	mu.Lock()
	defer mu.Unlock()

	if _, exists := s.dynamicPubSubs.Load(channel); exists {
		return
	}
	pubSub := s.valkeyClient.SSubscribe(s.valkeyClient.Context, channel)
	s.dynamicPubSubs.Store(channel, pubSub)
	go s.receiveMessages(pubSub)
}

// unsubscribeChannel unsubscribes a dynamic channel.
func (s *shardedValkeyAdapter) unsubscribeChannel(channel string) {
	if mu, exists := s.dynamicMutexes.Load(channel); exists {
		mu.Lock()
		if pubSub, ok := s.dynamicPubSubs.LoadAndDelete(channel); ok {
			if err := pubSub.SUnsubscribe(s.valkeyClient.Context, channel); err != nil {
				s.valkeyClient.Emit("error", err)
			}
			if err := pubSub.Close(); err != nil {
				s.valkeyClient.Emit("error", err)
			}
		}
		mu.Unlock()
		s.dynamicMutexes.Delete(channel)
	}
}

// Close unsubscribes from all channels and shuts down every Pub/Sub connection.
func (s *shardedValkeyAdapter) Close() {
	s.pubSubClients.Range(func(channel string, pubSub *valkey.ValkeyPubSub) bool {
		if err := pubSub.SUnsubscribe(s.valkeyClient.Context, channel); err != nil {
			s.valkeyClient.Emit("error", err)
		}
		if err := pubSub.Close(); err != nil {
			s.valkeyClient.Emit("error", err)
		}
		return true
	})

	s.dynamicPubSubs.Range(func(_ string, pubSub *valkey.ValkeyPubSub) bool {
		if err := pubSub.Close(); err != nil {
			s.valkeyClient.Emit("error", err)
		}
		return true
	})

	s.dynamicPubSubs.Clear()
	s.dynamicMutexes.Clear()
}

// receiveMessages continuously reads from a ValkeyPubSub and dispatches to onRawMessage.
func (s *shardedValkeyAdapter) receiveMessages(pubSub *valkey.ValkeyPubSub) {
	for {
		msg, err := pubSub.ReceiveMessage(s.valkeyClient.Context)
		if err != nil {
			if s.valkeyClient.Context.Err() != nil || errors.Is(err, valkey.ErrValkeyPubSubClosed) {
				return
			}
			s.valkeyClient.Emit("error", err)
			continue
		}
		s.onRawMessage([]byte(msg.Payload), msg.Channel)
	}
}

// DoPublish publishes a cluster message to the appropriate Valkey sharded channel.
func (s *shardedValkeyAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	channel := s.computeChannel(message)
	valkeyLog.Debug("publishing message of type %v to %s", message.Type, channel)

	msg, err := s.encode(message)
	if err != nil {
		return "", fmt.Errorf("failed to encode message: %w", err)
	}

	return "", s.valkeyClient.SPublish(s.valkeyClient.Context, channel, msg)
}

func (s *shardedValkeyAdapter) computeChannel(message *adapter.ClusterMessage) string {
	if message.Type != adapter.BROADCAST {
		return s.channel
	}

	data, ok := message.Data.(*adapter.BroadcastMessage)
	if !ok || data.RequestId != nil {
		return s.channel
	}

	if len(data.Opts.Rooms) == 1 {
		room := data.Opts.Rooms[0]
		if valkey.ShouldUseDynamicChannel(s.opts.SubscriptionMode(), room) {
			return s.dynamicChannel(room)
		}
	}

	return s.channel
}

func (s *shardedValkeyAdapter) dynamicChannel(room socket.Room) string {
	roomStr := string(room)
	var b strings.Builder
	b.Grow(len(s.channel) + len(roomStr) + 1)
	b.WriteString(s.channel)
	b.WriteString(roomStr)
	b.WriteByte('#')
	return b.String()
}

// DoPublishResponse publishes a response to the requester's per-server channel.
func (s *shardedValkeyAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	valkeyLog.Debug("publishing response of type %d to %s", response.Type, requesterUid)

	message, err := s.encode(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}

	return s.valkeyClient.SPublish(s.valkeyClient.Context, s.channel+string(requesterUid)+"#", message)
}

// encode serializes a cluster message using JSON or MessagePack.
func (s *shardedValkeyAdapter) encode(message *adapter.ClusterMessage) ([]byte, error) {
	switch message.Type {
	case adapter.BROADCAST, adapter.BROADCAST_ACK, adapter.FETCH_SOCKETS_RESPONSE,
		adapter.SERVER_SIDE_EMIT, adapter.SERVER_SIDE_EMIT_RESPONSE:
		if parser.HasBinary(message.Data) {
			return utils.MsgPack().Encode(message)
		}
	}
	return json.Marshal(message)
}

func (s *shardedValkeyAdapter) onRawMessage(rawMessage []byte, channel string) {
	if len(rawMessage) == 0 {
		valkeyLog.Debug("received empty message")
		return
	}

	message, err := s.decodeClusterMessage(rawMessage)
	if err != nil {
		valkeyLog.Debug("invalid message format: %s", err.Error())
		return
	}

	if channel == s.responseChannel {
		s.OnResponse(message)
	} else {
		s.OnMessage(message, "")
	}
}

func (s *shardedValkeyAdapter) decodeClusterMessage(rawMessage []byte) (*adapter.ClusterResponse, error) {
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

func (s *shardedValkeyAdapter) decodeData(messageType adapter.MessageType, rawData any) (any, error) {
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

// ServerCount returns the number of servers subscribed to this adapter's main channel.
func (s *shardedValkeyAdapter) ServerCount() int64 {
	result, err := s.valkeyClient.PubSubShardNumSub(s.valkeyClient.Context, s.channel)
	if err != nil {
		s.valkeyClient.Emit("error", err)
		return 0
	}

	if count, ok := result[s.channel]; ok {
		return count
	}
	return 0
}

func (s *shardedValkeyAdapter) isDynamicMode() bool {
	mode := s.opts.SubscriptionMode()
	return mode == valkey.DynamicSubscriptionMode || mode == valkey.DynamicPrivateSubscriptionMode
}

func (s *shardedValkeyAdapter) shouldUseASeparateNamespace(room socket.Room) bool {
	_, isPrivateRoom := s.Sids().Load(socket.SocketId(room))

	switch s.opts.SubscriptionMode() {
	case valkey.DynamicSubscriptionMode:
		return !isPrivateRoom
	case valkey.DynamicPrivateSubscriptionMode:
		return true
	default:
		return false
	}
}
