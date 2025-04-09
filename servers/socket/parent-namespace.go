package socket

import (
	"errors"
	"strconv"
	"sync/atomic"

	"github.com/zishang520/socket.io/servers/engine/v3/log"
	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

var (
	parent_namespace_log = log.NewLog("socket.io:parent-namespace")

	count atomic.Uint64
)

// A parent namespace is a special [Namespace] that holds a list of child namespaces which were created either
// with a regular expression or with a function.
//
//	parentNamespace := io.Of(regexp.MustCompile(`/dynamic-\d+`))
//
//	parentNamespace.On("connection", func(clients ...any) {
//		client := clients[0].(*socket.Socket)
//		childNamespace := client.Nsp()
//	}
//
//	// will reach all the clients that are in one of the child namespaces, like "/dynamic-101"
//
//	parentNamespace.Emit("hello", "world")
type parentNamespace struct {
	Namespace

	adapter  Adapter
	children *types.Set[Namespace]
}

func MakeParentNamespace() ParentNamespace {
	n := &parentNamespace{
		Namespace: MakeNamespace(),

		children: types.NewSet[Namespace](),
	}

	n.Prototype(n)

	return n
}

func NewParentNamespace(server *Server) ParentNamespace {
	n := MakeParentNamespace()

	n.Construct(server, "/_"+strconv.FormatUint(count.Add(1)-1, 10))

	return n
}

func (p *parentNamespace) Children() *types.Set[Namespace] {
	return p.children
}

func (p *parentNamespace) Adapter() Adapter {
	return p.adapter
}

func (p *parentNamespace) InitAdapter() {
	p.adapter = NewParentBroadcastAdapter(p)
}

func (p *parentNamespace) Emit(ev string, args ...any) error {
	for _, nsp := range p.children.Keys() {
		nsp.Emit(ev, args...)
	}
	return nil
}

func (p *parentNamespace) CreateChild(name string) Namespace {
	parent_namespace_log.Debug("creating child namespace %s", name)
	namespace := NewNamespace(p.Server(), name)

	namespace.Fns().Replace(p.Fns().All())

	namespace.On("connect", p.Listeners("connect")...)
	namespace.On("connection", p.Listeners("connection")...)
	p.children.Add(namespace)

	if p.Server().Opts().CleanupEmptyChildNamespaces() {
		namespace.Cleanup(func() {
			if namespace.Sockets().Len() == 0 {
				parent_namespace_log.Debug("closing child namespace %s", name)
				namespace.Adapter().Close()
				p.Server()._nsps.Delete(namespace.Name())
				p.children.Delete(namespace)
			}
		})
	}

	p.Server()._nsps.Store(name, namespace)

	p.Server().Sockets().EmitReserved("new_namespace", namespace)

	return namespace
}

func (p *parentNamespace) FetchSockets() func(func([]*RemoteSocket, error)) {
	return func(callback func([]*RemoteSocket, error)) {
		// note: we could make the FetchSockets() method work for dynamic namespaces created with a regex (by sending the
		// regex to the other Socket.IO servers, and returning the sockets of each matching namespace for example), but
		// the behavior for namespaces created with a function is less clear
		// note²: we cannot loop over each children namespace, because with multiple Socket.IO servers, a given namespace
		// may exist on one node but not exist on another (since it is created upon client connection)
		callback(nil, errors.New("FetchSockets() is not supported on parent namespaces"))
	}
}
