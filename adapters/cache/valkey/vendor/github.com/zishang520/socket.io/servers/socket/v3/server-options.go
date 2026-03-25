package socket

import (
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3/config"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	ConnectionStateRecoveryInterface interface {
		SetMaxDisconnectionDuration(int64)
		GetRawMaxDisconnectionDuration() types.Optional[int64]
		MaxDisconnectionDuration() int64

		SetSkipMiddlewares(bool)
		GetRawSkipMiddlewares() types.Optional[bool]
		SkipMiddlewares() bool
	}

	ConnectionStateRecovery struct {
		// The backup duration of the sessions and the packets.
		maxDisconnectionDuration types.Optional[int64]

		// Whether to skip middlewares upon successful connection state recovery.
		skipMiddlewares types.Optional[bool]
	}

	ServerOptionsInterface interface {
		config.OptionsInterface

		SetServeClient(bool)
		GetRawServeClient() types.Optional[bool]
		ServeClient() bool

		SetClientVersion(string)
		GetRawClientVersion() types.Optional[string]
		ClientVersion() string

		SetAdapter(AdapterConstructor)
		GetRawAdapter() types.Optional[AdapterConstructor]
		Adapter() AdapterConstructor

		SetParser(parser.Parser)
		GetRawParser() types.Optional[parser.Parser]
		Parser() parser.Parser

		SetConnectTimeout(time.Duration)
		GetRawConnectTimeout() types.Optional[time.Duration]
		ConnectTimeout() time.Duration

		SetConnectionStateRecovery(ConnectionStateRecoveryInterface)
		GetRawConnectionStateRecovery() types.Optional[ConnectionStateRecoveryInterface]
		ConnectionStateRecovery() ConnectionStateRecoveryInterface

		SetCleanupEmptyChildNamespaces(bool)
		GetRawCleanupEmptyChildNamespaces() types.Optional[bool]
		CleanupEmptyChildNamespaces() bool
	}

	ServerOptions struct {
		config.Options

		// whether to serve the client files
		serveClient types.Optional[bool]

		// Client file version
		clientVersion types.Optional[string]

		// the adapter to use
		adapter types.Optional[AdapterConstructor]

		// the parser to use
		parser types.Optional[parser.Parser]

		// how many ms before a client without namespace is closed
		connectTimeout types.Optional[time.Duration]

		// Whether to enable the recovery of connection state when a client temporarily disconnects.
		//
		// The connection state includes the missed packets, the rooms the socket was in and the `data` attribute.
		connectionStateRecovery types.Optional[ConnectionStateRecoveryInterface]

		// Whether to remove child namespaces that have no sockets connected to them
		cleanupEmptyChildNamespaces types.Optional[bool]
	}
)

func DefaultConnectionStateRecovery() *ConnectionStateRecovery {
	return &ConnectionStateRecovery{}
}

func (c *ConnectionStateRecovery) Assign(data ConnectionStateRecoveryInterface) ConnectionStateRecoveryInterface {
	if data == nil {
		return c
	}

	if data.GetRawMaxDisconnectionDuration() != nil {
		c.SetMaxDisconnectionDuration(data.MaxDisconnectionDuration())
	}
	if data.GetRawSkipMiddlewares() != nil {
		c.SetSkipMiddlewares(data.SkipMiddlewares())
	}

	return c
}

func DefaultServerOptions() *ServerOptions {
	return &ServerOptions{}
}

