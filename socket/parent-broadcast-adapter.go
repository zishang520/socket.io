package socket

import (
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

type (
	// A dummy adapter that only supports broadcasting to child (concrete) namespaces.
	parentBroadcastAdapter struct {
		Adapter

		children *types.Set[*Namespace]
	}
)

func MakeParentBroadcastAdapter(children *types.Set[*Namespace]) Adapter {
	s := &parentBroadcastAdapter{
		Adapter: MakeAdapter(),

		children: children,
	}

	s.Prototype(s)

	return s
}

func NewParentBroadcastAdapter(nsp NamespaceInterface, children *types.Set[*Namespace]) Adapter {
	s := MakeParentBroadcastAdapter(children)

	s.Construct(nsp)

	return s
}

func (s *parentBroadcastAdapter) Construct(nsp NamespaceInterface) {
	s.Adapter.Construct(nsp)
}

func (s *parentBroadcastAdapter) Broadcast(packet *parser.Packet, opts *BroadcastOptions) {
	for _, nsp := range s.children.Keys() {
		nsp.Adapter().Broadcast(packet, opts)
	}
}
