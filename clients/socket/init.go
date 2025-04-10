// Package socket implements a Socket.IO client in Go.
// It provides real-time bidirectional event-based communication between web clients and servers.
//
// Example usage:
//
//	socket, err := socket.Connect("http://localhost:8080", nil)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	socket.On("connect", func() {
//	    socket.Emit("hello", "world")
//	})
package socket

import (
	"github.com/zishang520/socket.io/clients/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/clients/socket/v3/utils"
)

var (
	manager_log = log.NewLog("socket.io-client:manager")
	socket_log  = log.NewLog("socket.io-client:socket")
	client_log  = log.NewLog("socket.io-client")

	RESERVED_EVENTS = types.NewSet("connect", "connect_error", "disconnect", "disconnecting", "newListener", "removeListener")

	Polling      = transports.Polling
	WebSocket    = transports.WebSocket
	WebTransport = transports.WebTransport

	cache types.Map[string, *Manager]
)

func init() {
	cache = types.Map[string, *Manager]{}
}

func lookup(uri string, opts OptionsInterface) (*Socket, error) {
	if opts == nil {
		opts = DefaultOptions()
	}

	path := "/socket.io"
	if opts.GetRawPath() != nil {
		path = opts.Path()
	}
	parsed, err := utils.Url(uri, path)
	if err != nil {
		return nil, err
	}

	source := parsed.String()
	id := parsed.Id
	sameNamespace := false
	if manager, ok := cache.Load(id); ok {
		_, sameNamespace = manager.nsps.Load(parsed.Path)
	}
	newConnection := opts.ForceNew() || !opts.Multiplex() || sameNamespace

	var io *Manager
	if newConnection {
		client_log.Debug("ignoring socket cache for %s", source)
		io = NewManager(source, opts)
	} else {
		manager, ok := cache.LoadOrStore(id, NewManager(source, opts))
		if !ok {
			client_log.Debug("new io instance for %s", source)
		}
		io = manager
	}
	if opts.GetRawQuery() == nil && parsed.RawQuery != "" {
		opts.SetQuery(parsed.Query())
	}

	return io.Socket(parsed.Path, opts), nil
}

func Io(uri string, opts OptionsInterface) (*Socket, error) {
	return lookup(uri, opts)
}

func Connect(uri string, opts OptionsInterface) (*Socket, error) {
	return lookup(uri, opts)
}
