// Package adapter implements a Redis sharded Pub/Sub adapter for Socket.IO clustering.
//
// This adapter leverages Redis 7.0's sharded Pub/Sub for improved scalability in clustered Redis deployments.
// Sharded Pub/Sub distributes channels across Redis cluster slots, enabling better horizontal scaling.
//
// See: https://redis.io/docs/manual/pubsub/#sharded-pubsub
package adapter

import (
	"encoding/json"
	"errors"
	"fmt"

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

// Private session room ID length used to identify private rooms.
const privateRoomIdLength = 20

// ShardedRedisAdapterBuilder creates sharded Redis adapters for Socket.IO namespaces.
type ShardedRedisAdapterBuilder struct {
	// Redis is the Redis client used for sharded Pub/Sub communication.
	Redis *redis.RedisClient
	// Opts contains configuration options for the sharded adapter.
	Opts ShardedRedisAdapterOptionsInterface
}

// New creates a new sharded Redis adapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (sb *ShardedRedisAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewShardedRedisAdapter(nsp, sb.Redis, sb.Opts)
}

type shardedRedisAdapter struct {
	adapter.ClusterAdapter

	pubSubClients   *types.Map[string, *rds.PubSub] // Channel to PubSub client mapping
	redisClient     *redis.RedisClient
	opts            *ShardedRedisAdapterOptions
	channel         string
	responseChannel string
}

// MakeShardedRedisAdapter creates a new uninitialized shardedRedisAdapter with default options.
// Call Construct() to complete initialization.
func MakeShardedRedisAdapter() ShardedRedisAdapter {
	c := &shardedRedisAdapter{
		ClusterAdapter: adapter.MakeClusterAdapter(),
		opts:           DefaultShardedRedisAdapterOptions(),
		pubSubClients:  &types.Map[string, *rds.PubSub]{},
	}
	c.Prototype(c)
	return c
}

// NewShardedRedisAdapter creates and initializes a new sharded Redis adapter.
// This is the primary constructor for creating sharded Redis adapters.
func NewShardedRedisAdapter(nsp socket.Namespace, redisClient *redis.RedisClient, opts any) ShardedRedisAdapter {
	c := MakeShardedRedisAdapter()
	c.SetRedis(redisClient)
	c.SetOpts(opts)
	c.Construct(nsp)
	return c
}

// SetRedis sets the Redis client for the sharded adapter.
func (s *shardedRedisAdapter) SetRedis(redisClient *redis.RedisClient) {
	s.redisClient = redisClient
}

// SetOpts sets the options for the sharded adapter.
// Accepts ShardedRedisAdapterOptionsInterface; other types are ignored.
func (s *shardedRedisAdapter) SetOpts(opts any) {
	if options, ok := opts.(ShardedRedisAdapterOptionsInterface); ok {
		s.opts.Assign(options)
	}
}

// Construct initializes the sharded adapter for the given namespace.
// It sets up Redis sharded Pub/Sub subscriptions and starts message handling goroutines.
func (s *shardedRedisAdapter) Construct(nsp socket.Namespace) {
	s.ClusterAdapter.Construct(nsp)

	// Apply default values
	if s.opts.GetRawChannelPrefix() == nil {
		s.opts.SetChannelPrefix(DefaultShardedChannelPrefix)
	}
	if s.opts.GetRawSubscriptionMode() == nil {
		s.opts.SetSubscriptionMode(DefaultShardedSubscriptionMode)
	}

	// Build channel names
	s.channel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	s.responseChannel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#" + string(s.Uid()) + "#"

	// Subscribe to each channel separately to avoid CROSSSLOT errors in Redis Cluster
	// See: https://github.com/zishang520/socket.io/issues/134
	channelPubSub := s.redisClient.Client.SSubscribe(s.redisClient.Context, s.channel)
	responsePubSub := s.redisClient.Client.SSubscribe(s.redisClient.Context, s.responseChannel)
	s.pubSubClients.Store(s.channel, channelPubSub)
	s.pubSubClients.Store(s.responseChannel, responsePubSub)

	// Set up dynamic subscription mode handlers
	if s.opts.SubscriptionMode() == DynamicSubscriptionMode ||
		s.opts.SubscriptionMode() == DynamicPrivateSubscriptionMode {
		s.setupDynamicSubscriptions()
	}

	// Start message receiving goroutines
	go s.receiveMessages(channelPubSub)
	go s.receiveMessages(responsePubSub)
}

