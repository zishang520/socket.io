package socket

import (
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

type (
	ParentBroadcastAdapterBuilder struct {
		AdapterConstructor

		Children *types.Set[Namespace]
	}

	// A dummy adapter that only supports broadcasting to child (concrete) namespaces.
	parentBroadcastAdapter struct {
		Adapter

		children *types.Set[Namespace]
	}
)

func (b *ParentBroadcastAdapterBuilder) New(nsp Namespace) Adapter {
	return NewParentBroadcastAdapter(nsp, b.Children)
}

func MakeParentBroadcastAdapter(children *types.Set[Namespace]) ParentBroadcastAdapter {
	s := &parentBroadcastAdapter{
		Adapter: MakeAdapter(),

		children: children,
	}

	s.Prototype(s)

	return s
}

func NewParentBroadcastAdapter(nsp Namespace, children *types.Set[Namespace]) ParentBroadcastAdapter {
	s := MakeParentBroadcastAdapter(children)

	s.Construct(nsp)

	return s
}

func (s *parentBroadcastAdapter) Construct(nsp Namespace) {
	s.Adapter.Construct(nsp)
}

func (s *parentBroadcastAdapter) Broadcast(packet *parser.Packet, opts *BroadcastOptions) {
	for _, nsp := range s.children.Keys() {
		nsp.Adapter().Broadcast(packet, opts)
	}
}
