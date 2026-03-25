// Package engine defines core types and interfaces for Engine.IO server, including BaseServer, Server, and Socket abstractions.
package engine

import (
	"io"
	"net/http"

	"github.com/quic-go/webtransport-go"
	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	SendCallback func(transports.Transport)

	// Middleware functions are functions that have access to the *types.HttpContext
	// and the next middleware function in the application's context cycle.
	Middleware func(*types.HttpContext, func(error))

	BaseServer interface {
		// Extends

		types.EventEmitter

		// Prototype

		Prototype(BaseServer)
		Proto() BaseServer

		// Getters

		Opts() config.ServerOptionsInterface
		// Protected
		Clients() *types.Map[string, Socket]
		ClientsCount() uint64
		// Protected
		Middlewares() []Middleware
		Transports() *types.Set[string]
		TransportsByName() map[string]transports.TransportCtor

		// Methods

		// Construct() should be called after calling Prototype()
		Construct(any)
		// Protected
		//
		Init()
		// Protected
		//
		// Compute the pathname of the requests that are handled by the server
		ComputePath(config.AttachOptionsInterface) string
		// Returns a list of available transports for upgrade given a certain transport.
		Upgrades(string) []string
		// Protected
		//
		// Verifies a request.
		Verify(*types.HttpContext, bool) (*types.CodeMessage, map[string]any)
		// Adds a new middleware.
		Use(Middleware)
		// Protected
		// Apply the middlewares to the request.
		ApplyMiddlewares(*types.HttpContext, func(error))
		// Closes all clients.
		Close() BaseServer
		// Protected
		Cleanup()
		// generate a socket id.
		// Overwrite this method to generate your custom socket id
		GenerateId(*types.HttpContext) (string, error)
		// Protected
		//
		// Handshakes a new client.
		Handshake(string, *types.HttpContext) (*types.CodeMessage, transports.Transport)
		// Protected
		CreateTransport(string, *types.HttpContext) (transports.Transport, error)
	}

	Server interface {
		// Extends

		BaseServer
		// Captures upgrade requests for a http.Handler, Need to handle server shutdown disconnecting client connections.
		http.Handler

		// Setters

		SetHttpServer(*types.HttpServer)

		// Getters

		HttpServer() *types.HttpServer

		// Methods

		CreateTransport(string, *types.HttpContext) (transports.Transport, error)
		// Handles an Engine.IO HTTP request.
		HandleRequest(*types.HttpContext)
		// Handles an Engine.IO HTTP Upgrade.
		HandleUpgrade(*types.HttpContext)
		OnWebTransportSession(*types.HttpContext, *webtransport.Server)
		// Captures upgrade requests for a *types.HttpServer.
		Attach(*types.HttpServer, any)
	}

	Socket interface {
		// Extends

		types.EventEmitter

		// Setters

		SetReadyState(string)

		// Getters

		Protocol() int
		Request() *types.HttpContext
		RemoteAddress() string
		Transport() transports.Transport
		Id() string
		ReadyState() string
		// Private
		Upgraded() bool
		// Private
		Upgrading() bool

		// Methods

		Construct(string, BaseServer, transports.Transport, *types.HttpContext, int)
		// Private
		//
		// Upgrades socket to the given transport
		MaybeUpgrade(transports.Transport)
		// Sends a message packet.
		Send(io.Reader, *packet.Options, SendCallback) Socket
		Write(io.Reader, *packet.Options, SendCallback) Socket
		// Closes the socket and underlying transport.
		Close(bool)
	}
)
