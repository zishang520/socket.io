package adapter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/adapters/redis/v3/types"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// Create a new Adapter based on Redis sharded Pub/Sub introduced in Redis 7.0.
//
// See: https://redis.io/docs/manual/pubsub/#sharded-pubsub
type ShardedRedisAdapterBuilder struct {
	// the Redis client used to publish/subscribe
	Redis *types.RedisClient
	// some additional options
	Opts ShardedRedisAdapterOptionsInterface
}

func (sb *ShardedRedisAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewShardedRedisAdapter(nsp, sb.Redis, sb.Opts)
}

type shardedRedisAdapter struct {
	adapter.ClusterAdapter

	pubSubClient    *redis.PubSub
	redisClient     *types.RedisClient
	opts            *ShardedRedisAdapterOptions
	channel         string
	responseChannel string
}

func MakeShardedRedisAdapter() ShardedRedisAdapter {
	c := &shardedRedisAdapter{
		ClusterAdapter: adapter.MakeClusterAdapter(),

		opts: DefaultShardedRedisAdapterOptions(),
	}

	c.Prototype(c)

	return c
}

func NewShardedRedisAdapter(nsp socket.Namespace, redis *types.RedisClient, opts any) ShardedRedisAdapter {
	c := MakeShardedRedisAdapter()

	c.SetRedis(redis)
	c.SetOpts(opts)

	c.Construct(nsp)

	return c
}

func (s *shardedRedisAdapter) SetRedis(redisClient *types.RedisClient) {
	s.redisClient = redisClient
}

func (s *shardedRedisAdapter) SetOpts(opts any) {
	if options, ok := opts.(ShardedRedisAdapterOptionsInterface); ok {
		s.opts.Assign(options)
	}
}

func (s *shardedRedisAdapter) Construct(nsp socket.Namespace) {
	s.ClusterAdapter.Construct(nsp)

	if s.opts.GetRawChannelPrefix() == nil {
		s.opts.SetChannelPrefix("socket.io")
	}

	if s.opts.GetRawSubscriptionMode() == nil {
		s.opts.SetSubscriptionMode(DynamicSubscriptionMode)
	}

	s.channel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	s.responseChannel = s.opts.ChannelPrefix() + "#" + nsp.Name() + "#" + string(s.Uid()) + "#"

	s.pubSubClient = s.redisClient.Client.SSubscribe(s.redisClient.Context, s.channel, s.responseChannel)

	if s.opts.SubscriptionMode() == DynamicSubscriptionMode ||
		s.opts.SubscriptionMode() == DynamicPrivateSubscriptionMode {
		s.On("create-room", func(room ...any) {
			if s.shouldUseASeparateNamespace(room[0].(socket.Room)) {
				if err := s.pubSubClient.SSubscribe(s.redisClient.Context, s.dynamicChannel(room[0].(socket.Room))); err != nil {
					s.redisClient.Emit("error", err)
				}
			}
		})
		s.On("delete-room", func(room ...any) {
			if s.shouldUseASeparateNamespace(room[0].(socket.Room)) {
				if err := s.pubSubClient.SUnsubscribe(s.redisClient.Context, s.dynamicChannel(room[0].(socket.Room))); err != nil {
					s.redisClient.Emit("error", err)
				}
			}
		})
	}

	go func() {
		defer s.pubSubClient.Close()

		for {
			select {
			case <-s.redisClient.Context.Done():
				return
			default:
				msg, err := s.pubSubClient.ReceiveMessage(s.redisClient.Context)
				if err != nil {
					s.redisClient.Emit("error", err)
					if err == redis.ErrClosed {
						return
					}
					continue // retry receiving messages
				}
				s.onRawMessage([]byte(msg.Payload), msg.Channel)
			}
		}
	}()
}

func (s *shardedRedisAdapter) Close() {
	channels := []string{s.channel, s.responseChannel}

	if s.opts.SubscriptionMode() == DynamicSubscriptionMode ||
		s.opts.SubscriptionMode() == DynamicPrivateSubscriptionMode {
		s.Rooms().Range(func(room socket.Room, _sids *types.Set[socket.SocketId]) bool {
			if s.shouldUseASeparateNamespace(room) {
				channels = append(channels, s.dynamicChannel(room))
			}
			return true
		})
	}

	if err := s.pubSubClient.SUnsubscribe(s.redisClient.Context, channels...); err != nil {
		s.redisClient.Emit("error", err)
	}
}

func (s *shardedRedisAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	channel := s.computeChannel(message)
	redis_log.Debug("publishing message of type %v to %s", message.Type, channel)

	msg, err := s.encode(message)
	if err != nil {
		return "", err
	}

	return "", s.redisClient.Client.SPublish(s.redisClient.Context, channel, msg).Err()
}

func (s *shardedRedisAdapter) computeChannel(message *adapter.ClusterMessage) string {
	// broadcast with ack can not use a dynamic channel, because the serverCount() method return the number of all
	// servers, not only the ones where the given room exists
	if message.Type != adapter.BROADCAST {
		return s.channel
	}

	data, ok := message.Data.(*adapter.BroadcastMessage)
	if !ok || data.RequestId != nil {
		return s.channel
	}

	if len(data.Opts.Rooms) == 1 {
		if room := data.Opts.Rooms[0]; (s.opts.SubscriptionMode() == DynamicSubscriptionMode && len(string(room)) != 20) ||
			s.opts.SubscriptionMode() == DynamicPrivateSubscriptionMode {
			return s.dynamicChannel(room)
		}
	}

	return s.channel
}

