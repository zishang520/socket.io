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
	"math"
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

	// errRestoreSessionReadLimit is returned when recovery cannot observe the
	// end of the stream within its bounded number of XRANGE calls.
	errRestoreSessionReadLimit = errors.New("session recovery exceeded XRANGE call limit")
)

// Configuration constants for Redis Streams adapter.
const (
	// restoreSessionMinXRangeCalls matches the Node.js protection against chasing a moving stream forever.
	restoreSessionMinXRangeCalls int64 = 100
	restoreSessionPageSize       int64 = 1000
)

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
	if r.opts.GetRawStreamName() == nil {
		r.opts.SetStreamName(DefaultStreamName)
	}
	if r.opts.GetRawStreamCount() == nil {
		r.opts.SetStreamCount(DefaultStreamCount)
	}
	if r.opts.GetRawChannelPrefix() == nil {
		r.opts.SetChannelPrefix(DefaultChannelPrefix)
	}
	if r.opts.GetRawMaxLen() == nil {
		r.opts.SetMaxLen(DefaultStreamMaxLen)
	}
	if r.opts.GetRawReadCount() == nil {
		r.opts.SetReadCount(DefaultStreamReadCount)
	}
	if r.opts.GetRawBlockTimeInMs() == nil {
		r.opts.SetBlockTimeInMs(DefaultBlockTimeInMs)
	}
	if r.opts.GetRawSessionKeyPrefix() == nil {
		r.opts.SetSessionKeyPrefix(DefaultSessionKeyPrefix)
	}

	r.ctx, r.cancel = context.WithCancel(r.redisClient.Context())
	r.server = nsp.Server()

	// Each namespace is routed to a specific stream to ensure ordering
	r.streamName = redis.StreamNameForNamespace(
		r.opts.StreamName(),
		nsp.Name(),
		r.opts.StreamCount(),
	)

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
	block, configErr := redisStreamsPollerBlock(r.opts.BlockTimeInMs())
	if configErr != nil {
		redisStreamsLog.Debug("invalid Redis Streams poller configuration: %s", configErr.Error())
		block = time.Duration(DefaultBlockTimeInMs) * time.Millisecond
	}
	poller, initialErr := acquireRedisStreamsPoller(r, block)
	r.streamPoller = poller
	if r.ctx.Err() != nil {
		releaseRedisStreamsPoller(poller, r)
		return
	}
	if configErr != nil {
		r.redisClient.Emit("error", configErr)
	}
	if initialErr != nil && r.ctx.Err() == nil {
		redisStreamsLog.Debug("error reading stream tail: %s", initialErr.Error())
		r.redisClient.Emit("error", initialErr)
	}
}

func (r *redisStreamsAdapter) onPubSubMessage(payload []byte, _ string) {
	if r.ctx != nil && r.ctx.Err() != nil {
		return
	}
	message, err := adapter.DecodeClusterMessage(payload)
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
	publishCtx := r.redisClient.Context()
	redisStreamsLog.Debug("publishing message: %+v", message)

	if isEphemeral(message) {
		// Ephemeral messages are sent via Redis PUB/SUB
		payload, err := adapter.EncodeClusterMessageMsgpack(message)
		if err != nil {
			return "", fmt.Errorf("failed to encode ephemeral message: %w", err)
		}
		if r.opts.UseShardedPubSub() {
			return "", r.redisClient.Client().SPublish(publishCtx, r.publicChannel, payload).Err()
		}
		return "", r.redisClient.Client().Publish(publishCtx, r.publicChannel, payload).Err()
	}

	// Durable messages are sent via Redis Streams
	rawMessage, err := redis.EncodeStreamMessage(message, r.opts.OnlyPlaintext())
	if err != nil {
		return "", fmt.Errorf("failed to encode stream message: %w", err)
	}
	entryID, err := redis.XAddContext(publishCtx, r.redisClient, r.streamName, rawMessage, r.opts.MaxLen())

	if err != nil {
		return "", err
	}

	return adapter.Offset(entryID), nil
}

