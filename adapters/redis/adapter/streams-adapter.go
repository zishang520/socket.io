// Package adapter implements a Redis Streams-based adapter for Socket.IO clustering.
// Redis Streams provide message persistence and enable session recovery across server restarts.
// Ephemeral messages (fetchSockets, serverSideEmit, broadcastWithAck) are sent via Redis PUB/SUB
// for compatibility with the Node.js @socket.io/redis-streams-adapter package.
package adapter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	rds "github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	redisStreamsLog = log.NewLog("socket.io-redis-streams")

	// offsetRegex validates Redis stream offset format (timestamp-sequence).
	offsetRegex = regexp.MustCompile(`^[0-9]+-[0-9]+$`)
)

// Configuration constants for Redis Streams adapter.
const (
	// restoreSessionMaxXRangeCalls limits the number of XRANGE calls during session restoration.
	restoreSessionMaxXRangeCalls = 100

	restoreSessionPageSize = 1000
)

// hashCode computes a hash code for the given string, matching the Node.js implementation.
// This is used to deterministically map namespaces to streams when streamCount > 1.
func hashCode(str string) int {
	hash := 0
	for _, chr := range str {
		hash = hash*31 + int(chr)
		hash &= 0x7FFFFFFF // Keep positive (match JS |= 0 but unsigned)
	}
	return hash
}

// computeStreamName determines which stream a namespace should use.
// With streamCount=1, returns the base stream name. Otherwise, uses
// a hash to distribute namespaces across multiple streams.
func computeStreamName(namespaceName string, opts RedisStreamsAdapterOptionsInterface) string {
	if opts.StreamCount() <= 1 {
		return opts.StreamName()
	}
	i := hashCode(namespaceName) % opts.StreamCount()
	return opts.StreamName() + "-" + strconv.Itoa(i)
}

// isEphemeral determines whether a message should be sent via PUB/SUB instead of Streams.
// Ephemeral messages include: broadcastWithAck, serverSideEmit, fetchSockets.
// This matches the Node.js implementation for cross-language compatibility.
func isEphemeral(message *adapter.ClusterMessage) bool {
	if message.Type == adapter.BROADCAST {
		if data, ok := message.Data.(*adapter.BroadcastMessage); ok {
			return data.RequestId != nil
		}
	}
	return message.Type == adapter.SERVER_SIDE_EMIT || message.Type == adapter.FETCH_SOCKETS
}

// RedisStreamsAdapterBuilder creates Redis Streams adapters for Socket.IO namespaces.
// It manages the shared polling loops and PUB/SUB subscriptions across all namespace adapters.
type RedisStreamsAdapterBuilder struct {
	// Redis is the Redis client used for stream operations.
	Redis *redis.RedisClient
	// Opts contains configuration options for the streams adapter.
	Opts RedisStreamsAdapterOptionsInterface

	namespaceToAdapters types.Map[string, RedisStreamsAdapter]
	polling             atomic.Bool // Indicates if polling loops are active
	cancel              types.Atomic[context.CancelFunc]
}

// startPolling continuously reads messages from a Redis stream and dispatches them.
func (sb *RedisStreamsAdapterBuilder) startPolling(ctx context.Context, client rds.UniversalClient, streamName string, options RedisStreamsAdapterOptionsInterface) {
	offset := "$"

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		response, err := client.XRead(ctx, &rds.XReadArgs{
			Streams: []string{streamName},
			ID:      offset,
			Count:   options.ReadCount(),
			Block:   time.Duration(options.BlockTimeInMs()) * time.Millisecond,
		}).Result()

		if err != nil {
			if errors.Is(err, rds.Nil) || errors.Is(err, context.Canceled) {
				continue
			}
			redisStreamsLog.Debug("error reading from stream: %s", err.Error())
			time.Sleep(1 * time.Second)
			continue
		}

		if len(response) == 0 {
			continue
		}

		// Process each message in the stream
		for _, entry := range response[0].Messages {
			redisStreamsLog.Debug("processing entry %s", entry.ID)

			message := RawClusterMessage(entry.Values)
			if nsp := message.Nsp(); nsp != "" {
				if adapter, exists := sb.namespaceToAdapters.Load(nsp); exists {
					if err := adapter.OnRawMessage(message, entry.ID); err != nil {
						redisStreamsLog.Debug("error processing message: %s", err.Error())
					}
				}
			}

			offset = entry.ID
		}
	}
}