// setupDynamicSubscriptions sets up event handlers for dynamic room subscriptions.
func (s *shardedRedisAdapter) setupDynamicSubscriptions() {
	s.On("create-room", func(rooms ...any) {
		room := slices.TryGetAny[socket.Room](rooms, 0)
		if !s.shouldUseASeparateNamespace(room) {
			return
		}

		dynamicChannel := s.dynamicChannel(room)
		dynamicPubSub := s.redisClient.Client.SSubscribe(s.redisClient.Context, dynamicChannel)
		s.pubSubClients.Store(dynamicChannel, dynamicPubSub)
		go s.receiveMessages(dynamicPubSub)
	})

	s.On("delete-room", func(rooms ...any) {
		room := slices.TryGetAny[socket.Room](rooms, 0)
		if !s.shouldUseASeparateNamespace(room) {
			return
		}

		dynamicChannel := s.dynamicChannel(room)
		if pubSub, ok := s.pubSubClients.LoadAndDelete(dynamicChannel); ok {
			if err := pubSub.SUnsubscribe(s.redisClient.Context, dynamicChannel); err != nil {
				s.redisClient.Emit("error", err)
			}
			pubSub.Close()
		}
	})
}

// Close unsubscribes from all channels and closes the Pub/Sub clients.
func (s *shardedRedisAdapter) Close() {
	s.pubSubClients.Range(func(channel string, pubSub *rds.PubSub) bool {
		if err := pubSub.SUnsubscribe(s.redisClient.Context, channel); err != nil {
			s.redisClient.Emit("error", err)
		}
		pubSub.Close()
		return true
	})
}