// DoPublishResponse publishes a response message via PUB/SUB to the requester's private channel.
// This matches the Node.js implementation where responses are sent via PUB/SUB.
func (r *redisStreamsAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	responseChannel := r.opts.ChannelPrefix() + "#" + r.Nsp().Name() + "#" + string(requesterUid) + "#"
	payload, err := adapter.EncodeClusterMessageMsgpack(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}
	if r.opts.UseShardedPubSub() {
		return r.redisClient.Client().SPublish(r.redisClient.Context(), responseChannel, payload).Err()
	}
	return r.redisClient.Client().Publish(r.redisClient.Context(), responseChannel, payload).Err()
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
		r.ClusterAdapter.Close()
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
	if err := r.ctx.Err(); err != nil {
		return
	}

	maxDisconnectionDuration := r.Nsp().Server().Opts().ConnectionStateRecovery().MaxDisconnectionDuration()
	if maxDisconnectionDuration <= 0 {
		r.redisClient.Emit("error", fmt.Errorf(
			"redis streams: maxDisconnectionDuration must be positive: %dms",
			maxDisconnectionDuration,
		))
		return
	}
	if maxDisconnectionDuration > math.MaxInt64/int64(time.Millisecond) {
		r.redisClient.Emit("error", fmt.Errorf(
			"redis streams: maxDisconnectionDuration overflows time.Duration: %dms",
			maxDisconnectionDuration,
		))
		return
	}

	sessionKey := r.opts.SessionKeyPrefix() + string(session.Pid)
	data, err := utils.MsgPack().Encode(session)
	if err != nil {
		r.redisClient.Emit("error", fmt.Errorf("redis streams: failed to encode session: %w", err))
		return
	}

	if err := r.redisClient.Client().Set(
		r.ctx,
		sessionKey,
		base64.StdEncoding.EncodeToString(data),
		time.Duration(maxDisconnectionDuration)*time.Millisecond,
	).Err(); err != nil && r.ctx.Err() == nil {
		r.redisClient.Emit("error", err)
	}
}

// RestoreSession restores a session from Redis and collects missed packets.
// It validates the offset format, retrieves the stored session, and iterates
// through the stream to find packets the client missed during disconnection.
func (r *redisStreamsAdapter) RestoreSession(pid socket.PrivateSessionId, offset string) (*socket.Session, error) {
	redisStreamsLog.Debug("restoring session %s from offset %s", pid, offset)
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}

	// Validate offset format
	if !offsetRegex.MatchString(offset) {
		return nil, errors.New("invalid offset format")
	}

	// Recovery reads use the write-side client as a primary-consistency
	// boundary. ClusterClient handles MOVED and ASK redirections itself.
	streamClient := r.redisClient.Client()

	sessionKey := r.opts.SessionKeyPrefix() + string(pid)

	// Use MULTI GET DEL for compatibility with Redis versions before 6.2.
	pipeline := r.redisClient.Client().TxPipeline()
	sessionCmd := pipeline.Get(r.ctx, sessionKey)
	pipeline.Del(r.ctx, sessionKey)
	_, err := pipeline.Exec(r.ctx)
	if err != nil && !errors.Is(err, rds.Nil) {
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}

	rawSession := sessionCmd.Val()
	if rawSession == "" {
		return nil, errors.New("session not found")
	}

	// Verify the offset exists in the stream
	offsets, err := streamClient.XRangeN(
		r.ctx,
		r.streamName,
		offset,
		offset,
		1,
	).Result()
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
	if session.SessionToPersist == nil {
		return nil, errors.New("invalid persisted session: missing session data")
	}
	if session.Rooms == nil {
		session.Rooms = types.NewSet[socket.Room]()
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

	// Allow one page beyond MAXLEN for approximate trimming, plus the empty read that finds the tail.
	maxLen := r.opts.MaxLen()
	pages := maxLen / restoreSessionPageSize
	if maxLen%restoreSessionPageSize != 0 {
		pages++
	}
	maxCalls := max(restoreSessionMinXRangeCalls, pages+2)
	for range maxCalls {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		entries, err := client.XRangeN(
			r.ctx,
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
				if recoverable {
					if data.Opts.Rooms == nil || data.Opts.Except == nil {
						return errors.New("invalid broadcast options: rooms and except are required")
					}
					if r.shouldIncludePacket(session.Rooms, data.Opts) {
						packetData, ok := data.Packet.Data.([]any)
						if !ok {
							return errors.New("invalid broadcast packet data")
						}
						session.MissedPackets = append(session.MissedPackets, slices.AppendCopy(packetData, entry.ID))
					}
				}
			}
			offset = entry.ID
		}

	}

	return errRestoreSessionReadLimit
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