func (s *shardedRedisAdapter) dynamicChannel(room socket.Room) string {
	return s.channel + string(room) + "#"
}

func (s *shardedRedisAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	redis_log.Debug("publishing response of type %d to %s", response.Type, requesterUid)

	message, err := s.encode(response)
	if err != nil {
		return err
	}
	return s.redisClient.Client.SPublish(s.redisClient.Context, s.channel+string(requesterUid)+"#", message).Err()
}

func (s *shardedRedisAdapter) encode(message *adapter.ClusterMessage) ([]byte, error) {
	mayContainBinary := message.Type == adapter.BROADCAST ||
		message.Type == adapter.BROADCAST_ACK ||
		message.Type == adapter.FETCH_SOCKETS_RESPONSE ||
		message.Type == adapter.SERVER_SIDE_EMIT ||
		message.Type == adapter.SERVER_SIDE_EMIT_RESPONSE
	if mayContainBinary && parser.HasBinary(message.Data) {
		return utils.MsgPack().Encode(message)
	} else {
		return json.Marshal(message)
	}
}

func (s *shardedRedisAdapter) onRawMessage(rawMessage []byte, channel string) {
	// Prepare the structure to hold the decoded message
	var message *adapter.ClusterResponse
	var err error

	// Check the message format based on the first byte
	if rawMessage[0] == '{' { // JSON format
		message, err = s.decodeClusterMessageJSON(rawMessage)
	} else { // MessagePack format
		message, err = s.decodeClusterMessageMsgPack(rawMessage)
	}

	// If an error occurred during decoding, log and exit
	if err != nil {
		redis_log.Debug("invalid message format: %s", err.Error())
		return
	}

	// Handle the message based on the channel type
	if channel == s.responseChannel {
		s.OnResponse(message)
	} else {
		s.OnMessage(message, "")
	}
}

// Decode ClusterMessage in JSON format
func (s *shardedRedisAdapter) decodeClusterMessageJSON(rawMessage []byte) (*adapter.ClusterResponse, error) {
	var rawMsg struct {
		Uid  adapter.ServerId    `json:"uid,omitempty" msgpack:"uid,omitempty"`
		Nsp  string              `json:"nsp,omitempty" msgpack:"nsp,omitempty"`
		Type adapter.MessageType `json:"type,omitempty" msgpack:"type,omitempty"`
		Data json.RawMessage     `json:"data,omitempty" msgpack:"data,omitempty"`
	}

	// Attempt to unmarshal the JSON data
	if err := json.Unmarshal(rawMessage, &rawMsg); err != nil {
		return nil, fmt.Errorf("invalid JSON format: %w", err)
	}

	// Decode the message data and populate the ClusterResponse
	return s.buildClusterResponse(rawMsg.Uid, rawMsg.Nsp, rawMsg.Type, rawMsg.Data)
}

// Decode ClusterMessage in MessagePack format
func (s *shardedRedisAdapter) decodeClusterMessageMsgPack(rawMessage []byte) (*adapter.ClusterResponse, error) {
	var rawMsg struct {
		Uid  adapter.ServerId    `json:"uid,omitempty" msgpack:"uid,omitempty"`
		Nsp  string              `json:"nsp,omitempty" msgpack:"nsp,omitempty"`
		Type adapter.MessageType `json:"type,omitempty" msgpack:"type,omitempty"`
		Data msgpack.RawMessage  `json:"data,omitempty" msgpack:"data,omitempty"`
	}

	// Attempt to decode the MessagePack data
	if err := utils.MsgPack().Decode(rawMessage, &rawMsg); err != nil {
		return nil, fmt.Errorf("invalid MessagePack format: %w", err)
	}

	// Decode the message data and populate the ClusterResponse
	return s.buildClusterResponse(rawMsg.Uid, rawMsg.Nsp, rawMsg.Type, rawMsg.Data)
}

// Helper method to build ClusterResponse from the raw data
func (s *shardedRedisAdapter) buildClusterResponse(uid adapter.ServerId, nsp string, messageType adapter.MessageType, rawData any) (*adapter.ClusterResponse, error) {
	message := &adapter.ClusterResponse{
		Uid:  uid,
		Nsp:  nsp,
		Type: messageType,
	}

	// Decode the specific message data based on the message type
	data, err := s.decodeData(messageType, rawData)
	if err != nil {
		return nil, fmt.Errorf("invalid data format: %w", err)
	}

	message.Data = data
	return message, nil
}

func (s *shardedRedisAdapter) decodeData(messageType adapter.MessageType, rawData any) (any, error) {
	// Pre-allocate the target message structure based on the message type
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

	// Decode data based on the format (JSON or MessagePack)
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

func (s *shardedRedisAdapter) shouldUseASeparateNamespace(room socket.Room) bool {
	_, isPrivateRoom := s.Sids().Load(socket.SocketId(room))

	return (s.opts.SubscriptionMode() == DynamicSubscriptionMode && !isPrivateRoom) ||
		s.opts.SubscriptionMode() == DynamicPrivateSubscriptionMode
}
