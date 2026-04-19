// Package adapter implements a Valkey Streams-based adapter for Socket.IO clustering.
// Valkey Streams provide message persistence and enable session recovery across server restarts.
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
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	valkeyStreamsLog = log.NewLog("socket.io-valkey-streams")
	offsetRegex      = regexp.MustCompile(`^[0-9]+-[0-9]+$`)
)

const (
	restoreSessionMaxXRangeCalls = 100
	restoreSessionPageSize       = 1000
)

// hashCode computes a hash code for the given string, matching the Node.js implementation.
// This is used to deterministically map namespaces to streams when streamCount > 1.
func hashCode(str string) int {
	hash := 0
	for _, chr := range str {
		hash = hash*31 + int(chr)
		hash &= 0x7FFFFFFF
	}
	return hash
}

// computeStreamName determines which stream a namespace should use.
func computeStreamName(namespaceName string, opts ValkeyStreamsAdapterOptionsInterface) string {
	if opts.StreamCount() <= 1 {
		return opts.StreamName()
	}
	i := hashCode(namespaceName) % opts.StreamCount()
	return opts.StreamName() + "-" + strconv.Itoa(i)
}

// isEphemeral determines whether a message should be sent via PUB/SUB instead of Streams.
func isEphemeral(message *adapter.ClusterMessage) bool {
	if message.Type == adapter.BROADCAST {
		if data, ok := message.Data.(*adapter.BroadcastMessage); ok {
			return data.RequestId != nil
		}
	}
	return message.Type == adapter.SERVER_SIDE_EMIT || message.Type == adapter.FETCH_SOCKETS
}

// ValkeyStreamsAdapterBuilder creates Valkey Streams adapters for Socket.IO namespaces.
type ValkeyStreamsAdapterBuilder struct {
	// Valkey is the Valkey client used for stream operations.
	Valkey *valkey.ValkeyClient
	// Opts contains configuration options for the streams adapter.
	Opts ValkeyStreamsAdapterOptionsInterface

	namespaceToAdapters types.Map[string, ValkeyStreamsAdapter]
	polling             atomic.Bool
	cancelFunc          types.Atomic[context.CancelFunc]
}

func (sb *ValkeyStreamsAdapterBuilder) startPolling(ctx context.Context, streamName string, options ValkeyStreamsAdapterOptionsInterface) {
	offset := "$"

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		entries, err := sb.Valkey.XRead(ctx, []string{streamName}, offset, options.ReadCount(), time.Duration(options.BlockTimeInMs())*time.Millisecond)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				continue
			}
			valkeyStreamsLog.Debug("error reading from stream: %s", err.Error())
			time.Sleep(1 * time.Second)
			continue
		}

		for _, entry := range entries {
			valkeyStreamsLog.Debug("processing entry %s", entry.ID)

			message := RawClusterMessage(toAnyMap(entry.FieldValues))
			if nsp := message.Nsp(); nsp != "" {
				if adapterInst, exists := sb.namespaceToAdapters.Load(nsp); exists {
					if err := adapterInst.OnRawMessage(message, entry.ID); err != nil {
						valkeyStreamsLog.Debug("error processing message: %s", err.Error())
					}
				}
			}

			offset = entry.ID
		}
	}
}

