package socket

import (
	"sync"
	"time"

	"github.com/zishang520/engine.io/utils"
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
