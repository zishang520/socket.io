// Package adapter implements a Valkey Streams-based adapter for Socket.IO clustering.
// Valkey Streams provide message persistence and enable session recovery across server restarts.
// Ephemeral messages (fetchSockets, serverSideEmit, broadcastWithAck) are sent via Valkey PUB/SUB
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
	"sync"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	valkeyStreamsLog = log.NewLog("socket.io-valkey-streams")

	// offsetRegex validates Valkey stream offset format (timestamp-sequence).
	offsetRegex = regexp.MustCompile(`^[0-9]+-[0-9]+$`)

	// errRestoreSessionReadLimit is returned when recovery cannot observe the
	// end of the stream within its bounded number of XRANGE calls.
	errRestoreSessionReadLimit = errors.New("session recovery exceeded XRANGE call limit")
)

const (
	// restoreSessionMaxXRangeCalls limits adapter-issued XRANGE calls while collecting missed packets.
	restoreSessionMaxXRangeCalls = 100
	restoreSessionPageSize       = 1000
)

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
	Valkey *valkey.ValkeyClient
	Opts   ValkeyStreamsAdapterOptionsInterface
}

// New creates a new Valkey Streams adapter for the given namespace.
func (sb *ValkeyStreamsAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	return NewValkeyStreamsAdapter(nsp, sb.Valkey, sb.Opts)
}

