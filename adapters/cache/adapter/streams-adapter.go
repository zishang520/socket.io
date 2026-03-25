// Package adapter implements a cache Streams-based adapter for Socket.IO clustering.
// Streams provide message persistence and enable session recovery across server restarts.
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

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	cacheStreamsLog = log.NewLog("socket.io-cache-streams")

	offsetRegex = regexp.MustCompile(`^[0-9]+-[0-9]+$`)
)

const (
	restoreSessionMaxXRangeCalls = 100
	restoreSessionPageSize       = 1000
	xReadBlockTimeout            = 5000 * time.Millisecond

	defaultHeartbeatInterval = 5_000
	defaultHeartbeatTimeout  = 10_000
)

// CacheStreamsAdapterBuilder creates cache streams adapters for Socket.IO namespaces.
type CacheStreamsAdapterBuilder struct {
	// Cache is the cache client used for stream operations.
	Cache cache.CacheClient
	// Opts contains configuration options.
	Opts CacheStreamsAdapterOptionsInterface

	namespaceToAdapters types.Map[string, CacheStreamsAdapter]
	polling             atomic.Bool
	cancelFunc          types.Atomic[context.CancelFunc]
}

// startPolling continuously reads messages from the cache stream and dispatches them.
func (sb *CacheStreamsAdapterBuilder) startPolling(ctx context.Context, options CacheStreamsAdapterOptionsInterface) {
	offset := "$"

	for {
		select {
		case <-ctx.Done():
			sb.polling.Store(false)
			return
		default:
		}

		streams, err := sb.Cache.XRead(ctx, []string{options.StreamName()}, offset, options.ReadCount(), xReadBlockTimeout)
		if err != nil {
			if errors.Is(err, cache.ErrNil) || errors.Is(err, context.Canceled) {
				continue
			}
			cacheStreamsLog.Debug("error reading from stream: %s", err.Error())
			time.Sleep(1 * time.Second)
			continue
		}

		if len(streams) == 0 {
			continue
		}

		for _, entry := range streams[0].Messages {
			cacheStreamsLog.Debug("processing entry %s", entry.ID)

			message := RawClusterMessage(entry.Values)
			if nsp := message.Nsp(); nsp != "" {
				if a, exists := sb.namespaceToAdapters.Load(nsp); exists {
					if err := a.OnRawMessage(message, entry.ID); err != nil {
						cacheStreamsLog.Debug("error processing message: %s", err.Error())
					}
				}
			}

			offset = entry.ID
		}
	}
}

// New creates a new cache streams adapter for the given namespace.
// Implements the socket.AdapterBuilder interface.
func (sb *CacheStreamsAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	options := DefaultCacheStreamsAdapterOptions().Assign(sb.Opts)

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

	adapterInstance := NewCacheStreamsAdapter(nsp, sb.Cache, options)
	sb.namespaceToAdapters.Store(nsp.Name(), adapterInstance)

	if sb.polling.CompareAndSwap(false, true) {
		ctx, cancelFunc := context.WithCancel(sb.Cache.Context())
		sb.cancelFunc.Store(cancelFunc)
		go sb.startPolling(ctx, options)
	}

	adapterInstance.Cleanup(func() {
		sb.namespaceToAdapters.Delete(nsp.Name())
		if sb.namespaceToAdapters.Len() == 0 {
			if cancelFunc := sb.cancelFunc.Load(); cancelFunc != nil {
				cancelFunc()
				sb.cancelFunc.Store(nil)
			}
		}
	})

	return adapterInstance
}

// cacheStreamsAdapter implements CacheStreamsAdapter using cache stream operations.
type cacheStreamsAdapter struct {
	adapter.ClusterAdapterWithHeartbeat

	cacheClient cache.CacheClient
	opts        *CacheStreamsAdapterOptions
	cleanupFunc types.Callable
}