func (s *ServerOptions) Assign(data ServerOptionsInterface) ServerOptionsInterface {
	if data == nil {
		return s
	}

	s.Options.Assign(data)

	if data.GetRawServeClient() != nil {
		s.SetServeClient(data.ServeClient())
	}
	if data.GetRawClientVersion() != nil {
		s.SetClientVersion(data.ClientVersion())
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
	c.maxDisconnectionDuration = types.NewSome(maxDisconnectionDuration)
}
func (c *ConnectionStateRecovery) GetRawMaxDisconnectionDuration() types.Optional[int64] {
	return c.maxDisconnectionDuration
}
func (c *ConnectionStateRecovery) MaxDisconnectionDuration() int64 {
	if c.maxDisconnectionDuration == nil {
		return 0
	}

	return c.maxDisconnectionDuration.Get()
}

func (c *ConnectionStateRecovery) SetSkipMiddlewares(skipMiddlewares bool) {
	c.skipMiddlewares = types.NewSome(skipMiddlewares)
}
func (c *ConnectionStateRecovery) GetRawSkipMiddlewares() types.Optional[bool] {
	return c.skipMiddlewares
}
func (c *ConnectionStateRecovery) SkipMiddlewares() bool {
	if c.skipMiddlewares == nil {
		return false
	}

	return c.skipMiddlewares.Get()
}

func (s *ServerOptions) SetServeClient(serveClient bool) {
	s.serveClient = types.NewSome(serveClient)
}
func (s *ServerOptions) GetRawServeClient() types.Optional[bool] {
	return s.serveClient
}
func (s *ServerOptions) ServeClient() bool {
	if s.serveClient == nil {
		return false
	}

	return s.serveClient.Get()
}

func (s *ServerOptions) SetClientVersion(clientVersion string) {
	s.clientVersion = types.NewSome(clientVersion)
}
func (s *ServerOptions) GetRawClientVersion() types.Optional[string] {
	return s.clientVersion
}
func (s *ServerOptions) ClientVersion() string {
	if s.clientVersion == nil {
		return ""
	}

	return s.clientVersion.Get()
}

func (s *ServerOptions) SetAdapter(adapter AdapterConstructor) {
	s.adapter = types.NewSome(adapter)
}
func (s *ServerOptions) GetRawAdapter() types.Optional[AdapterConstructor] {
	return s.adapter
}
func (s *ServerOptions) Adapter() AdapterConstructor {
	if s.adapter == nil {
		return nil
	}

	return s.adapter.Get()
}

func (s *ServerOptions) SetParser(parser parser.Parser) {
	s.parser = types.NewSome(parser)
}
func (s *ServerOptions) GetRawParser() types.Optional[parser.Parser] {
	return s.parser
}
func (s *ServerOptions) Parser() parser.Parser {
	if s.parser == nil {
		return nil
	}

	return s.parser.Get()
}

func (s *ServerOptions) SetConnectTimeout(connectTimeout time.Duration) {
	s.connectTimeout = types.NewSome(connectTimeout)
}
func (s *ServerOptions) GetRawConnectTimeout() types.Optional[time.Duration] {
	return s.connectTimeout
}
func (s *ServerOptions) ConnectTimeout() time.Duration {
	if s.connectTimeout == nil {
		return 0
	}

	return s.connectTimeout.Get()
}

func (s *ServerOptions) SetConnectionStateRecovery(connectionStateRecovery ConnectionStateRecoveryInterface) {
	s.connectionStateRecovery = types.NewSome(connectionStateRecovery)
}
func (s *ServerOptions) GetRawConnectionStateRecovery() types.Optional[ConnectionStateRecoveryInterface] {
	return s.connectionStateRecovery
}
func (s *ServerOptions) ConnectionStateRecovery() ConnectionStateRecoveryInterface {
	if s.connectionStateRecovery == nil {
		return nil
	}

	return s.connectionStateRecovery.Get()
}

func (s *ServerOptions) SetCleanupEmptyChildNamespaces(cleanupEmptyChildNamespaces bool) {
	s.cleanupEmptyChildNamespaces = types.NewSome(cleanupEmptyChildNamespaces)
}
func (s *ServerOptions) GetRawCleanupEmptyChildNamespaces() types.Optional[bool] {
	return s.cleanupEmptyChildNamespaces
}
func (s *ServerOptions) CleanupEmptyChildNamespaces() bool {
	if s.cleanupEmptyChildNamespaces == nil {
		return false
	}

	return s.cleanupEmptyChildNamespaces.Get()
}