type valkeyStreamsAdapter struct {
	adapter.ClusterAdapter

	valkeyClient *valkey.ValkeyClient
	opts         *ValkeyStreamsAdapterOptions

	streamName    string
	publicChannel string

	pubSubs      []*valkey.ValkeyPubSub
	streamPoller *valkeyStreamsPoller

	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
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

func (r *valkeyStreamsAdapter) SetValkey(client *valkey.ValkeyClient) {
	r.valkeyClient = client
}

func (r *valkeyStreamsAdapter) SetOpts(opts any) {
	if options, ok := opts.(ValkeyStreamsAdapterOptionsInterface); ok {
		r.opts.Assign(options)
	}
}

func (r *valkeyStreamsAdapter) Construct(nsp socket.Namespace) {
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

	r.ctx, r.cancel = context.WithCancel(r.valkeyClient.Context())
	r.streamName = valkey.StreamNameForNamespace(
		r.opts.StreamName(),
		nsp.Name(),
		r.opts.StreamCount(),
	)

	r.publicChannel = r.opts.ChannelPrefix() + "#" + nsp.Name() + "#"
	privateChannel := r.publicChannel + string(r.Uid()) + "#"

	if r.opts.UseShardedPubSub() {
		// Public and private channels normally belong to different hash slots, so
		// they require independent sharded subscriptions with valkey-go.
		r.pubSubs = []*valkey.ValkeyPubSub{
			r.valkeyClient.SSubscribe(r.ctx, r.publicChannel),
			r.valkeyClient.SSubscribe(r.ctx, privateChannel),
		}
	} else {
		r.pubSubs = []*valkey.ValkeyPubSub{
			r.valkeyClient.Subscribe(r.ctx, r.publicChannel, privateChannel),
		}
	}
	for _, pubSub := range r.pubSubs {
		go r.handlePubSubMessages(pubSub)
	}
	poller, err := acquireValkeyStreamsPoller(r)
	r.streamPoller = poller
	if r.ctx.Err() != nil {
		releaseValkeyStreamsPoller(poller, r)
		return
	}
	if err != nil {
		valkeyStreamsLog.Debug("error reading stream tail: %s", err.Error())
		r.valkeyClient.Emit("error", err)
	}
}

func (r *valkeyStreamsAdapter) handlePubSubMessages(pubSub *valkey.ValkeyPubSub) {
	for {
		message, err := pubSub.ReceiveMessage(r.ctx)
		if err != nil {
			return
		}
		r.onPubSubMessage([]byte(message.Message))
	}
}

func (r *valkeyStreamsAdapter) onPubSubMessage(payload []byte) {
	if r.ctx.Err() != nil {
		return
	}
	message, err := adapter.DecodeClusterMessage(payload)
	if err != nil {
		valkeyStreamsLog.Debug("invalid PUB/SUB message format: %s", err.Error())
		return
	}
	r.OnMessage(message, "")
}

func (r *valkeyStreamsAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	publishCtx := r.valkeyClient.Context()
	valkeyStreamsLog.Debug("publishing message: %+v", message)

	if isEphemeral(message) {
		payload, err := adapter.EncodeClusterMessageMsgpack(message)
		if err != nil {
			return "", fmt.Errorf("failed to encode ephemeral message: %w", err)
		}
		if r.opts.UseShardedPubSub() {
			return "", r.valkeyClient.SPublish(publishCtx, r.publicChannel, payload)
		}
		return "", r.valkeyClient.Publish(publishCtx, r.publicChannel, payload)
	}

	rawMessage, err := valkey.EncodeStreamMessage(message, r.opts.OnlyPlaintext())
	if err != nil {
		return "", fmt.Errorf("failed to encode stream message: %w", err)
	}
	entryID, err := r.valkeyClient.XAdd(publishCtx, r.streamName, rawMessage, r.opts.MaxLen())
	if err != nil {
		return "", err
	}
	return adapter.Offset(entryID), nil
}

func (r *valkeyStreamsAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	responseChannel := r.opts.ChannelPrefix() + "#" + r.Nsp().Name() + "#" + string(requesterUid) + "#"
	payload, err := adapter.EncodeClusterMessageMsgpack(response)
	if err != nil {
		return fmt.Errorf("failed to encode response: %w", err)
	}
	if r.opts.UseShardedPubSub() {
		return r.valkeyClient.SPublish(r.valkeyClient.Context(), responseChannel, payload)
	}
	return r.valkeyClient.Publish(r.valkeyClient.Context(), responseChannel, payload)
}

func (r *valkeyStreamsAdapter) ServerCount() (int64, error) {
	var counts map[string]int64
	var err error
	if r.opts.UseShardedPubSub() {
		counts, err = r.valkeyClient.PubSubShardNumSub(r.ctx, r.publicChannel)
	} else {
		counts, err = r.valkeyClient.PubSubNumSub(r.ctx, r.publicChannel)
	}
	if err != nil {
		return 0, err
	}
	return counts[r.publicChannel], nil
}

func (r *valkeyStreamsAdapter) Close() {
	r.closeOnce.Do(func() {
		r.ClusterAdapter.Close()
		if r.cancel != nil {
			r.cancel()
		}
		for _, pubSub := range r.pubSubs {
			_ = pubSub.Close()
		}
		if r.streamPoller != nil {
			releaseValkeyStreamsPoller(r.streamPoller, r)
		}
	})
}

func (r *valkeyStreamsAdapter) OnRawMessage(rawMessage RawClusterMessage, offset string) error {
	message, err := valkey.DecodeStreamMessage(rawMessage)
	if err != nil {
		return err
	}
	r.OnMessage(message, adapter.Offset(offset))
	return nil
}

func (r *valkeyStreamsAdapter) PersistSession(session *socket.SessionToPersist) {
	valkeyStreamsLog.Debug("persisting session: %v", session)
	if err := r.ctx.Err(); err != nil {
		return
	}

	maxDisconnectionDuration := r.Nsp().Server().Opts().ConnectionStateRecovery().MaxDisconnectionDuration()
	if maxDisconnectionDuration <= 0 {
		r.valkeyClient.Emit("error", fmt.Errorf(
			"valkey streams: maxDisconnectionDuration must be positive: %dms",
			maxDisconnectionDuration,
		))
		return
	}
	if maxDisconnectionDuration > math.MaxInt64/int64(time.Millisecond) {
		r.valkeyClient.Emit("error", fmt.Errorf(
			"valkey streams: maxDisconnectionDuration overflows time.Duration: %dms",
			maxDisconnectionDuration,
		))
		return
	}

	sessionKey := r.opts.SessionKeyPrefix() + string(session.Pid)
	data, err := utils.MsgPack().Encode(session)
	if err != nil {
		r.valkeyClient.Emit("error", fmt.Errorf("valkey streams: failed to encode session: %w", err))
		return
	}

	if err := r.valkeyClient.Set(
		r.ctx,
		sessionKey,
		base64.StdEncoding.EncodeToString(data),
		time.Duration(maxDisconnectionDuration)*time.Millisecond,
	); err != nil && r.ctx.Err() == nil {
		r.valkeyClient.Emit("error", err)
	}
}

func (r *valkeyStreamsAdapter) RestoreSession(pid socket.PrivateSessionId, offset string) (*socket.Session, error) {
	valkeyStreamsLog.Debug("restoring session %s from offset %s", pid, offset)
	if err := r.ctx.Err(); err != nil {
		return nil, err
	}
	if !offsetRegex.MatchString(offset) {
		return nil, errors.New("invalid offset format")
	}

	sessionKey := r.opts.SessionKeyPrefix() + string(pid)
	rawSession, err := r.valkeyClient.GetDel(r.ctx, sessionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve session: %w", err)
	}
	if rawSession == "" {
		return nil, errors.New("session not found")
	}

	offsets, err := r.valkeyClient.XRangeN(r.ctx, r.streamName, offset, offset, 1)
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

	valkeyStreamsLog.Debug("found session: %+v", session)
	if err := r.collectMissedPackets(session, offset); err != nil {
		return nil, err
	}
	return session, nil
}

func (r *valkeyStreamsAdapter) collectMissedPackets(session *socket.Session, offset string) error {
	broadcastType := strconv.Itoa(int(adapter.BROADCAST))

	for range restoreSessionMaxXRangeCalls {
		if err := r.ctx.Err(); err != nil {
			return err
		}
		entries, err := r.valkeyClient.XRangeN(
			r.ctx,
			r.streamName,
			"("+offset,
			"+",
			restoreSessionPageSize,
		)
		if err != nil {
			return fmt.Errorf("failed to retrieve missed packets: %w", err)
		}
		if len(entries) == 0 {
			return nil
		}

		for _, entry := range entries {
			rawMessage := RawClusterMessage(entry.FieldValues)
			if rawMessage.Nsp() == r.Nsp().Name() && rawMessage.Type() == broadcastType {
				message, err := valkey.DecodeStreamMessage(rawMessage)
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

func (*valkeyStreamsAdapter) shouldIncludePacket(sessionRooms *types.Set[socket.Room], opts *adapter.PacketOptions) bool {
	included := len(opts.Rooms) == 0 || _slices.ContainsFunc(opts.Rooms, sessionRooms.Has)
	if _slices.ContainsFunc(opts.Except, sessionRooms.Has) {
		return false
	}
	return included
}
