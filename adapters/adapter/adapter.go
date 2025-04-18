package adapter

import (
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// AdapterBuilder is a builder for creating Adapter instances.
type (
	AdapterBuilder struct {
	}
)

// New creates a new Adapter for the given Namespace.
func (*AdapterBuilder) New(nsp socket.Namespace) Adapter {
	return NewAdapter(nsp)
}

// MakeAdapter returns a new default Adapter instance.
func MakeAdapter() Adapter {
	return socket.MakeAdapter()
}

// NewAdapter creates a new Adapter for the given Namespace.
func NewAdapter(nsp socket.Namespace) Adapter {
	return socket.NewAdapter(nsp)
}
