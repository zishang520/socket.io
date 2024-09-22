package socket

import (
	"time"

	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/engine.io/v2/utils"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

type (
	SessionAwareAdapterBuilder struct {
	}

	sessionAwareAdapter struct {
		Adapter

		maxDisconnectionDuration int64

		sessions *types.Map[PrivateSessionId, *SessionWithTimestamp]
		packets  *types.Slice[*PersistedPacket]
	}
)

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
	s.maxDisconnectionDuration = nsp.Server().Opts().ConnectionStateRecovery().MaxDisconnectionDuration()

	timer := utils.SetInterval(func() {
		threshold := time.Now().UnixMilli() - s.maxDisconnectionDuration
		s.sessions.Range(func(sessionId PrivateSessionId, session *SessionWithTimestamp) bool {
			if session.DisconnectedAt < threshold {
				s.sessions.Delete(sessionId)
			}
			return true
		})
		s.packets.RangeAndSplice(func(packet *PersistedPacket, i int) (bool, int, int, []*PersistedPacket) {
			return packet.EmittedAt < threshold, 0, i + 1, nil
		}, true)
	}, 60*1000*time.Millisecond)
	// prevents the timer from keeping the process alive
	timer.Unref()
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

	// Find the index of the packet with the given offset
	index := s.packets.FindIndex(func(packet *PersistedPacket) bool {
		return packet.Id == offset
	})

	if index == -1 {
		// the offset may be too old
		return nil, nil
	}

	// Use a pre-allocated slice to avoid memory allocation in the loop
	missedPackets := make([]any, 0, s.packets.Len()-index-1)
	missedNum := 0
	// Iterate over the packets and append the data of those that should be included
	for i := index + 1; i < s.packets.Len(); i++ {
		packet, err := s.packets.Get(i)
		if err != nil {
			break
		}
		if shouldIncludePacket(session.Rooms, packet.Opts) {
			missedPackets = append(missedPackets, packet.Data)
			missedNum++
		}
	}

	// Create a new Session object and return it
	return &Session{
		SessionToPersist: session.SessionToPersist,
		MissedPackets:    missedPackets[:missedNum],
	}, nil
}

func (s *sessionAwareAdapter) Broadcast(packet *parser.Packet, opts *BroadcastOptions) {
	isEventPacket := packet.Type == parser.EVENT
	// packets with acknowledgement are not stored because the acknowledgement function cannot be serialized and
	// restored on another server upon reconnection
	withoutAcknowledgement := packet.Id == nil
	notVolatile := opts == nil || opts.Flags == nil || opts.Flags.Volatile == false
	if isEventPacket && withoutAcknowledgement && notVolatile {
		id := utils.YeastDate()
		// the offset is stored at the end of the data array, so the client knows the ID of the last packet it has
		// processed (and the format is backward-compatible)
		packet.Data = append(packet.Data.([]any), id)

		s.packets.Push(&PersistedPacket{
			Id:        id,
			EmittedAt: time.Now().UnixMilli(),
			Data:      packet.Data,
			Opts:      opts,
		})
	}
	s.Adapter.Broadcast(packet, opts)
}

func shouldIncludePacket(sessionRooms *types.Set[Room], opts *BroadcastOptions) bool {
	included := opts.Rooms.Len() == 0
	notExcluded := true
	for _, room := range sessionRooms.Keys() {
		if included && !notExcluded {
			break
		}
		if !included && opts.Rooms.Has(room) {
			included = true
		}
		if notExcluded && opts.Except.Has(room) {
			notExcluded = false
		}
	}
	return included && notExcluded
}
