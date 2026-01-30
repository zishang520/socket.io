// Package adapter implements a Redis Streams-based adapter for Socket.IO clustering.
// Redis Streams provide message persistence and enable session recovery across server restarts.
package adapter

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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

	// xReadBlockTimeout is the blocking timeout for XREAD operations.
	xReadBlockTimeout = 5000 * time.Millisecond

	// defaultHeartbeatInterval is the default interval between heartbeats.
	defaultHeartbeatInterval = 5_000

	// defaultHeartbeatTimeout is the default timeout for heartbeat responses.
	defaultHeartbeatTimeout = 10_000
)

// RedisStreamsAdapterBuilder creates Redis Streams adapters for Socket.IO namespaces.
// It manages the shared polling loop across all namespace adapters.
type RedisStreamsAdapterBuilder struct {
	// Redis is the Redis client used for stream operations.
	Redis *redis.RedisClient
	// Opts contains configuration options for the streams adapter.
	Opts RedisStreamsAdapterOptionsInterface

	namespaceToAdapters types.Map[string, RedisStreamsAdapter]
	offset              types.Atomic[string] // Default: "$" (read new entries only)
	polling             atomic.Bool          // Indicates if polling loop is active
	shouldClose         atomic.Bool          // Signals the polling loop to stop
}