// New creates a new Redis Streams adapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (sb *RedisStreamsAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	options := DefaultRedisStreamsAdapterOptions().Assign(sb.Opts)

	// Apply default values
	if options.GetRawStreamName() == nil {
		options.SetStreamName(DefaultStreamName)
	}
	if options.GetRawStreamCount() == nil {
		options.SetStreamCount(DefaultStreamCount)
	}
	if options.GetRawChannelPrefix() == nil {
		options.SetChannelPrefix(DefaultChannelPrefix)
	}
	if options.GetRawMaxLen() == nil {
		options.SetMaxLen(DefaultStreamMaxLen)
	}
	if options.GetRawReadCount() == nil {
		options.SetReadCount(DefaultStreamReadCount)
	}
	if options.GetRawBlockTimeInMs() == nil {
		options.SetBlockTimeInMs(DefaultBlockTimeInMs)
	}
	if options.GetRawSessionKeyPrefix() == nil {
		options.SetSessionKeyPrefix(DefaultSessionKeyPrefix)
	}
	if options.GetRawHeartbeatInterval() == nil {
		options.SetHeartbeatInterval(5_000)
	}
	if options.GetRawHeartbeatTimeout() == nil {
		options.SetHeartbeatTimeout(10_000)
	}

	adapterInstance := NewRedisStreamsAdapter(nsp, sb.Redis, options)
	sb.namespaceToAdapters.Store(nsp.Name(), adapterInstance)

	// Start polling loops if not already running
	if sb.polling.CompareAndSwap(false, true) {
		ctx, cancel := context.WithCancel(sb.Redis.Context)
		sb.cancel.Store(cancel)

		// Create one read client per stream
		if options.StreamCount() <= 1 {
			go sb.startPolling(ctx, sb.Redis.Sub(), options.StreamName(), options)
		} else {
			for i := range options.StreamCount() {
				streamName := options.StreamName() + "-" + strconv.Itoa(i)
				go sb.startPolling(ctx, sb.Redis.Sub(), streamName, options)
			}
		}
	}

	// Register cleanup callback
	adapterInstance.Cleanup(func() {
		sb.namespaceToAdapters.Delete(nsp.Name())
		if sb.namespaceToAdapters.Len() == 0 {
			sb.polling.Store(false)
			if cancel := sb.cancel.Swap(nil); cancel != nil {
				cancel()
			}
		}
	})

	return adapterInstance
}

// redisStreamsAdapter implements the RedisStreamsAdapter interface using Redis Streams
// with PUB/SUB for ephemeral messages, matching the Node.js implementation.
type redisStreamsAdapter struct {
	adapter.ClusterAdapter

	redisClient *redis.RedisClient
	opts        *RedisStreamsAdapterOptions
	cleanupFunc types.Callable

	streamName    string // The specific stream for this namespace
	publicChannel string // PUB/SUB channel for ephemeral messages

	pubsub *rds.PubSub // PUB/SUB subscription for this adapter

	ctx    context.Context
	cancel context.CancelFunc
}

// MakeRedisStreamsAdapter creates a new uninitialized redisStreamsAdapter.
// Call Construct() to complete initialization before use.
func MakeRedisStreamsAdapter() RedisStreamsAdapter {
	a := &redisStreamsAdapter{
		ClusterAdapter: adapter.MakeClusterAdapter(),
		opts:           DefaultRedisStreamsAdapterOptions(),
		cleanupFunc:    nil,
	}

	a.Prototype(a)

	return a
}

// NewRedisStreamsAdapter creates and initializes a new Redis Streams adapter.
// This is the preferred way to create a streams adapter instance.
func NewRedisStreamsAdapter(nsp socket.Namespace, client *redis.RedisClient, opts any) RedisStreamsAdapter {
	a := MakeRedisStreamsAdapter()

	a.SetRedis(client)
	a.SetOpts(opts)
	a.Construct(nsp)

	return a
}

// SetRedis sets the Redis client for stream operations.
func (r *redisStreamsAdapter) SetRedis(client *redis.RedisClient) {
	r.redisClient = client
}

// SetOpts sets the configuration options for the streams adapter.
func (r *redisStreamsAdapter) SetOpts(opts any) {
	if options, ok := opts.(RedisStreamsAdapterOptionsInterface); ok {
		r.opts.Assign(options)
	}
}

