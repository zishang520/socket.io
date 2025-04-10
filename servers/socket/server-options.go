package socket

import (
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
)

type (
	ConnectionStateRecovery struct {
		// The backup duration of the sessions and the packets.
		maxDisconnectionDuration *int64

		// Whether to skip middlewares upon successful connection state recovery.
		skipMiddlewares *bool
	}

	ServerOptionsInterface interface {
		config.ServerOptionsInterface
		config.AttachOptionsInterface

		SetServeClient(bool)
		GetRawServeClient() *bool
		ServeClient() bool

		SetAdapter(AdapterConstructor)
		GetRawAdapter() AdapterConstructor
		Adapter() AdapterConstructor

		SetParser(parser.Parser)
		GetRawParser() parser.Parser
		Parser() parser.Parser

		SetConnectTimeout(time.Duration)
		GetRawConnectTimeout() *time.Duration
		ConnectTimeout() time.Duration

		SetConnectionStateRecovery(*ConnectionStateRecovery)
		GetRawConnectionStateRecovery() *ConnectionStateRecovery
		ConnectionStateRecovery() *ConnectionStateRecovery

		SetCleanupEmptyChildNamespaces(bool)
		GetRawCleanupEmptyChildNamespaces() *bool
		CleanupEmptyChildNamespaces() bool
	}

	ServerOptions struct {
		config.ServerOptions
		config.AttachOptions

		// whether to serve the client files
		serveClient *bool

		// the adapter to use
		adapter AdapterConstructor

		// the parser to use
		parser parser.Parser

		// how many ms before a client without namespace is closed
		connectTimeout *time.Duration

		// Whether to enable the recovery of connection state when a client temporarily disconnects.
		//
		// The connection state includes the missed packets, the rooms the socket was in and the `data` attribute.
		connectionStateRecovery *ConnectionStateRecovery

		// Whether to remove child namespaces that have no sockets connected to them
		cleanupEmptyChildNamespaces *bool
	}
)

func DefaultServerOptions() *ServerOptions {
	return &ServerOptions{}
}

func (s *ServerOptions) Assign(data ServerOptionsInterface) ServerOptionsInterface {
	if data == nil {
		return s
	}

	s.ServerOptions.Assign(data)

	s.AttachOptions.Assign(data)

	if data.GetRawServeClient() != nil {
		s.SetServeClient(data.ServeClient())
	}
	if data.GetRawAdapter() != nil {
		s.SetAdapter(data.Adapter())
	}
	if data.GetRawParser() != nil {
		s.SetParser(data.Parser())
	}
	if data.GetRawConnectTimeout() != nil {
		s.SetConnectTimeout(data.ConnectTimeout())
	}
	if data.GetRawConnectionStateRecovery() != nil {
		s.SetConnectionStateRecovery(data.ConnectionStateRecovery())
	}
	if data.GetRawCleanupEmptyChildNamespaces() != nil {
		s.SetCleanupEmptyChildNamespaces(data.CleanupEmptyChildNamespaces())
	}

	return s
}

func (c *ConnectionStateRecovery) SetMaxDisconnectionDuration(maxDisconnectionDuration int64) {
	c.maxDisconnectionDuration = &maxDisconnectionDuration
}
func (c *ConnectionStateRecovery) GetRawMaxDisconnectionDuration() *int64 {
	return c.maxDisconnectionDuration
}
func (c *ConnectionStateRecovery) MaxDisconnectionDuration() int64 {
	if c.maxDisconnectionDuration == nil {
		return 0
	}

	return *c.maxDisconnectionDuration
}

func (c *ConnectionStateRecovery) SetSkipMiddlewares(skipMiddlewares bool) {
	c.skipMiddlewares = &skipMiddlewares
}
func (c *ConnectionStateRecovery) GetRawSkipMiddlewares() *bool {
	return c.skipMiddlewares
}
func (c *ConnectionStateRecovery) SkipMiddlewares() bool {
	if c.skipMiddlewares == nil {
		return false
	}

	return *c.skipMiddlewares
}

func (s *ServerOptions) SetServeClient(serveClient bool) {
	s.serveClient = &serveClient
}
func (s *ServerOptions) GetRawServeClient() *bool {
	return s.serveClient
}
func (s *ServerOptions) ServeClient() bool {
	if s.serveClient == nil {
		return false
	}

	return *s.serveClient
}

func (s *ServerOptions) SetAdapter(adapter AdapterConstructor) {
	s.adapter = adapter
}
func (s *ServerOptions) GetRawAdapter() AdapterConstructor {
	return s.adapter
}
func (s *ServerOptions) Adapter() AdapterConstructor {
	return s.adapter
}

func (s *ServerOptions) SetParser(parser parser.Parser) {
	s.parser = parser
}
func (s *ServerOptions) GetRawParser() parser.Parser {
	return s.parser
}
func (s *ServerOptions) Parser() parser.Parser {
	return s.parser
}

func (s *ServerOptions) SetConnectTimeout(connectTimeout time.Duration) {
	s.connectTimeout = &connectTimeout
}
func (s *ServerOptions) GetRawConnectTimeout() *time.Duration {
	return s.connectTimeout
}
func (s *ServerOptions) ConnectTimeout() time.Duration {
	if s.connectTimeout == nil {
		return 0
	}

	return *s.connectTimeout
}

func (s *ServerOptions) SetConnectionStateRecovery(connectionStateRecovery *ConnectionStateRecovery) {
	s.connectionStateRecovery = connectionStateRecovery
}
func (s *ServerOptions) GetRawConnectionStateRecovery() *ConnectionStateRecovery {
	return s.connectionStateRecovery
}
func (s *ServerOptions) ConnectionStateRecovery() *ConnectionStateRecovery {
	return s.connectionStateRecovery
}

func (s *ServerOptions) SetCleanupEmptyChildNamespaces(cleanupEmptyChildNamespaces bool) {
	s.cleanupEmptyChildNamespaces = &cleanupEmptyChildNamespaces
}
func (s *ServerOptions) GetRawCleanupEmptyChildNamespaces() *bool {
	return s.cleanupEmptyChildNamespaces
}
func (s *ServerOptions) CleanupEmptyChildNamespaces() bool {
	if s.cleanupEmptyChildNamespaces == nil {
		return false
	}

	return *s.cleanupEmptyChildNamespaces
}
