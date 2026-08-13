// Package adapter implements a Redis Streams-based adapter for Socket.IO clustering.
// Redis Streams provide message persistence and enable session recovery across server restarts.
// Ephemeral messages (fetchSockets, serverSideEmit, broadcastWithAck) are sent via Redis PUB/SUB
// for compatibility with the Node.js @socket.io/redis-streams-adapter package.
package adapter

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	_slices "slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/slices"
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
)

// hashCode computes a hash code for the given string, matching the Node.js implementation.
// This is used to deterministically map namespaces to streams when streamCount > 1.
func hashCode(str string) int32 {
	var hash int32
	for _, chr := range str {
		if chr <= 0xffff {
			hash = hash*31 + chr
			continue
		}
		chr -= 0x10000
		hash = hash*31 + 0xd800 + (chr >> 10)
		hash = hash*31 + 0xdc00 + (chr & 0x3ff)
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
	i := int64(hashCode(namespaceName)) % int64(opts.StreamCount())
	return opts.StreamName() + "-" + strconv.FormatInt(i, 10)
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
	mu                  sync.Mutex
	cancel              context.CancelFunc
}

// startPolling continuously reads messages from a Redis stream and dispatches them.
func (sb *RedisStreamsAdapterBuilder) startPolling(ctx context.Context, client rds.UniversalClient, streamName string, options RedisStreamsAdapterOptionsInterface) {
	readArgs := &rds.XReadArgs{
		Streams: []string{streamName},
		ID:      "$",
		Count:   options.ReadCount(),
		Block:   utils.FromMilliseconds(options.BlockTimeInMs()),
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		response, err := client.XRead(ctx, readArgs).Result()

		if err != nil {
			if errors.Is(err, rds.Nil) || errors.Is(err, context.Canceled) {
				continue
			}
			redisStreamsLog.Debug("error reading from stream: %s", err.Error())
			time.Sleep(time.Second)
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

			readArgs.ID = entry.ID
		}
	}
}

// New creates a new Redis Streams adapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (sb *RedisStreamsAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	name := nsp.Name()
	options := DefaultRedisStreamsAdapterOptions().Assign(sb.Opts)
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
		options.SetHeartbeatInterval(5_000 * time.Millisecond)
	}
	if options.GetRawHeartbeatTimeout() == nil {
		options.SetHeartbeatTimeout(10_000)
	}

	adapterInstance := NewRedisStreamsAdapter(nsp, sb.Redis, options)

	sb.mu.Lock()
	sb.namespaceToAdapters.Store(name, adapterInstance)
	if sb.cancel == nil {
		ctx, cancel := context.WithCancel(sb.Redis.Context)
		sb.cancel = cancel
		if streamCount := options.StreamCount(); streamCount <= 1 {
			go sb.startPolling(ctx, sb.Redis.Sub(), options.StreamName(), options)
		} else {
			for i := range streamCount {
				streamName := options.StreamName() + "-" + strconv.Itoa(i)
				go sb.startPolling(ctx, sb.Redis.Sub(), streamName, options)
			}
		}
	}
	sb.mu.Unlock()

	// Register cleanup callback
	adapterInstance.Cleanup(func() {
		sb.mu.Lock()
		defer sb.mu.Unlock()

		if !sb.namespaceToAdapters.CompareAndDelete(name, adapterInstance) || sb.namespaceToAdapters.Len() != 0 {
			return
		}

		if sb.cancel != nil {
			sb.cancel()
		}
		sb.cancel = nil
	})

	return adapterInstance
}

// redisStreamsAdapter implements the RedisStreamsAdapter interface using Redis Streams
// with PUB/SUB for ephemeral messages, matching the Node.js implementation.
type redisStreamsAdapter struct {
	adapter.ClusterAdapter

	redisClient *redis.RedisClient
	opts        *RedisStreamsAdapterOptions
	cleanupFunc atomic.Pointer[types.Callable]

	streamName    string // The specific stream for this namespace
	publicChannel string // PUB/SUB channel for ephemeral messages

	pubsub        *rds.PubSub // public and, in classic mode, private subscription
	privatePubSub *rds.PubSub // private subscription in sharded mode

	ctx    context.Context
	cancel context.CancelFunc
}