// poll continuously reads messages from the Redis stream and dispatches them to the appropriate adapter.
func (sb *RedisStreamsAdapterBuilder) poll(options RedisStreamsAdapterOptionsInterface) {
	for {
		// Check termination conditions
		if sb.shouldClose.Load() || sb.namespaceToAdapters.Len() == 0 {
			sb.polling.Store(false)
			return
		}

		// Get current offset, defaulting to "$" for new entries only
		offset := sb.offset.Load()
		if offset == "" {
			offset = "$"
		}

		response, err := sb.Redis.Client.XRead(sb.Redis.Context, &rds.XReadArgs{
			Streams: []string{options.StreamName()},
			ID:      offset,
			Count:   options.ReadCount(),
			Block:   xReadBlockTimeout,
		}).Result()

		if err != nil {
			redisStreamsLog.Debug("error reading from stream: %s", err.Error())
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

			sb.offset.Store(entry.ID)
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
	if options.GetRawMaxLen() == nil {
		options.SetMaxLen(DefaultStreamMaxLen)
	}
	if options.GetRawReadCount() == nil {
		options.SetReadCount(DefaultStreamReadCount)
	}
	if options.GetRawSessionKeyPrefix() == nil {
		options.SetSessionKeyPrefix(DefaultSessionKeyPrefix)
	}
	if options.GetRawHeartbeatInterval() == nil {
		options.SetHeartbeatInterval(defaultHeartbeatInterval)
	}
	if options.GetRawHeartbeatTimeout() == nil {
		options.SetHeartbeatTimeout(defaultHeartbeatTimeout)
	}

	adapterInstance := NewRedisStreamsAdapter(nsp, sb.Redis, options)
	sb.namespaceToAdapters.Store(nsp.Name(), adapterInstance)

	// Start polling loop if not already running
	if sb.polling.CompareAndSwap(false, true) {
		sb.shouldClose.Store(false)
		go sb.poll(options)
	}

	// Register cleanup callback
	adapterInstance.Cleanup(func() {
		sb.namespaceToAdapters.Delete(nsp.Name())
		if sb.namespaceToAdapters.Len() == 0 {
			sb.shouldClose.Store(true)
		}
	})

	return adapterInstance
}

// redisStreamsAdapter implements the RedisStreamsAdapter interface using Redis Streams.
// It provides reliable message delivery with built-in persistence for session recovery.
type redisStreamsAdapter struct {
	adapter.ClusterAdapterWithHeartbeat

	redisClient *redis.RedisClient
	opts        *RedisStreamsAdapterOptions
	cleanupFunc types.Callable // Cleanup callback for resource management
}

// MakeRedisStreamsAdapter creates a new uninitialized redisStreamsAdapter.
// Call Construct() to complete initialization before use.
func MakeRedisStreamsAdapter() RedisStreamsAdapter {
	a := &redisStreamsAdapter{
		ClusterAdapterWithHeartbeat: adapter.MakeClusterAdapterWithHeartbeat(),
		opts:                        DefaultRedisStreamsAdapterOptions(),
		cleanupFunc:                 nil,
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
// Options are merged with the parent ClusterAdapterWithHeartbeat options.
func (r *redisStreamsAdapter) SetOpts(opts any) {
	r.ClusterAdapterWithHeartbeat.SetOpts(opts)

	if options, ok := opts.(RedisStreamsAdapterOptionsInterface); ok {
		r.opts.Assign(options)
	}
}

// Construct initializes the streams adapter for the given namespace.
// This method must be called before using the adapter.
func (r *redisStreamsAdapter) Construct(nsp socket.Namespace) {
	r.ClusterAdapterWithHeartbeat.Construct(nsp)
	r.Init()
}

// DoPublish publishes a cluster message to the Redis stream.
// The message is encoded and added to the stream with automatic ID generation.
// Returns the stream entry ID as the offset for connection state recovery.
func (r *redisStreamsAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	redisStreamsLog.Debug("publishing message: %+v", message)

	entryID, err := r.redisClient.Client.XAdd(r.redisClient.Context, &rds.XAddArgs{
		Stream: r.opts.StreamName(),
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

// DoPublishResponse publishes a response message to the Redis stream.
// This is used for request-response patterns in the cluster.
func (r *redisStreamsAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	_, err := r.DoPublish(response)
	return err
}

// encode converts a ClusterResponse into a RawClusterMessage for Redis Streams storage.
// Binary data is encoded as base64-encoded MessagePack, while other data uses JSON.
func (redisStreamsAdapter) encode(message *adapter.ClusterResponse) RawClusterMessage {
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
	if mayContainBinary && parser.HasBinary(message.Data) {
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

// Cleanup registers a cleanup callback to be called when the adapter is closed.
func (r *redisStreamsAdapter) Cleanup(cleanup func()) {
	r.cleanupFunc = cleanup
}

// Close releases resources and invokes the registered cleanup callback.
func (r *redisStreamsAdapter) Close() {
	defer r.ClusterAdapterWithHeartbeat.Close()

	if r.cleanupFunc != nil {
		r.cleanupFunc()
	}
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
		decodedData, err := base64.StdEncoding.DecodeString(data)
		if err != nil {
			return nil, fmt.Errorf("failed to decode base64 data: %w", err)
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
	offsets, err := r.redisClient.Client.XRange(r.redisClient.Context, r.opts.StreamName(), offset, offset).Result()
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

	for i := 0; i < restoreSessionMaxXRangeCalls; i++ {
		entries, err := r.redisClient.Client.XRange(
			r.redisClient.Context,
			r.opts.StreamName(),
			r.nextOffset(offset),
			"+",
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
							session.MissedPackets = append(session.MissedPackets, data.Packet)
						}
					}
				}
			}
			offset = entry.ID
		}
	}
}

// nextOffset computes the next stream entry ID by incrementing the sequence number.
// Redis stream IDs have the format "timestamp-sequence".
func (redisStreamsAdapter) nextOffset(offset string) string {
	dashPos := strings.LastIndex(offset, "-")
	if dashPos == -1 {
		return offset
	}

	timestamp := offset[:dashPos]
	sequence := offset[dashPos+1:]

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
		for _, room := range opts.Rooms {
			if sessionRooms.Has(room) {
				included = true
				break
			}
		}
	}

	// Check if session is excluded
	for _, room := range opts.Except {
		if sessionRooms.Has(room) {
			return false
		}
	}

	return included
}