// MakeCacheStreamsAdapter returns an uninitialized cacheStreamsAdapter.
func MakeCacheStreamsAdapter() CacheStreamsAdapter {
	a := &cacheStreamsAdapter{
		ClusterAdapterWithHeartbeat: adapter.MakeClusterAdapterWithHeartbeat(),
		opts:                        DefaultCacheStreamsAdapterOptions(),
	}
	a.Prototype(a)
	return a
}

// NewCacheStreamsAdapter creates and fully initialises a cache streams adapter.
func NewCacheStreamsAdapter(nsp socket.Namespace, client cache.CacheClient, opts any) CacheStreamsAdapter {
	a := MakeCacheStreamsAdapter()
	a.SetCache(client)
	a.SetOpts(opts)
	a.Construct(nsp)
	return a
}

func (r *cacheStreamsAdapter) SetCache(client cache.CacheClient) { r.cacheClient = client }

func (r *cacheStreamsAdapter) SetOpts(opts any) {
	r.ClusterAdapterWithHeartbeat.SetOpts(opts)
	if o, ok := opts.(CacheStreamsAdapterOptionsInterface); ok {
		r.opts.Assign(o)
	}
}

func (r *cacheStreamsAdapter) Construct(nsp socket.Namespace) {
	r.ClusterAdapterWithHeartbeat.Construct(nsp)
	r.Init()
}

// DoPublish publishes a cluster message to the stream and returns the entry ID as the offset.
func (r *cacheStreamsAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	cacheStreamsLog.Debug("publishing message: %+v", message)

	entryID, err := r.cacheClient.XAdd(
		r.cacheClient.Context(),
		r.opts.StreamName(),
		r.opts.MaxLen(),
		true,
		map[string]any(r.encode(message)),
	)
	if err != nil {
		return "", err
	}
	return adapter.Offset(entryID), nil
}

// DoPublishResponse publishes a response message to the stream.
func (r *cacheStreamsAdapter) DoPublishResponse(_ adapter.ServerId, response *adapter.ClusterResponse) error {
	_, err := r.DoPublish(response)
	return err
}

