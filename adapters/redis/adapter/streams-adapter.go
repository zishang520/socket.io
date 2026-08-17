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
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
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
	restoreSessionPageSize       = 1000
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
type RedisStreamsAdapterBuilder struct {
	// Redis is the Redis client used for stream operations.
	Redis *redis.RedisClient
	// Opts contains configuration options for the streams adapter.
	Opts RedisStreamsAdapterOptionsInterface
}

// New creates a new Redis Streams adapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (sb *RedisStreamsAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewRedisStreamsAdapter(nsp, sb.Redis, sb.Opts)
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

	pubSub             *redisPubSub
	pubSubSubscription *redisSubscription
	shardedPubSub      *shardedPubSub
	subscription       *shardedSubscription
	streamPoller       *redisStreamsPoller
	server             *socket.Server

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
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
	options := DefaultRedisStreamsAdapterOptions()
	if provided, ok := opts.(RedisStreamsAdapterOptionsInterface); ok {
		options.Assign(provided)
	}
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
		options.SetHeartbeatInterval(5 * time.Second)
	}
	if options.GetRawHeartbeatTimeout() == nil {
		options.SetHeartbeatTimeout(10_000)
	}

	a := MakeRedisStreamsAdapter()
	a.SetRedis(client)
	a.SetOpts(options)
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

	r.ctx, r.cancel = context.WithCancel(r.redisClient.Context())
	r.server = nsp.Server()

	// Each namespace is routed to a specific stream to ensure ordering
	r.streamName = computeStreamName(nsp.Name(), r.opts)

	// Set up PUB/SUB channels matching Node.js format: prefix#nsp# and prefix#nsp#uid#
	r.publicChannel = r.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	privateChannel := r.publicChannel + string(r.Uid()) + "#"

	// Subscribe to both public and private channels for PUB/SUB messages
	if r.opts.UseShardedPubSub() {
		r.shardedPubSub = acquireShardedPubSub(r.server, r.redisClient)
		r.subscription = r.shardedPubSub.newSubscription(r.onPubSubMessage)
		r.subscription.Subscribe(r.publicChannel)
		r.subscription.Subscribe(privateChannel)
		_ = r.shardedPubSub.flush(r.ctx)
	} else {
		r.pubSub = acquireRedisPubSub(r.server, r.redisClient)
		r.pubSubSubscription = r.pubSub.newSubscription(r.onPubSubMessage)
		r.pubSubSubscription.Subscribe(r.publicChannel, privateChannel)
		_ = r.pubSub.flush(r.ctx)
	}
	r.streamPoller = acquireRedisStreamsPoller(r)
}

func (r *redisStreamsAdapter) onPubSubMessage(payload []byte, _ string) {
	if r.ctx != nil && r.ctx.Err() != nil {
		return
	}
	message, err := redis.UnmarshalClusterMessage(payload)
	if err != nil {
		redisStreamsLog.Debug("invalid PUB/SUB message format: %s", err.Error())
		return
	}
	r.OnMessage(message, "")
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
			return "", r.redisClient.Client().SPublish(r.ctx, r.publicChannel, payload).Err()
		}
		return "", r.redisClient.Client().Publish(r.ctx, r.publicChannel, payload).Err()
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
		return r.redisClient.Client().SPublish(r.ctx, responseChannel, payload).Err()
	}
	return r.redisClient.Client().Publish(r.ctx, responseChannel, payload).Err()
}

// ServerCount returns the number of servers connected to the cluster,
// determined by the number of PUB/SUB subscribers on the public channel.
func (r *redisStreamsAdapter) ServerCount() (int64, error) {
	return pubSubNumSub(r.ctx, r.redisClient.Sub(), r.opts.UseShardedPubSub(), r.publicChannel)
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
	var cleanup func()
	r.closeOnce.Do(func() {
		if r.cancel != nil {
			r.cancel()
		}
		if r.pubSubSubscription != nil {
			r.pubSubSubscription.Close()
		}
		if r.pubSub != nil {
			releaseRedisPubSub(r.server, r.redisClient, r.pubSub)
		}
		if r.subscription != nil {
			r.subscription.Close()
		}
		if r.shardedPubSub != nil {
			releaseShardedPubSub(r.server, r.redisClient, r.shardedPubSub)
		}
		if r.streamPoller != nil {
			releaseRedisStreamsPoller(r.streamPoller, r)
		}
		if callback := r.cleanupFunc.Swap(nil); callback != nil {
			cleanup = *callback
		}
		r.ClusterAdapter.Close()
	})
	if cleanup != nil {
		cleanup()
	}
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

	if err := r.redisClient.Client().Set(
		r.redisClient.Context(),
		sessionKey,
		base64.StdEncoding.EncodeToString(data),
		ttl,
	).Err(); err != nil {
		r.redisClient.Emit("error", err)
	}
}

// recoveryClient returns the write-side owner of this adapter's stream. Recovery
// reads must not use SubClient: it may be a replica and lag behind the XADD that
// produced the client's offset.
func (r *redisStreamsAdapter) recoveryClient() (rds.Cmdable, error) {
	switch client := r.redisClient.Client().(type) {
	case *rds.ClusterClient:
		return client.MasterForKey(r.redisClient.Context(), r.streamName)
	default:
		// Opaque UniversalClient implementations cannot expose their topology;
		// their write client must provide primary-consistent stream reads.
		return client, nil
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

	streamClient, err := r.recoveryClient()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve stream owner: %w", err)
	}

	sessionKey := r.opts.SessionKeyPrefix() + string(pid)

	// Use MULTI GET DEL for compatibility with Redis versions before 6.2.
	pipeline := r.redisClient.Client().TxPipeline()
	sessionCmd := pipeline.Get(r.redisClient.Context(), sessionKey)
	pipeline.Del(r.redisClient.Context(), sessionKey)
	_, err = pipeline.Exec(r.redisClient.Context())
	if err != nil && !errors.Is(err, rds.Nil) {
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}

	rawSession := sessionCmd.Val()
	if rawSession == "" {
		return nil, errors.New("session not found")
	}

	// Verify the offset exists in the stream
	offsets, err := streamClient.XRange(r.redisClient.Context(), r.streamName, offset, offset).Result()
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
	if err := r.collectMissedPackets(streamClient, session, offset); err != nil {
		return nil, err
	}

	return session, nil
}

// collectMissedPackets iterates through the Redis stream to find packets
// that the session missed during disconnection.
func (r *redisStreamsAdapter) collectMissedPackets(client rds.Cmdable, session *socket.Session, offset string) error {
	broadcastTypeStr := strconv.Itoa(int(adapter.BROADCAST))

	for range restoreSessionMaxXRangeCalls {
		entries, err := client.XRangeN(
			r.redisClient.Context(),
			r.streamName,
			r.nextOffset(offset),
			"+",
			restoreSessionPageSize,
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
				if !ok || data.Packet == nil || data.Opts == nil {
					return errors.New("invalid broadcast message")
				}
				recoverable := data.Packet.Type == parser.EVENT && data.Packet.Id == nil &&
					(data.Opts.Flags == nil || !data.Opts.Flags.Volatile)
				if recoverable && r.shouldIncludePacket(session.Rooms, data.Opts) {
					packetData := slices.AppendCopy(utils.TryCast[[]any](data.Packet.Data), entry.ID)
					session.MissedPackets = append(session.MissedPackets, packetData)
				}
			}
			offset = entry.ID
		}

		if len(entries) < restoreSessionPageSize {
			break
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