// receiveMessages continuously receives and processes messages from a Pub/Sub client.
func (s *shardedRedisAdapter) receiveMessages(pubSub *rds.PubSub) {
	for {
		select {
		case <-s.redisClient.Context.Done():
			return
		default:
			msg, err := pubSub.ReceiveMessage(s.redisClient.Context)
			if err != nil {
				s.redisClient.Emit("error", err)
				if errors.Is(err, rds.ErrClosed) {
					return
				}
				continue
			}
			s.onRawMessage([]byte(msg.Payload), msg.Channel)
		}
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

	return "", s.redisClient.Client.SPublish(s.redisClient.Context, channel, msg).Err()
}

// computeChannel determines the correct channel for a given cluster message.
// Broadcast messages may use a room-specific dynamic channel for optimization.
func (s *shardedRedisAdapter) computeChannel(message *adapter.ClusterMessage) string {
	// Non-broadcast messages always use the main channel
	if message.Type != adapter.BROADCAST {
		return s.channel
	}

	data, ok := message.Data.(*adapter.BroadcastMessage)
	if !ok || data.RequestId != nil {
		// Broadcast with ack cannot use dynamic channels because serverCount()
		// returns all servers, not only those where the room exists
		return s.channel
	}

	// Use dynamic channel for single-room broadcasts
	if len(data.Opts.Rooms) == 1 {
		room := data.Opts.Rooms[0]
		if s.shouldUseDynamicChannel(room) {
			return s.dynamicChannel(room)
		}
	}

	return s.channel
}

// shouldUseDynamicChannel determines if a dynamic channel should be used for the given room.
func (s *shardedRedisAdapter) shouldUseDynamicChannel(room socket.Room) bool {
	switch s.opts.SubscriptionMode() {
	case DynamicSubscriptionMode:
		// Private rooms (session IDs) have length of privateRoomIdLength
		return len(string(room)) != privateRoomIdLength
	case DynamicPrivateSubscriptionMode:
		return true
	default:
		return false
	}
}

// dynamicChannel returns the dynamic channel name for a specific room.
func (s *shardedRedisAdapter) dynamicChannel(room socket.Room) string {
	return s.channel + string(room) + "#"
}

// DoPublishResponse publishes a response to a specific requester's channel.
func (s *shardedRedisAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	redisLog.Debug("publishing response of type %d to %s", response.Type, requesterUid)

	message, err := s.encode(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}

	return s.redisClient.Client.SPublish(s.redisClient.Context, s.channel+string(requesterUid)+"#", message).Err()
}

// encode encodes a cluster message using JSON or MessagePack depending on content.
// Binary data is encoded with MessagePack for efficiency.
func (s *shardedRedisAdapter) encode(message *adapter.ClusterMessage) ([]byte, error) {
	mayContainBinary := message.Type == adapter.BROADCAST ||
		message.Type == adapter.BROADCAST_ACK ||
		message.Type == adapter.FETCH_SOCKETS_RESPONSE ||
		message.Type == adapter.SERVER_SIDE_EMIT ||
		message.Type == adapter.SERVER_SIDE_EMIT_RESPONSE

	if mayContainBinary && parser.HasBinary(message.Data) {
		return utils.MsgPack().Encode(message)
	}
	return json.Marshal(message)
}

// onRawMessage handles incoming raw messages from Redis and dispatches them appropriately.
func (s *shardedRedisAdapter) onRawMessage(rawMessage []byte, channel string) {
	if len(rawMessage) == 0 {
		redisLog.Debug("received empty message")
		return
	}

	var message *adapter.ClusterResponse
	var err error

	// Detect message format by first byte
	if rawMessage[0] == '{' {
		message, err = s.decodeClusterMessageJSON(rawMessage)
	} else {
		message, err = s.decodeClusterMessageMsgPack(rawMessage)
	}

	if err != nil {
		redisLog.Debug("invalid message format: %s", err.Error())
		return
	}

	// Route message based on channel
	if channel == s.responseChannel {
		s.OnResponse(message)
	} else {
		s.OnMessage(message, "")
	}
}

// decodeClusterMessageJSON decodes a cluster message from JSON format.
func (s *shardedRedisAdapter) decodeClusterMessageJSON(rawMessage []byte) (*adapter.ClusterResponse, error) {
	var rawMsg struct {
		Uid  adapter.ServerId    `json:"uid,omitempty"`
		Nsp  string              `json:"nsp,omitempty"`
		Type adapter.MessageType `json:"type,omitempty"`
		Data json.RawMessage     `json:"data,omitempty"`
	}

	if err := json.Unmarshal(rawMessage, &rawMsg); err != nil {
		return nil, fmt.Errorf("invalid JSON format: %w", err)
	}

	return s.buildClusterResponse(rawMsg.Uid, rawMsg.Nsp, rawMsg.Type, rawMsg.Data)
}

// decodeClusterMessageMsgPack decodes a cluster message from MessagePack format.
func (s *shardedRedisAdapter) decodeClusterMessageMsgPack(rawMessage []byte) (*adapter.ClusterResponse, error) {
	var rawMsg struct {
		Uid  adapter.ServerId    `msgpack:"uid,omitempty"`
		Nsp  string              `msgpack:"nsp,omitempty"`
		Type adapter.MessageType `msgpack:"type,omitempty"`
		Data msgpack.RawMessage  `msgpack:"data,omitempty"`
	}

	if err := utils.MsgPack().Decode(rawMessage, &rawMsg); err != nil {
		return nil, fmt.Errorf("invalid MessagePack format: %w", err)
	}

	return s.buildClusterResponse(rawMsg.Uid, rawMsg.Nsp, rawMsg.Type, rawMsg.Data)
}

// buildClusterResponse constructs a ClusterResponse from raw data.
func (s *shardedRedisAdapter) buildClusterResponse(uid adapter.ServerId, nsp string, messageType adapter.MessageType, rawData any) (*adapter.ClusterResponse, error) {
	message := &adapter.ClusterResponse{
		Uid:  uid,
		Nsp:  nsp,
		Type: messageType,
	}

	data, err := s.decodeData(messageType, rawData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode data: %w", err)
	}

	message.Data = data
	return message, nil
}

// decodeData decodes the message data based on the message type and format.
func (s *shardedRedisAdapter) decodeData(messageType adapter.MessageType, rawData any) (any, error) {
	// Determine target type based on message type
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

	// Decode based on raw data format
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

// ServerCount returns the number of servers subscribed to the sharded channel.
func (s *shardedRedisAdapter) ServerCount() int64 {
	result, err := s.redisClient.Client.PubSubShardNumSub(s.redisClient.Context, s.channel).Result()
	if err != nil {
		s.redisClient.Emit("error", err)
		return 0
	}

	if count, ok := result[s.channel]; ok {
		return count
	}
	return 0
}

// shouldUseASeparateNamespace determines if a separate namespace should be used for a room.
// This is used in dynamic subscription modes to optimize channel usage.
func (s *shardedRedisAdapter) shouldUseASeparateNamespace(room socket.Room) bool {
	_, isPrivateRoom := s.Sids().Load(socket.SocketId(room))

	switch s.opts.SubscriptionMode() {
	case DynamicSubscriptionMode:
		return !isPrivateRoom
	case DynamicPrivateSubscriptionMode:
		return true
	default:
		return false
	}
}