// Construct initializes the streams adapter for the given namespace.
// Sets up stream name, PUB/SUB channels, and subscriptions.
func (r *redisStreamsAdapter) Construct(nsp socket.Namespace) {
	r.ClusterAdapter.Construct(nsp)

	r.ctx, r.cancel = context.WithCancel(r.redisClient.Context)

	// Each namespace is routed to a specific stream to ensure ordering
	r.streamName = computeStreamName(nsp.Name(), r.opts)

	// Set up PUB/SUB channels matching Node.js format: prefix#nsp# and prefix#nsp#uid#
	r.publicChannel = r.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	privateChannel := r.opts.ChannelPrefix() + "#" + nsp.Name() + "#" + string(r.Uid()) + "#"

	// Subscribe to both public and private channels for PUB/SUB messages
	if r.opts.UseShardedPubSub() {
		r.pubsub = r.redisClient.Sub().SSubscribe(r.ctx, r.publicChannel, privateChannel)
	} else {
		r.pubsub = r.redisClient.Sub().Subscribe(r.ctx, r.publicChannel, privateChannel)
	}
	go r.handlePubSubMessages()
}

// handlePubSubMessages listens for PUB/SUB messages (ephemeral messages and responses).
func (r *redisStreamsAdapter) handlePubSubMessages() {
	defer func() { _ = r.pubsub.Close() }()
	for {
		msg, err := r.pubsub.ReceiveMessage(r.ctx)
		if err != nil {
			if errors.Is(err, rds.ErrClosed) || r.ctx.Err() != nil {
				return
			}
			redisStreamsLog.Debug("error receiving PUB/SUB message: %s", err.Error())
			continue
		}

		var message adapter.ClusterMessage
		if err := utils.MsgPack().Decode([]byte(msg.Payload), &message); err != nil {
			redisStreamsLog.Debug("invalid PUB/SUB message format: %s", err.Error())
			continue
		}

		r.OnMessage(&message, "")
	}
}

// DoPublish publishes a cluster message.
// Ephemeral messages (fetchSockets, serverSideEmit, broadcastWithAck) go via PUB/SUB.
// Durable messages (broadcast, socketsJoin, etc.) go via Redis Streams.
func (r *redisStreamsAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	redisStreamsLog.Debug("publishing message: %+v", message)

	if isEphemeral(message) {
		// Ephemeral messages are sent via Redis PUB/SUB
		payload, err := utils.MsgPack().Encode(message)
		if err != nil {
			return "", fmt.Errorf("failed to encode ephemeral message: %w", err)
		}
		if r.opts.UseShardedPubSub() {
			err = r.redisClient.Client.SPublish(r.ctx, r.publicChannel, payload).Err()
		} else {
			err = r.redisClient.Client.Publish(r.ctx, r.publicChannel, payload).Err()
		}
		if err != nil {
			return "", err
		}
		return "", nil
	}

	// Durable messages are sent via Redis Streams
	entryID, err := r.redisClient.Client.XAdd(r.redisClient.Context, &rds.XAddArgs{
		Stream: r.streamName,
		MaxLen: r.opts.MaxLen(),
		Approx: true, // Use approximate trimming (~) for better performance
		ID:     "*",  // Let Redis generate the ID
		Values: map[string]any(r.encode(message)),
	}).Result()

	if err != nil {
		return "", err
	}

	return adapter.Offset(entryID), nil
}

// DoPublishResponse publishes a response message via PUB/SUB to the requester's private channel.
// This matches the Node.js implementation where responses are sent via PUB/SUB.
func (r *redisStreamsAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	responseChannel := r.opts.ChannelPrefix() + "#" + r.Nsp().Name() + "#" + string(requesterUid) + "#"
	payload, err := utils.MsgPack().Encode(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}
	if r.opts.UseShardedPubSub() {
		return r.redisClient.Client.SPublish(r.ctx, responseChannel, payload).Err()
	}
	return r.redisClient.Client.Publish(r.ctx, responseChannel, payload).Err()
}

// encode converts a ClusterResponse into a RawClusterMessage for Redis Streams storage.
// Binary data is encoded as base64-encoded MessagePack, while other data uses JSON.
func (r *redisStreamsAdapter) encode(message *adapter.ClusterResponse) RawClusterMessage {
	rawMessage := RawClusterMessage{
		"uid":  string(message.Uid),
		"nsp":  message.Nsp,
		"type": strconv.Itoa(int(message.Type)),
	}

	if message.Data == nil {
		return rawMessage
	}

	// Determine if the message type may contain binary data
	mayContainBinary := message.Type == adapter.BROADCAST ||
		message.Type == adapter.FETCH_SOCKETS_RESPONSE ||
		message.Type == adapter.SERVER_SIDE_EMIT ||
		message.Type == adapter.SERVER_SIDE_EMIT_RESPONSE ||
		message.Type == adapter.BROADCAST_ACK

	// Use MessagePack for binary data, JSON for text data
	if !r.opts.OnlyPlaintext() && mayContainBinary && parser.HasBinary(message.Data) {
		if data, err := utils.MsgPack().Encode(message.Data); err == nil {
			rawMessage["data"] = base64.StdEncoding.EncodeToString(data)
		}
	} else {
		if data, err := json.Marshal(message.Data); err == nil {
			rawMessage["data"] = string(data)
		}
	}

	return rawMessage
}