// MakeRedisStreamsAdapter creates a new uninitialized redisStreamsAdapter.
// Call Construct() to complete initialization before use.
func MakeRedisStreamsAdapter() RedisStreamsAdapter {
	a := &redisStreamsAdapter{
		ClusterAdapter: adapter.MakeClusterAdapter(),
		opts:           DefaultRedisStreamsAdapterOptions(),
	}

	a.Prototype(a)

	return a
}

// NewRedisStreamsAdapter creates and initializes a new Redis Streams adapter.
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
	privateChannel := r.publicChannel + string(r.Uid()) + "#"

	// Subscribe to both public and private channels for PUB/SUB messages
	if r.opts.UseShardedPubSub() {
		r.pubsub = r.redisClient.Sub().SSubscribe(r.ctx, r.publicChannel)
		r.privatePubSub = r.redisClient.Sub().SSubscribe(r.ctx, privateChannel)
		go r.handlePubSubMessages(r.privatePubSub)
	} else {
		r.pubsub = r.redisClient.Sub().Subscribe(r.ctx, r.publicChannel, privateChannel)
	}
	go r.handlePubSubMessages(r.pubsub)
}

// handlePubSubMessages listens for PUB/SUB messages (ephemeral messages and responses).
func (r *redisStreamsAdapter) handlePubSubMessages(pubsub *rds.PubSub) {
	defer func() { _ = pubsub.Close() }()
	for {
		msg, err := pubsub.ReceiveMessage(r.ctx)
		if err != nil {
			if errors.Is(err, rds.ErrClosed) || r.ctx.Err() != nil {
				return
			}
			redisStreamsLog.Debug("error receiving PUB/SUB message: %s", err.Error())
			continue
		}

		message, err := redis.UnmarshalClusterMessage([]byte(msg.Payload))
		if err != nil {
			redisStreamsLog.Debug("invalid PUB/SUB message format: %s", err.Error())
			continue
		}

		r.OnMessage(message, "")
	}
}

// DoPublish publishes a cluster message.
// Ephemeral messages (fetchSockets, serverSideEmit, broadcastWithAck) go via PUB/SUB.
// Durable messages (broadcast, socketsJoin, etc.) go via Redis Streams.
func (r *redisStreamsAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	redisStreamsLog.Debug("publishing message: %+v", message)

	if isEphemeral(message) {
		// Ephemeral messages are sent via Redis PUB/SUB
		payload, err := redis.EncodeClusterMessageMsgpack(message)
		if err != nil {
			return "", fmt.Errorf("failed to encode ephemeral message: %w", err)
		}
		if r.opts.UseShardedPubSub() {
			return "", r.redisClient.Client.SPublish(r.ctx, r.publicChannel, payload).Err()
		}
		return "", r.redisClient.Client.Publish(r.ctx, r.publicChannel, payload).Err()
	}

	// Durable messages are sent via Redis Streams
	rawMessage, err := redis.EncodeStreamMessage(message, r.opts.OnlyPlaintext())
	if err != nil {
		return "", fmt.Errorf("failed to encode stream message: %w", err)
	}
	entryID, err := redis.XAdd(r.redisClient, r.streamName, rawMessage, r.opts.MaxLen())

	if err != nil {
		return "", err
	}

	return adapter.Offset(entryID), nil
}

// DoPublishResponse publishes a response message via PUB/SUB to the requester's private channel.
// This matches the Node.js implementation where responses are sent via PUB/SUB.
func (r *redisStreamsAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	responseChannel := r.opts.ChannelPrefix() + "#" + r.Nsp().Name() + "#" + string(requesterUid) + "#"
	payload, err := redis.EncodeClusterMessageMsgpack(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}
	if r.opts.UseShardedPubSub() {
		return r.redisClient.Client.SPublish(r.ctx, responseChannel, payload).Err()
	}
	return r.redisClient.Client.Publish(r.ctx, responseChannel, payload).Err()
}

// ServerCount returns the number of servers connected to the cluster,
// determined by the number of PUB/SUB subscribers on the public channel.
func (r *redisStreamsAdapter) ServerCount() (int64, error) {
	return pubSubNumSub(r.ctx, r.redisClient.Client, r.opts.UseShardedPubSub(), r.publicChannel)
}

