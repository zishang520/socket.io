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
	xReadBlockTimeout            = 5000 * time.Millisecond
	defaultHeartbeatInterval     = 5_000
	defaultHeartbeatTimeout      = 10_000
)

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

func (sb *ValkeyStreamsAdapterBuilder) startPolling(ctx context.Context, options ValkeyStreamsAdapterOptionsInterface) {
	offset := "$"

	for {
		select {
		case <-ctx.Done():
			sb.polling.Store(false)
			return
		default:
		}

		entries, err := sb.Valkey.XRead(ctx, []string{options.StreamName()}, offset, options.ReadCount(), xReadBlockTimeout)
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

	adapterInstance := NewValkeyStreamsAdapter(nsp, sb.Valkey, options)
	sb.namespaceToAdapters.Store(nsp.Name(), adapterInstance)

	if sb.polling.CompareAndSwap(false, true) {
		ctx, cancelFunc := context.WithCancel(sb.Valkey.Context)
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

type valkeyStreamsAdapter struct {
	adapter.ClusterAdapterWithHeartbeat

	valkeyClient *valkey.ValkeyClient
	opts         *ValkeyStreamsAdapterOptions
	cleanupFunc  types.Callable
}

// MakeValkeyStreamsAdapter creates a new uninitialized valkeyStreamsAdapter.
func MakeValkeyStreamsAdapter() ValkeyStreamsAdapter {
	a := &valkeyStreamsAdapter{
		ClusterAdapterWithHeartbeat: adapter.MakeClusterAdapterWithHeartbeat(),
		opts:                        DefaultValkeyStreamsAdapterOptions(),
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
	r.ClusterAdapterWithHeartbeat.SetOpts(opts)
	if options, ok := opts.(ValkeyStreamsAdapterOptionsInterface); ok {
		r.opts.Assign(options)
	}
}

func (r *valkeyStreamsAdapter) Construct(nsp socket.Namespace) {
	r.ClusterAdapterWithHeartbeat.Construct(nsp)
	r.Init()
}

// DoPublish publishes a cluster message to the Valkey stream.
func (r *valkeyStreamsAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	valkeyStreamsLog.Debug("publishing message: %+v", message)

	encoded := r.encode(message)
	entryID, err := r.valkeyClient.XAdd(
		r.valkeyClient.Context,
		r.opts.StreamName(),
		r.opts.MaxLen(),
		encoded,
	)
	if err != nil {
		return "", err
	}
	return adapter.Offset(entryID), nil
}

// DoPublishResponse publishes a response message to the Valkey stream.
func (r *valkeyStreamsAdapter) DoPublishResponse(_ adapter.ServerId, response *adapter.ClusterResponse) error {
	_, err := r.DoPublish(response)
	return err
}

func (valkeyStreamsAdapter) encode(message *adapter.ClusterResponse) map[string]any {
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

func (r *valkeyStreamsAdapter) Cleanup(cleanup func()) { r.cleanupFunc = cleanup }

func (r *valkeyStreamsAdapter) Close() {
	defer r.ClusterAdapterWithHeartbeat.Close()
	if r.cleanupFunc != nil {
		r.cleanupFunc()
	}
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

	offsets, err := r.valkeyClient.XRange(r.valkeyClient.Context, r.opts.StreamName(), offset, offset)
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
			r.opts.StreamName(),
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