// ServerCount returns the number of servers connected to the cluster,
// determined by the number of PUB/SUB subscribers on the public channel.
func (r *redisStreamsAdapter) ServerCount() int64 {
	var result map[string]int64
	var err error
	if r.opts.UseShardedPubSub() {
		result, err = r.redisClient.Client.PubSubShardNumSub(r.ctx, r.publicChannel).Result()
	} else {
		result, err = r.redisClient.Client.PubSubNumSub(r.ctx, r.publicChannel).Result()
	}
	if err != nil {
		redisStreamsLog.Debug("error getting server count: %s", err.Error())
		return 1
	}
	if count, ok := result[r.publicChannel]; ok {
		return count
	}
	return 1
}

// Cleanup registers a cleanup callback to be called when the adapter is closed.
func (r *redisStreamsAdapter) Cleanup(cleanup func()) {
	r.cleanupFunc = cleanup
}

// Close releases resources and invokes the registered cleanup callback.
func (r *redisStreamsAdapter) Close() {
	defer r.cancel()

	if r.pubsub != nil {
		_ = r.pubsub.Close()
	}

	if r.cleanupFunc != nil {
		r.cleanupFunc()
	}

	r.ClusterAdapter.Close()
}

// OnRawMessage processes a raw message from the Redis stream.
// It decodes the message and dispatches it to the appropriate handler.
func (r *redisStreamsAdapter) OnRawMessage(rawMessage RawClusterMessage, offset string) error {
	message, err := r.decode(rawMessage)
	if err != nil {
		return err
	}

	r.OnMessage(message, adapter.Offset(offset))
	return nil
}

// decode converts a RawClusterMessage into a ClusterResponse.
// It handles both JSON and base64-encoded MessagePack formats.
func (r *redisStreamsAdapter) decode(rawMessage RawClusterMessage) (*adapter.ClusterResponse, error) {
	// Parse the message type
	messageType, err := strconv.ParseInt(rawMessage.Type(), 10, 0)
	if err != nil {
		return nil, fmt.Errorf("invalid message type: %w", err)
	}

	// Initialize the base message
	message := &adapter.ClusterMessage{
		Uid:  adapter.ServerId(rawMessage.Uid()),
		Nsp:  rawMessage.Nsp(),
		Type: adapter.MessageType(messageType),
	}

	// Return early if no data to process
	data := rawMessage.Data()
	if data == "" {
		return message, nil
	}

	// Detect format by first character: '{' indicates JSON, otherwise base64 MessagePack
	var rawData any
	if data[0] == '{' {
		rawData = json.RawMessage(data)
	} else {
		decodedData, b64Err := base64.StdEncoding.DecodeString(data)
		if b64Err != nil {
			return nil, fmt.Errorf("failed to decode base64 data: %w", b64Err)
		}
		rawData = msgpack.RawMessage(decodedData)
	}

	// Decode message data based on the message type
	message.Data, err = r.decodeData(message.Type, rawData)
	if err != nil {
		return nil, err
	}

	return message, nil
}

// decodeData deserializes the message payload based on the message type and format.
// It allocates the appropriate struct type and unmarshals the data into it.
func (r *redisStreamsAdapter) decodeData(messageType adapter.MessageType, rawData any) (any, error) {
	// Allocate the appropriate target struct based on message type
	var target any
	switch messageType {
	case adapter.INITIAL_HEARTBEAT, adapter.HEARTBEAT, adapter.ADAPTER_CLOSE:
		// These message types have no data payload
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

	// Unmarshal the data based on its format
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
		return nil, errors.New("unsupported data format: expected JSON or MessagePack")
	}

	return target, nil
}

// PersistSession saves a session to Redis for later recovery.
// The session is serialized using MessagePack and stored with a TTL based on
// the server's MaxDisconnectionDuration setting.
func (r *redisStreamsAdapter) PersistSession(session *socket.SessionToPersist) {
	redisStreamsLog.Debug("persisting session: %v", session)

	sessionKey := r.opts.SessionKeyPrefix() + string(session.Pid)
	data, err := utils.MsgPack().Encode(session)
	if err != nil {
		redisStreamsLog.Debug("failed to encode session: %s", err.Error())
		return
	}

	ttl := time.Duration(r.Nsp().Server().Opts().ConnectionStateRecovery().MaxDisconnectionDuration()) * time.Millisecond

	if err := r.redisClient.Client.Set(
		r.redisClient.Context,
		sessionKey,
		base64.StdEncoding.EncodeToString(data),
		ttl,
	).Err(); err != nil {
		r.redisClient.Emit("error", err)
	}
}

