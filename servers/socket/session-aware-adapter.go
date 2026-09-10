package socket

import (
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type (
	// SessionAwareAdapterBuilder is a builder for creating session-aware Adapter instances
	// that support connection state recovery.
	SessionAwareAdapterBuilder struct {
	}

	sessionAwareAdapter struct {
		Adapter

		maxDisconnectionDuration int64

		sessions     *types.Map[PrivateSessionId, *SessionWithTimestamp]
		packets      *types.Slice[*PersistedPacket]
		cleanupTimer *utils.Timer
	}
)

// New creates a new SessionAwareAdapter for the given Namespace.
func (*SessionAwareAdapterBuilder) New(nsp Namespace) Adapter {
	return NewSessionAwareAdapter(nsp)
}

func MakeSessionAwareAdapter() SessionAwareAdapter {
	s := &sessionAwareAdapter{
		Adapter: MakeAdapter(),

		sessions: &types.Map[PrivateSessionId, *SessionWithTimestamp]{},
		packets:  types.NewSlice[*PersistedPacket](),
	}

	s.Prototype(s)

	return s
}

func NewSessionAwareAdapter(nsp Namespace) SessionAwareAdapter {
	s := MakeSessionAwareAdapter()

	s.Construct(nsp)

	return s
}

func (s *sessionAwareAdapter) Construct(nsp Namespace) {
	s.Adapter.Construct(nsp)

	cleanupInterval := DefaultSessionCleanupInterval
	if connectionStateRecovery := nsp.Server().Opts().ConnectionStateRecovery(); connectionStateRecovery != nil {
		s.maxDisconnectionDuration = connectionStateRecovery.MaxDisconnectionDuration()
		if connectionStateRecovery.GetRawSessionCleanupInterval() != nil {
			cleanupInterval = connectionStateRecovery.SessionCleanupInterval()
		}
	} else {
		s.maxDisconnectionDuration = DefaultMaxDisconnectionDuration
	}

	s.cleanupTimer = utils.SetInterval(func() {
		threshold := time.Now().UnixMilli() - s.maxDisconnectionDuration
		s.sessions.Range(func(sessionId PrivateSessionId, session *SessionWithTimestamp) bool {
			if session.DisconnectedAt < threshold {
				s.sessions.Delete(sessionId)
			}
			return true
		})
		_, _ = s.packets.RangeAndSplice(func(packet *PersistedPacket, i int) (bool, int, int, []*PersistedPacket) {
			return packet.EmittedAt < threshold, 0, i + 1, nil
		}, true)
	}, cleanupInterval)
	// prevents the timer from keeping the process alive
	s.cleanupTimer.Unref()
}

// Close stops the session cleanup timer and releases resources.
func (s *sessionAwareAdapter) Close() {
	utils.ClearInterval(s.cleanupTimer)
	s.Adapter.Close()
}

func (s *sessionAwareAdapter) PersistSession(session *SessionToPersist) {
	s.sessions.Store(session.Pid, &SessionWithTimestamp{SessionToPersist: session, DisconnectedAt: time.Now().UnixMilli()})
}

func (s *sessionAwareAdapter) RestoreSession(pid PrivateSessionId, offset string) (*Session, error) {
	session, ok := s.sessions.Load(pid)
	if !ok {
		// the session may have expired
		return nil, nil
	}

	hasExpired := session.DisconnectedAt+s.maxDisconnectionDuration < time.Now().UnixMilli()
	if hasExpired {
		// the session has expired
		s.sessions.Delete(pid)
		return nil, nil
	}

	sessionRooms := session.Rooms.Keys()
	var missedPackets []any
	s.packets.DoRead(func(packets []*PersistedPacket) {
		for index, packet := range packets {
			if packet.Id != offset {
				continue
			}

			missedPackets = make([]any, 0, len(packets)-index-1)
			for _, packet := range packets[index+1:] {
				if shouldIncludePacket(sessionRooms, packet.Opts) {
					missedPackets = append(missedPackets, packet.Data)
				}
			}
			return
		}
	})

	if missedPackets == nil {
		// the offset may be too old
		return nil, nil
	}

	return &Session{
		SessionToPersist: session.SessionToPersist,
		MissedPackets:    missedPackets,
	}, nil
}

func (s *sessionAwareAdapter) Broadcast(packet *parser.Packet, opts *BroadcastOptions) {
	isEventPacket := packet.Type == parser.EVENT
	// packets with acknowledgement are not stored because the acknowledgement function cannot be serialized and
	// restored on another server upon reconnection
	withoutAcknowledgement := packet.Id == nil
	notVolatile := opts == nil || opts.Flags == nil || !opts.Flags.Volatile
	if isEventPacket && withoutAcknowledgement && notVolatile {
		data, _, _, err := types.MaterializeData(packet.Data)
		if err != nil {
			s.Emit("error", err)
			return
		}
		packet.Data = data
		id := utils.YeastDate()
		// the offset is stored at the end of the data array, so the client knows the ID of the last packet it has
		// processed (and the format is backward-compatible)
		packet.Data = append(utils.TryCast[[]any](packet.Data), id)

		s.packets.Push(&PersistedPacket{
			Id:        id,
			EmittedAt: time.Now().UnixMilli(),
			Data:      packet.Data,
			Opts:      opts,
		})
	}
	s.Adapter.Broadcast(packet, opts)
}

func shouldIncludePacket(sessionRooms []Room, opts *BroadcastOptions) bool {
	if opts == nil {
		return true
	}
	included := opts.Rooms == nil || opts.Rooms.Len() == 0
	for _, room := range sessionRooms {
		if opts.Except != nil && opts.Except.Has(room) {
			return false
		}
		if !included && opts.Rooms.Has(room) {
			included = true
		}
	}
	return included
}