// encode converts a ClusterMessage to a RawClusterMessage for stream storage.
func (cacheStreamsAdapter) encode(message *adapter.ClusterResponse) RawClusterMessage {
	rawMessage := RawClusterMessage{
		"uid":  string(message.Uid),
		"nsp":  message.Nsp,
		"type": strconv.Itoa(int(message.Type)),
	}

	if message.Data == nil {
		return rawMessage
	}

	mayContainBinary := message.Type == adapter.BROADCAST ||
		message.Type == adapter.FETCH_SOCKETS_RESPONSE ||
		message.Type == adapter.SERVER_SIDE_EMIT ||
		message.Type == adapter.SERVER_SIDE_EMIT_RESPONSE ||
		message.Type == adapter.BROADCAST_ACK

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

// Cleanup registers a cleanup callback.
func (r *cacheStreamsAdapter) Cleanup(cleanup func()) { r.cleanupFunc = cleanup }

// Close releases resources and invokes the cleanup callback.
func (r *cacheStreamsAdapter) Close() {
	defer r.ClusterAdapterWithHeartbeat.Close()
	if r.cleanupFunc != nil {
		r.cleanupFunc()
	}
}

// OnRawMessage decodes and dispatches a raw stream entry.
func (r *cacheStreamsAdapter) OnRawMessage(rawMessage RawClusterMessage, offset string) error {
	message, err := r.decode(rawMessage)
	if err != nil {
		return err
	}
	r.OnMessage(message, adapter.Offset(offset))
	return nil
}

func (r *cacheStreamsAdapter) decode(rawMessage RawClusterMessage) (*adapter.ClusterResponse, error) {
	messageType, err := strconv.ParseInt(rawMessage.Type(), 10, 0)
	if err != nil {
		return nil, fmt.Errorf("invalid message type: %w", err)
	}

	message := &adapter.ClusterMessage{
		Uid:  adapter.ServerId(rawMessage.Uid()),
		Nsp:  rawMessage.Nsp(),
		Type: adapter.MessageType(messageType),
	}

	data := rawMessage.Data()
	if data == "" {
		return message, nil
	}

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

	message.Data, err = r.decodeData(message.Type, rawData)
	if err != nil {
		return nil, err
	}

	return message, nil
}

func (r *cacheStreamsAdapter) decodeData(messageType adapter.MessageType, rawData any) (any, error) {
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

// PersistSession stores a session in the cache for later recovery.
func (r *cacheStreamsAdapter) PersistSession(session *socket.SessionToPersist) {
	cacheStreamsLog.Debug("persisting session: %v", session)

	sessionKey := r.opts.SessionKeyPrefix() + string(session.Pid)
	data, err := utils.MsgPack().Encode(session)
	if err != nil {
		cacheStreamsLog.Debug("failed to encode session: %s", err.Error())
		return
	}

	ttl := time.Duration(r.Nsp().Server().Opts().ConnectionStateRecovery().MaxDisconnectionDuration()) * time.Millisecond

	if err := r.cacheClient.Set(
		r.cacheClient.Context(),
		sessionKey,
		base64.StdEncoding.EncodeToString(data),
		ttl,
	); err != nil {
		r.cacheClient.Emit("error", err)
	}
}

// RestoreSession recovers a session from the cache and replays missed stream entries.
func (r *cacheStreamsAdapter) RestoreSession(pid socket.PrivateSessionId, offset string) (*socket.Session, error) {
	cacheStreamsLog.Debug("restoring session %s from offset %s", pid, offset)

	if !offsetRegex.MatchString(offset) {
		return nil, errors.New("invalid offset format")
	}

	sessionKey := r.opts.SessionKeyPrefix() + string(pid)

	rawSession, err := r.cacheClient.GetDel(r.cacheClient.Context(), sessionKey)
	if err != nil && !errors.Is(err, cache.ErrNil) {
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}
	if rawSession == "" {
		return nil, errors.New("session not found")
	}

	offsets, err := r.cacheClient.XRange(r.cacheClient.Context(), r.opts.StreamName(), offset, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to verify offset: %w", err)
	}
	if len(offsets) == 0 {
		return nil, errors.New("offset not found in stream")
	}

	rawSessionBytes, err := base64.StdEncoding.DecodeString(rawSession)
	if err != nil {
		return nil, fmt.Errorf("failed to decode session data: %w", err)
	}

	session := &socket.Session{}
	if err := utils.MsgPack().Decode(rawSessionBytes, &session.SessionToPersist); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	cacheStreamsLog.Debug("found session: %+v", session)

	r.collectMissedPackets(session, offset)

	return session, nil
}

func (r *cacheStreamsAdapter) collectMissedPackets(session *socket.Session, offset string) {
	broadcastTypeStr := strconv.Itoa(int(adapter.BROADCAST))

	for range restoreSessionMaxXRangeCalls {
		entries, err := r.cacheClient.XRangeN(
			r.cacheClient.Context(),
			r.opts.StreamName(),
			r.nextOffset(offset),
			"+",
			restoreSessionPageSize,
		)

		if err != nil || len(entries) == 0 {
			break
		}

		for _, entry := range entries {
			rawMessage := RawClusterMessage(entry.Values)

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

		if len(entries) < restoreSessionPageSize {
			break
		}
	}
}

func (cacheStreamsAdapter) nextOffset(offset string) string {
	timestamp, sequence, found := strings.Cut(offset, "-")
	if !found {
		return offset
	}
	if seqNum, err := strconv.ParseUint(sequence, 10, 64); err == nil {
		return timestamp + "-" + strconv.FormatUint(seqNum+1, 10)
	}
	return offset
}

func (cacheStreamsAdapter) shouldIncludePacket(sessionRooms *types.Set[socket.Room], opts *adapter.PacketOptions) bool {
	included := len(opts.Rooms) == 0
	if !included {
		if slices.ContainsFunc(opts.Rooms, sessionRooms.Has) {
			included = true
		}
	}
	if slices.ContainsFunc(opts.Except, sessionRooms.Has) {
		return false
	}
	return included
}
