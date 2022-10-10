package socket

import (
	"time"

	"github.com/zishang520/engine.io/config"
	"github.com/zishang520/socket.io/parser"
)

type ServerOptionsInterface interface {
	config.ServerOptionsInterface
	config.AttachOptionsInterface

	SetServeClient(serveClient bool)
	GetRawServeClient() *bool
	ServeClient() bool

	SetAdapter(adapter Adapter)
	GetRawAdapter() Adapter
	Adapter() Adapter

	SetParser(parser parser.Parser)
	GetRawParser() parser.Parser
	Parser() parser.Parser

	SetConnectTimeout(connectTimeout time.Duration)
	GetRawConnectTimeout() *time.Duration
	ConnectTimeout() time.Duration
}

type ServerOptions struct {
	config.ServerOptions
	config.AttachOptions

	// whether to serve the client files
	serveClient *bool

	// the adapter to use
	adapter Adapter

	// the parser to use
	parser parser.Parser

	// how many ms before a client without namespace is closed
	connectTimeout *time.Duration
}

func DefaultServerOptions() *ServerOptions {
	a := &ServerOptions{}
	return a
}

func (s *ServerOptions) Assign(data ServerOptionsInterface) (ServerOptionsInterface, error) {
	if data == nil {
		return s, nil
	}

	if s.GetRawPingTimeout() == nil {
		s.SetPingTimeout(data.PingTimeout())
	}
	if s.GetRawPingInterval() == nil {
		s.SetPingInterval(data.PingInterval())
	}
	if s.GetRawUpgradeTimeout() == nil {
		s.SetUpgradeTimeout(data.UpgradeTimeout())
	}
	if s.GetRawMaxHttpBufferSize() == nil {
		s.SetMaxHttpBufferSize(data.MaxHttpBufferSize())
	}
	if s.GetRawAllowRequest() == nil {
		s.SetAllowRequest(data.AllowRequest())
	}
	if s.GetRawTransports() == nil {
		s.SetTransports(data.Transports())
	}
	if s.GetRawAllowUpgrades() == nil {
		s.SetAllowUpgrades(data.AllowUpgrades())
	}
	if s.GetRawPerMessageDeflate() == nil {
		s.SetPerMessageDeflate(data.PerMessageDeflate())
	}
	if s.GetRawHttpCompression() == nil {
		s.SetHttpCompression(data.HttpCompression())
	}
	if s.GetRawInitialPacket() == nil {
		s.SetInitialPacket(data.InitialPacket())
	}
	if s.GetRawCookie() == nil {
		s.SetCookie(data.Cookie())
	}
	if s.GetRawCors() == nil {
		s.SetCors(data.Cors())
	}
	if s.GetRawAllowEIO3() == nil {
		s.SetAllowEIO3(data.AllowEIO3())
	}

	if s.GetRawPath() == nil {
		s.SetPath(data.Path())
	}

	if s.GetRawDestroyUpgradeTimeout() == nil {
		s.SetDestroyUpgradeTimeout(data.DestroyUpgradeTimeout())
	}

	if s.GetRawDestroyUpgrade() == nil {
		s.SetDestroyUpgrade(data.DestroyUpgrade())
	}

	if s.GetRawServeClient() == nil {
		s.SetServeClient(data.ServeClient())
	}

	if s.GetRawAdapter() == nil {
		s.SetAdapter(data.Adapter())
	}

	if s.GetRawParser() == nil {
		s.SetParser(data.Parser())
	}

	if s.GetRawConnectTimeout() == nil {
		s.SetConnectTimeout(data.ConnectTimeout())
	}

	return s, nil
}

func (s *ServerOptions) Path() string {
	if s.GetRawPath() == nil {
		return "/socket.io"
	}

	return s.AttachOptions.Path()
}

func (s *ServerOptions) SetServeClient(serveClient bool) {
	s.serveClient = &serveClient
}
func (s *ServerOptions) GetRawServeClient() *bool {
	return s.serveClient
}
func (s *ServerOptions) ServeClient() bool {
	if s.serveClient == nil {
		return true
	}

	return *s.serveClient
}

func (s *ServerOptions) SetAdapter(adapter Adapter) {
	s.adapter = adapter
}
func (s *ServerOptions) GetRawAdapter() Adapter {
	return s.adapter
}
func (s *ServerOptions) Adapter() Adapter {
	if s.adapter == nil {
		return &adapter{}
	}
	return s.adapter
}

func (s *ServerOptions) SetParser(parser parser.Parser) {
	s.parser = parser
}
func (s *ServerOptions) GetRawParser() parser.Parser {
	return s.parser
}
func (s *ServerOptions) Parser() parser.Parser {
	if s.parser == nil {
		return parser.NewParser()
	}
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
		return time.Duration(45000 * time.Millisecond)
	}

	return *s.connectTimeout
}