// RestoreSession restores a session from Redis and collects missed packets.
// It validates the offset format, retrieves the stored session, and iterates
// through the stream to find packets the client missed during disconnection.
func (r *redisStreamsAdapter) RestoreSession(pid socket.PrivateSessionId, offset string) (*socket.Session, error) {
	redisStreamsLog.Debug("restoring session %s from offset %s", pid, offset)

	// Validate offset format
	if !offsetRegex.MatchString(offset) {
		return nil, errors.New("invalid offset format")
	}

	sessionKey := r.opts.SessionKeyPrefix() + string(pid)

	// Get and delete the session atomically
	rawSession, err := r.redisClient.Client.GetDel(r.redisClient.Context, sessionKey).Result()
	if err != nil && !errors.Is(err, rds.Nil) {
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}

	if rawSession == "" {
		return nil, errors.New("session not found")
	}

	// Verify the offset exists in the stream
	offsets, err := r.redisClient.Sub().XRange(r.redisClient.Context, r.streamName, offset, offset).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to verify offset: %w", err)
	}

	if len(offsets) == 0 {
		return nil, errors.New("offset not found in stream")
	}

	// Decode the session data
	rawSessionBytes, err := base64.StdEncoding.DecodeString(rawSession)
	if err != nil {
		return nil, fmt.Errorf("failed to decode session data: %w", err)
	}

	session := &socket.Session{}
	if err := utils.MsgPack().Decode(rawSessionBytes, &session.SessionToPersist); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	redisStreamsLog.Debug("found session: %+v", session)

	// Collect missed packets from the stream
	r.collectMissedPackets(session, offset)

	return session, nil
}

// collectMissedPackets iterates through the Redis stream to find packets
// that the session missed during disconnection.
func (r *redisStreamsAdapter) collectMissedPackets(session *socket.Session, offset string) {
	broadcastTypeStr := strconv.Itoa(int(adapter.BROADCAST))

	for range restoreSessionMaxXRangeCalls {
		entries, err := r.redisClient.Sub().XRangeN(
			r.redisClient.Context,
			r.streamName,
			r.nextOffset(offset),
			"+",
			restoreSessionPageSize,
		).Result()

		if err != nil || len(entries) == 0 {
			break
		}

		for _, entry := range entries {
			rawMessage := RawClusterMessage(entry.Values)

			// Only process broadcast messages for this namespace
			if rawMessage.Nsp() == r.Nsp().Name() && rawMessage.Type() == broadcastTypeStr {
				if message, err := r.decode(rawMessage); err == nil {
					if data, ok := message.Data.(*adapter.BroadcastMessage); ok {
						if r.shouldIncludePacket(session.Rooms, data.Opts) {
							packetData := append(utils.TryCast[[]any](data.Packet.Data), entry.ID)
							session.MissedPackets = append(session.MissedPackets, packetData)
						}
					}
				}
			}
			offset = entry.ID
		}

		if len(entries) < restoreSessionPageSize {
			break
		}
	}
}

// nextOffset computes the next stream entry ID by incrementing the sequence number.
// Redis stream IDs have the format "timestamp-sequence".
func (redisStreamsAdapter) nextOffset(offset string) string {
	timestamp, sequence, found := strings.Cut(offset, "-")
	if !found {
		return offset
	}

	if seqNum, err := strconv.ParseUint(sequence, 10, 64); err == nil {
		return timestamp + "-" + strconv.FormatUint(seqNum+1, 10)
	}

	return offset
}

// shouldIncludePacket determines if a packet should be included for session recovery.
// A packet is included if:
// 1. It was sent to all rooms (no specific rooms) OR to a room the session is in
// 2. It was not sent to a room that excludes the session
func (redisStreamsAdapter) shouldIncludePacket(sessionRooms *types.Set[socket.Room], opts *adapter.PacketOptions) bool {
	// Check if packet targets the session's rooms
	included := len(opts.Rooms) == 0
	if !included {
		if slices.ContainsFunc(opts.Rooms, sessionRooms.Has) {
			included = true
		}
	}

	// Check if session is excluded
	if slices.ContainsFunc(opts.Except, sessionRooms.Has) {
		return false
	}

	return included
}
