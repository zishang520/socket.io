package adapter

import (
	"github.com/zishang520/socket.io/v2/socket"
)

type (
	AdapterBuilder struct {
	}
)

func (*AdapterBuilder) New(nsp socket.Namespace) Adapter {
	return NewAdapter(nsp)
}

func MakeAdapter() Adapter {
	return socket.MakeAdapter()
}

func NewAdapter(nsp socket.Namespace) Adapter {
	return socket.NewAdapter(nsp)
}
