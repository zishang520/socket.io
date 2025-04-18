package adapter

import (
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// SessionAwareAdapterBuilder is a builder for creating SessionAwareAdapter instances.
type (
	SessionAwareAdapterBuilder struct {
	}
)

// New creates a new SessionAwareAdapter for the given Namespace.
func (*SessionAwareAdapterBuilder) New(nsp socket.Namespace) Adapter {
	return NewSessionAwareAdapter(nsp)
}

// MakeSessionAwareAdapter returns a new default SessionAwareAdapter instance.
func MakeSessionAwareAdapter() SessionAwareAdapter {
	return socket.MakeSessionAwareAdapter()
}

// NewSessionAwareAdapter creates a new SessionAwareAdapter for the given Namespace.
func NewSessionAwareAdapter(nsp socket.Namespace) SessionAwareAdapter {
	return socket.NewSessionAwareAdapter(nsp)
}
