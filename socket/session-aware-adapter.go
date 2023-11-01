package socket

import (
	"sync"
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

		sessions   *types.Map[PrivateSessionId, *SessionWithTimestamp]
		packets    []*PersistedPacket
		mu_packets sync.RWMutex
	}
)

func (*SessionAwareAdapterBuilder) New(nsp NamespaceInterface) Adapter {
	s := &sessionAwareAdapter{}
	s.Adapter = new(AdapterBuilder).New(nsp)
	s.SetBroadcast(s.broadcast)

	s.maxDisconnectionDuration = nsp.Server().Opts().ConnectionStateRecovery().MaxDisconnectionDuration()

	s.sessions = &types.Map[PrivateSessionId, *SessionWithTimestamp]{}

	timer := utils.SetInterval(func() {
		threshold := time.Now().UnixMilli() - s.maxDisconnectionDuration
		s.sessions.Range(func(sessionId PrivateSessionId, session *SessionWithTimestamp) bool {
			if session.DisconnectedAt < threshold {
				s.sessions.Delete(sessionId)
			}
			return true
		})
		s.mu_packets.Lock()
		defer s.mu_packets.Unlock()

		for i, packet := range s.packets {
			if packet.EmittedAt < threshold {
				copy(s.packets, s.packets[i+1:])
				s.packets = s.packets[:len(s.packets)-i-1]
				break
			}
		}
	}, 60*1000*time.Millisecond)
	// prevents the timer from keeping the process alive
	timer.Unref()
	return s
}

func (s *sessionAwareAdapter) PersistSession(session *SessionToPersist) {
	_session := &SessionWithTimestamp{SessionToPersist: session, DisconnectedAt: time.Now().UnixMilli()}
	s.sessions.Store(_session.Pid, _session)
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

	s.mu_packets.RLock()
	defer s.mu_packets.RUnlock()

	// Find the index of the packet with the given offset
	index := -1
	for i, packet := range s.packets {
		if packet.Id == offset {
			index = i
			break
		}
	}

	if index == -1 {
		return nil, nil
	}

	// Use a pre-allocated slice to avoid memory allocation in the loop
	missedPackets := make([]any, 0, len(s.packets)-index-1)
	missedNum := 0
	// Iterate over the packets and append the data of those that should be included
	for i := index + 1; i < len(s.packets); i++ {
		packet := s.packets[i]
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

func (s *sessionAwareAdapter) GetBroadcast() func(*parser.Packet, *BroadcastOptions) {
	return s.broadcast
}

func (s *sessionAwareAdapter) broadcast(packet *parser.Packet, opts *BroadcastOptions) {
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

		s.mu_packets.Lock()
		defer s.mu_packets.Unlock()

		s.packets = append(s.packets, &PersistedPacket{
			Id:        id,
			EmittedAt: time.Now().UnixMilli(),
			Data:      packet.Data,
			Opts:      opts,
		})
	}
	s.Adapter.GetBroadcast()(packet, opts)
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