// Cleanup registers a cleanup callback to be called when the adapter is closed.
func (r *redisStreamsAdapter) Cleanup(cleanup func()) {
	if cleanup == nil {
		r.cleanupFunc.Store(nil)
		return
	}
	r.cleanupFunc.Store(&cleanup)
}

// Close releases resources and invokes the registered cleanup callback.
func (r *redisStreamsAdapter) Close() {
	defer r.cancel()

	if r.pubsub != nil {
		_ = r.pubsub.Close()
	}
	if r.privatePubSub != nil {
		_ = r.privatePubSub.Close()
	}

	if cleanup := r.cleanupFunc.Swap(nil); cleanup != nil {
		(*cleanup)()
	}

	r.ClusterAdapter.Close()
}

// OnRawMessage processes a raw message from the Redis stream.
// It decodes the message and dispatches it to the appropriate handler.
func (r *redisStreamsAdapter) OnRawMessage(rawMessage RawClusterMessage, offset string) error {
	message, err := redis.DecodeStreamMessage(rawMessage)
	if err != nil {
		return err
	}

	r.OnMessage(message, adapter.Offset(offset))
	return nil
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

	ttl := utils.FromMilliseconds(r.Nsp().Server().Opts().ConnectionStateRecovery().MaxDisconnectionDuration())

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

	// Use MULTI GET DEL for compatibility with Redis versions before 6.2.
	pipeline := r.redisClient.Client.TxPipeline()
	sessionCmd := pipeline.Get(r.redisClient.Context, sessionKey)
	pipeline.Del(r.redisClient.Context, sessionKey)
	_, err := pipeline.Exec(r.redisClient.Context)
	if err != nil && !errors.Is(err, rds.Nil) {
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}

	rawSession := sessionCmd.Val()
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

	session := &socket.Session{MissedPackets: []any{}}
	if err := utils.MsgPack().Decode(rawSessionBytes, &session.SessionToPersist); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	redisStreamsLog.Debug("found session: %+v", session)

	// Collect missed packets from the stream
	if err := r.collectMissedPackets(session, offset); err != nil {
		return nil, err
	}

	return session, nil
}

// collectMissedPackets iterates through the Redis stream to find packets
// that the session missed during disconnection.
func (r *redisStreamsAdapter) collectMissedPackets(session *socket.Session, offset string) error {
	broadcastTypeStr := strconv.Itoa(int(adapter.BROADCAST))

	for range restoreSessionMaxXRangeCalls {
		entries, err := r.redisClient.Sub().XRange(
			r.redisClient.Context,
			r.streamName,
			r.nextOffset(offset),
			"+",
		).Result()

		if err != nil {
			return fmt.Errorf("failed to retrieve missed packets: %w", err)
		}
		if len(entries) == 0 {
			return nil
		}

		for _, entry := range entries {
			rawMessage := RawClusterMessage(entry.Values)

			// Only process broadcast messages for this namespace
			if rawMessage.Nsp() == r.Nsp().Name() && rawMessage.Type() == broadcastTypeStr {
				message, err := redis.DecodeStreamMessage(rawMessage)
				if err != nil {
					return err
				}
				data, ok := message.Data.(*adapter.BroadcastMessage)
				if !ok || data.Packet == nil {
					return errors.New("invalid broadcast message")
				}
				if r.shouldIncludePacket(session.Rooms, data.Opts) {
					packetData := slices.AppendCopy(utils.TryCast[[]any](data.Packet.Data), entry.ID)
					session.MissedPackets = append(session.MissedPackets, packetData)
				}
			}
			offset = entry.ID
		}
	}

	return nil
}

// nextOffset computes the next stream entry ID by incrementing the sequence number.
// Redis stream IDs have the format "timestamp-sequence".
func (*redisStreamsAdapter) nextOffset(offset string) string {
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
func (*redisStreamsAdapter) shouldIncludePacket(sessionRooms *types.Set[socket.Room], opts *adapter.PacketOptions) bool {
	// Check if packet targets the session's rooms
	included := len(opts.Rooms) == 0 || _slices.ContainsFunc(opts.Rooms, sessionRooms.Has)

	// Check if session is excluded
	if _slices.ContainsFunc(opts.Except, sessionRooms.Has) {
		return false
	}

	return included
}
