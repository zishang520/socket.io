package socket

import (
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

type (
	ParentBroadcastAdapterBuilder struct {
		AdapterConstructor
	}

	// A dummy adapter that only supports broadcasting to child (concrete) namespaces.
	parentBroadcastAdapter struct {
		Adapter
	}
)

func (b *ParentBroadcastAdapterBuilder) New(nsp Namespace) Adapter {
	return NewParentBroadcastAdapter(nsp)
}

func MakeParentBroadcastAdapter() ParentBroadcastAdapter {
	s := &parentBroadcastAdapter{
		Adapter: MakeAdapter(),
	}

	s.Prototype(s)

	return s
}

func NewParentBroadcastAdapter(nsp Namespace) ParentBroadcastAdapter {
	s := MakeParentBroadcastAdapter()

	s.Construct(nsp)

	return s
}

func (s *parentBroadcastAdapter) Broadcast(packet *parser.Packet, opts *BroadcastOptions) {
	for _, nsp := range s.Nsp().(ParentNamespace).Children().Keys() {
		nsp.Adapter().Broadcast(packet, opts)
	}
}
