// Package engine provides entry points for creating and attaching Engine.IO servers, including HTTP/WebSocket/WebTransport support.
package engine

import (
	"net/http"

	"github.com/zishang520/socket.io/parsers/engine/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

const Protocol = parser.Protocol

type (
	TransportCtor = transports.TransportCtor

	WebSocketBuilder    = transports.WebSocketBuilder
	WebTransportBuilder = transports.WebTransportBuilder
	PollingBuilder      = transports.PollingBuilder
)

var (
	Polling      TransportCtor = &PollingBuilder{}
	WebSocket    TransportCtor = &WebSocketBuilder{}
	WebTransport TransportCtor = &WebTransportBuilder{}
)

func New(server any, args ...any) Server {
	switch s := server.(type) {
	case *types.HttpServer:
		return Attach(s, append(args, nil)[0])
	case any:
		return NewServer(s)
	}
	return NewServer(nil)
}

// Creates an http.Server exclusively used for WS upgrades.
func Listen(addr string, options any, fn types.Callable) Server {
	server := types.NewWebServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "Not Implemented", http.StatusNotImplemented)
	}))

	// create engine server
	engine := Attach(server, options)
	engine.SetHttpServer(server)

	server.Listen(addr, fn)

	return engine
}

// Captures upgrade requests for a types.HttpServer.
func Attach(server *types.HttpServer, options any) Server {
	engine := NewServer(options)
	engine.Attach(server, options)
	return engine
}