// New creates a new Valkey Streams adapter for the given namespace.
func (sb *ValkeyStreamsAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	options := DefaultValkeyStreamsAdapterOptions().Assign(sb.Opts)

	if options.GetRawStreamName() == nil {
		options.SetStreamName(DefaultStreamName)
	}
	if options.GetRawStreamCount() == nil {
		options.SetStreamCount(DefaultStreamCount)
	}
	if options.GetRawChannelPrefix() == nil {
		options.SetChannelPrefix(DefaultStreamChannelPrefix)
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

	adapterInstance := NewValkeyStreamsAdapter(nsp, sb.Valkey, options)
	sb.namespaceToAdapters.Store(nsp.Name(), adapterInstance)

	if sb.polling.CompareAndSwap(false, true) {
		ctx, cancelFunc := context.WithCancel(sb.Valkey.Context)
		sb.cancelFunc.Store(cancelFunc)

		if options.StreamCount() <= 1 {
			go sb.startPolling(ctx, options.StreamName(), options)
		} else {
			for i := range options.StreamCount() {
				streamName := options.StreamName() + "-" + strconv.Itoa(i)
				go sb.startPolling(ctx, streamName, options)
			}
		}
	}

	adapterInstance.Cleanup(func() {
		sb.namespaceToAdapters.Delete(nsp.Name())
		if sb.namespaceToAdapters.Len() == 0 {
			sb.polling.Store(false)
			if cancelFunc := sb.cancelFunc.Load(); cancelFunc != nil {
				cancelFunc()
				sb.cancelFunc.Store(nil)
			}
		}
	})

	return adapterInstance
}

type valkeyStreamsAdapter struct {
	adapter.ClusterAdapter

	valkeyClient *valkey.ValkeyClient
	opts         *ValkeyStreamsAdapterOptions
	cleanupFunc  types.Callable

	streamName    string               // The specific stream for this namespace
	publicChannel string               // PUB/SUB channel for ephemeral messages
	pubsub        *valkey.ValkeyPubSub // PUB/SUB subscription for this adapter

	ctx    context.Context
	cancel context.CancelFunc
}

// MakeValkeyStreamsAdapter creates a new uninitialized valkeyStreamsAdapter.
func MakeValkeyStreamsAdapter() ValkeyStreamsAdapter {
	a := &valkeyStreamsAdapter{
		ClusterAdapter: adapter.MakeClusterAdapter(),
		opts:           DefaultValkeyStreamsAdapterOptions(),
	}
	a.Prototype(a)
	return a
}

// NewValkeyStreamsAdapter creates and initializes a new Valkey Streams adapter.
func NewValkeyStreamsAdapter(nsp socket.Namespace, client *valkey.ValkeyClient, opts any) ValkeyStreamsAdapter {
	a := MakeValkeyStreamsAdapter()
	a.SetValkey(client)
	a.SetOpts(opts)
	a.Construct(nsp)
	return a
}

func (r *valkeyStreamsAdapter) SetValkey(client *valkey.ValkeyClient) { r.valkeyClient = client }

func (r *valkeyStreamsAdapter) SetOpts(opts any) {
	if options, ok := opts.(ValkeyStreamsAdapterOptionsInterface); ok {
		r.opts.Assign(options)
	}
}

func (r *valkeyStreamsAdapter) Construct(nsp socket.Namespace) {
	r.ClusterAdapter.Construct(nsp)

	r.ctx, r.cancel = context.WithCancel(r.valkeyClient.Context)

	// Each namespace is routed to a specific stream to ensure ordering
	r.streamName = computeStreamName(nsp.Name(), r.opts)

	// Set up PUB/SUB channels matching Node.js format: prefix#nsp# and prefix#nsp#uid#
	r.publicChannel = r.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	privateChannel := r.opts.ChannelPrefix() + "#" + nsp.Name() + "#" + string(r.Uid()) + "#"

	// Subscribe to both public and private channels for PUB/SUB messages
	if r.opts.UseShardedPubSub() {
		r.pubsub = r.valkeyClient.SSubscribe(r.ctx, r.publicChannel, privateChannel)
	} else {
		r.pubsub = r.valkeyClient.Subscribe(r.ctx, r.publicChannel, privateChannel)
	}
	go r.handlePubSubMessages()
}

// handlePubSubMessages listens for PUB/SUB messages (ephemeral messages and responses).
func (r *valkeyStreamsAdapter) handlePubSubMessages() {
	defer func() { _ = r.pubsub.Close() }()
	for {
		msg, err := r.pubsub.ReceiveMessage(r.ctx)
		if err != nil {
			if errors.Is(err, valkey.ErrValkeyPubSubClosed) || r.ctx.Err() != nil {
				return
			}
			valkeyStreamsLog.Debug("error receiving PUB/SUB message: %s", err.Error())
			continue
		}

		var message adapter.ClusterMessage
		if err := utils.MsgPack().Decode([]byte(msg.Payload), &message); err != nil {
			valkeyStreamsLog.Debug("invalid PUB/SUB message format: %s", err.Error())
			continue
		}

		r.OnMessage(&message, "")
	}
}

// DoPublish publishes a cluster message.
// Ephemeral messages (fetchSockets, serverSideEmit, broadcastWithAck) go via PUB/SUB.
// Durable messages (broadcast, socketsJoin, etc.) go via Valkey Streams.
func (r *valkeyStreamsAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	valkeyStreamsLog.Debug("publishing message: %+v", message)

	if isEphemeral(message) {
		// Ephemeral messages are sent via Valkey PUB/SUB
		payload, err := utils.MsgPack().Encode(message)
		if err != nil {
			return "", fmt.Errorf("failed to encode ephemeral message: %w", err)
		}
		if r.opts.UseShardedPubSub() {
			err = r.valkeyClient.SPublish(r.ctx, r.publicChannel, payload)
		} else {
			err = r.valkeyClient.Publish(r.ctx, r.publicChannel, payload)
		}
		if err != nil {
			return "", err
		}
		return "", nil
	}

	// Durable messages are sent via Valkey Streams
	encoded := r.encode(message)
	entryID, err := r.valkeyClient.XAdd(
		r.valkeyClient.Context,
		r.streamName,
		r.opts.MaxLen(),
		encoded,
	)
	if err != nil {
		return "", err
	}
	return adapter.Offset(entryID), nil
}

// DoPublishResponse publishes a response message via PUB/SUB to the requester's private channel.
func (r *valkeyStreamsAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	responseChannel := r.opts.ChannelPrefix() + "#" + r.Nsp().Name() + "#" + string(requesterUid) + "#"
	payload, err := utils.MsgPack().Encode(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}
	if r.opts.UseShardedPubSub() {
		return r.valkeyClient.SPublish(r.ctx, responseChannel, payload)
	}
	return r.valkeyClient.Publish(r.ctx, responseChannel, payload)
}

func (r *valkeyStreamsAdapter) encode(message *adapter.ClusterResponse) map[string]any {
	rawMessage := map[string]any{
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
func (r *valkeyStreamsAdapter) ServerCount() int64 {
	var result map[string]int64
	var err error
	if r.opts.UseShardedPubSub() {
		result, err = r.valkeyClient.PubSubShardNumSub(r.ctx, r.publicChannel)
	} else {
		result, err = r.valkeyClient.PubSubNumSub(r.ctx, r.publicChannel)
	}
	if err != nil {
		valkeyStreamsLog.Debug("error getting server count: %s", err.Error())
		return 1
	}
	if count, ok := result[r.publicChannel]; ok {
		return count
	}
	return 1
}

func (r *valkeyStreamsAdapter) Cleanup(cleanup func()) { r.cleanupFunc = cleanup }

func (r *valkeyStreamsAdapter) Close() {
	defer r.cancel()

	if r.pubsub != nil {
		_ = r.pubsub.Close()
	}

	if r.cleanupFunc != nil {
		r.cleanupFunc()
	}

	r.ClusterAdapter.Close()
}

// OnRawMessage processes a raw message from the Valkey stream.
func (r *valkeyStreamsAdapter) OnRawMessage(rawMessage RawClusterMessage, offset string) error {
	message, err := r.decode(rawMessage)
	if err != nil {
		return err
	}
	r.OnMessage(message, adapter.Offset(offset))
	return nil
}

func (r *valkeyStreamsAdapter) decode(rawMessage RawClusterMessage) (*adapter.ClusterResponse, error) {
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

func (r *valkeyStreamsAdapter) decodeData(messageType adapter.MessageType, rawData any) (any, error) {
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

// PersistSession saves a session to Valkey for later recovery.
func (r *valkeyStreamsAdapter) PersistSession(session *socket.SessionToPersist) {
	valkeyStreamsLog.Debug("persisting session: %v", session)

	sessionKey := r.opts.SessionKeyPrefix() + string(session.Pid)
	data, err := utils.MsgPack().Encode(session)
	if err != nil {
		valkeyStreamsLog.Debug("failed to encode session: %s", err.Error())
		return
	}

	ttl := time.Duration(r.Nsp().Server().Opts().ConnectionStateRecovery().MaxDisconnectionDuration()) * time.Millisecond

	if err := r.valkeyClient.Set(
		r.valkeyClient.Context,
		sessionKey,
		base64.StdEncoding.EncodeToString(data),
		ttl,
	); err != nil {
		r.valkeyClient.Emit("error", err)
	}
}

// RestoreSession restores a session from Valkey and collects missed packets.
func (r *valkeyStreamsAdapter) RestoreSession(pid socket.PrivateSessionId, offset string) (*socket.Session, error) {
	valkeyStreamsLog.Debug("restoring session %s from offset %s", pid, offset)

	if !offsetRegex.MatchString(offset) {
		return nil, errors.New("invalid offset format")
	}

	sessionKey := r.opts.SessionKeyPrefix() + string(pid)

	rawSession, err := r.valkeyClient.GetDel(r.valkeyClient.Context, sessionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}
	if rawSession == "" {
		return nil, errors.New("session not found")
	}

	offsets, err := r.valkeyClient.XRange(r.valkeyClient.Context, r.streamName, offset, offset)
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

	valkeyStreamsLog.Debug("found session: %+v", session)
	r.collectMissedPackets(session, offset)

	return session, nil
}

func (r *valkeyStreamsAdapter) collectMissedPackets(session *socket.Session, offset string) {
	broadcastTypeStr := strconv.Itoa(int(adapter.BROADCAST))

	for range restoreSessionMaxXRangeCalls {
		entries, err := r.valkeyClient.XRangeN(
			r.valkeyClient.Context,
			r.streamName,
			r.nextOffset(offset),
			"+",
			restoreSessionPageSize,
		)

		if err != nil || len(entries) == 0 {
			break
		}

		for _, entry := range entries {
			rawMessage := RawClusterMessage(toAnyMap(entry.FieldValues))

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

func (valkeyStreamsAdapter) nextOffset(offset string) string {
	timestamp, sequence, found := strings.Cut(offset, "-")
	if !found {
		return offset
	}
	if seqNum, err := strconv.ParseUint(sequence, 10, 64); err == nil {
		return timestamp + "-" + strconv.FormatUint(seqNum+1, 10)
	}
	return offset
}

func (valkeyStreamsAdapter) shouldIncludePacket(sessionRooms *types.Set[socket.Room], opts *adapter.PacketOptions) bool {
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

// toAnyMap converts a map[string]string to map[string]any for RawClusterMessage compatibility.
func toAnyMap(m map[string]string) map[string]any {
	result := make(map[string]any, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}
