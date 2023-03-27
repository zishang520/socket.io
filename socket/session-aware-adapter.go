package socket

import (
	"sync"
	"time"

	"github.com/zishang520/engine.io/utils"
	"github.com/zishang520/socket.io/parser"
)

type SessionAwareAdapter struct {
	*adapter

	maxDisconnectionDuration int64

	sessions *sync.Map
	packets  []*PersistedPacket
}

func (*SessionAwareAdapter) New(nsp NamespaceInterface) Adapter {
	s := &SessionAwareAdapter{}
	s.adapter = &adapter{}
	s.adapter.New(nsp)
	s._broadcast = s.broadcast

	s.maxDisconnectionDuration =
		nsp.Server().opts.ConnectionStateRecovery().MaxDisconnectionDuration()

	s.sessions = &sync.Map{}

	timer := utils.SetInterval(func() {
		threshold := time.Now().UnixMilli() - s.maxDisconnectionDuration
		s.sessions.Range(func(sessionId any, session any) bool {
			if session.(*SessionWithTimestamp).DisconnectedAt < threshold {
				s.sessions.Delete(sessionId)
			}
			return true
		})
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

func (s *SessionAwareAdapter) PersistSession(session *SessionToPersist) {
	_session := &SessionWithTimestamp{SessionToPersist: session, DisconnectedAt: time.Now().UnixMilli()}
	s.sessions.Store(_session.Pid, _session)
}

func (s *SessionAwareAdapter) RestoreSession(pid PrivateSessionId, offset string) *Session {
	session, ok := s.sessions.Load(pid)
	if !ok {
		// the session may have expired
		return nil
	}

	_session, ok := session.(*SessionWithTimestamp)
	if !ok {
		// This session is not of type *SessionWithTimestamp
		return nil
	}

	hasExpired := _session.DisconnectedAt+s.maxDisconnectionDuration < time.Now().UnixMilli()
	if hasExpired {
		// the session has expired
		s.sessions.Delete(pid)
		return nil
	}

	// Find the index of the packet with the given offset
	index := sort.Search(len(s.Packets), func(i int) bool { return s.Packets[i].Id >= offset })
	if index == len(s.Packets) || s.Packets[index].Id != offset {
		// the offset may be too old
		return nil
	}

	// Use a pre-allocated slice to avoid memory allocation in the loop
	missedPackets := make([]any, 0, len(s.Packets)-index-1)

	// Iterate over the packets and append the data of those that should be included
	for i := index + 1; i < len(s.Packets); i++ {
		packet := s.Packets[i]
		if shouldIncludePacket(session.Rooms, packet.Opts) {
			missedPackets = append(missedPackets, packet.Data)
		}
	}

	// Create a new Session object and return it
	return &Session{
		SessionToPersist: session.SessionToPersist,
		MissedPackets:    missedPackets,
	}
}

func (s *SessionAwareAdapter) broadcast(packet *parser.Packet, opts *BroadcastOptions) {
	isEventPacket := packet.Type == parser.EVENT
	// packets with acknowledgement are not stored because the acknowledgement function cannot be serialized and
	// restored on another server upon reconnection
	withoutAcknowledgement := packet.Id == nil
	notVolatile := opts.Flags != nil && opts.Flags.Volatile == false

	if isEventPacket && withoutAcknowledgement && notVolatile {
		id := utils.YeastDate()
		// the offset is stored at the end of the data array, so the client knows the ID of the last packet it has
		// processed (and the format is backward-compatible)
		packet.Data = any(append(packet.Data.([]any), any(id)))
		s.packets = append(s.packets, &PersistedPacket{
			Id:        id,
			EmittedAt: time.Now().UnixMilli(),
			Data:      packet.Data,
			Opts:      opts,
		})
	}
	s.adapter.broadcast(packet, opts)
}
