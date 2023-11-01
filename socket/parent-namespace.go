package socket

import (
	"errors"
	"strconv"
	"sync/atomic"

	"github.com/zishang520/engine.io/v2/log"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io-go-parser/v2/parser"
)

var (
	parent_namespace_log = log.NewLog("socket.io:parent-namespace")

	count uint64 = 0
)

type ParentNamespace struct {
	*Namespace

	children *types.Set[*Namespace]
}

// A parent namespace is a special {@link Namespace} that holds a list of child namespaces which were created either
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
func NewParentNamespace(server *Server) *ParentNamespace {
	p := &ParentNamespace{}
	p.Namespace = NewNamespace(server, "/_"+strconv.FormatUint(atomic.AddUint64(&count, 1)-1, 10))
	p.children = types.NewSet[*Namespace]()
	p._initAdapter()

	return p
}

func (p *ParentNamespace) _initAdapter() {
	p.adapter.SetBroadcast(func(packet *parser.Packet, opts *BroadcastOptions) {
		for _, nsp := range p.children.Keys() {
			nsp.adapter.Broadcast(packet, opts)
		}
	})
}

func (p *ParentNamespace) Emit(ev string, args ...any) error {
	for _, nsp := range p.children.Keys() {
		nsp.Emit(ev, args...)
	}
	return nil
}

func (p *ParentNamespace) CreateChild(name string) *Namespace {
	parent_namespace_log.Debug("creating child namespace %s", name)
	namespace := NewNamespace(p.server, name)

	namespace._fns_mu.RLock()
	namespace._fns = make([]func(*Socket, func(*ExtendedError)), len(p._fns))
	copy(namespace._fns, p._fns)
	namespace._fns_mu.RUnlock()

	namespace.AddListener("connect", p.Listeners("connect")...)
	namespace.AddListener("connection", p.Listeners("connection")...)
	p.children.Add(namespace)

	if p.server.Opts().CleanupEmptyChildNamespaces() {
		namespace._remove = func(socket *Socket) {
			namespace.namespace_remove(socket)
			if namespace.sockets.Len() == 0 {
				parent_namespace_log.Debug("closing child namespace %s", name)
				namespace.adapter.Close()
				p.server._nsps.Delete(namespace.name)
				p.children.Delete(namespace)
			}
		}
	}

	p.server._nsps.Store(name, namespace)

	p.server.Sockets().EmitReserved("new_namespace", namespace)
	return namespace
}

func (p *ParentNamespace) FetchSockets() func(func([]*RemoteSocket, error)) {
	return func(callback func([]*RemoteSocket, error)) {
		// note: we could make the FetchSockets() method work for dynamic namespaces created with a regex (by sending the
		// regex to the other Socket.IO servers, and returning the sockets of each matching namespace for example), but
		// the behavior for namespaces created with a function is less clear
		// note²: we cannot loop over each children namespace, because with multiple Socket.IO servers, a given namespace
		// may exist on one node but not exist on another (since it is created upon client connection)
		callback(nil, errors.New("FetchSockets() is not supported on parent namespaces"))
	}
}
