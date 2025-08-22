package socket

import (
	"time"

	"github.com/zishang520/socket.io/clients/engine/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// ManagerOptionsInterface defines the configuration interface for a Socket.IO Manager.
// It extends EngineOptionsInterface and provides additional options for reconnection, multiplexing, timeouts, and parser selection.
type (
	EngineOptionsInterface = engine.SocketOptionsInterface
	EngineOptions          = engine.SocketOptions

	// ManagerOptionsInterface defines the configuration interface for a Socket.IO Manager.
	ManagerOptionsInterface interface {
		EngineOptionsInterface

		SetForceNew(bool)
		GetRawForceNew() types.Optional[bool]
		ForceNew() bool

		SetMultiplex(bool)
		GetRawMultiplex() types.Optional[bool]
		Multiplex() bool

		SetPath(string)
		GetRawPath() types.Optional[string]
		Path() string

		SetReconnection(bool)
		GetRawReconnection() types.Optional[bool]
		Reconnection() bool

		SetReconnectionAttempts(float64)
		GetRawReconnectionAttempts() types.Optional[float64]
		ReconnectionAttempts() float64

		SetReconnectionDelay(float64)
		GetRawReconnectionDelay() types.Optional[float64]
		ReconnectionDelay() float64

		SetReconnectionDelayMax(float64)
		GetRawReconnectionDelayMax() types.Optional[float64]
		ReconnectionDelayMax() float64

		SetRandomizationFactor(float64)
		GetRawRandomizationFactor() types.Optional[float64]
		RandomizationFactor() float64

		SetTimeout(time.Duration)
		GetRawTimeout() types.Optional[time.Duration]
		Timeout() time.Duration

		SetAutoConnect(bool)
		GetRawAutoConnect() types.Optional[bool]
		AutoConnect() bool

		SetParser(parser.Parser)
		GetRawParser() types.Optional[parser.Parser]
		Parser() parser.Parser
	}

	// ManagerOptions holds configuration for a Socket.IO Manager instance.
	ManagerOptions struct {
		EngineOptions

		forceNew             types.Optional[bool]
		multiplex            types.Optional[bool]
		path                 types.Optional[string]
		reconnection         types.Optional[bool]
		reconnectionAttempts types.Optional[float64]
		reconnectionDelay    types.Optional[float64]
		reconnectionDelayMax types.Optional[float64]
		randomizationFactor  types.Optional[float64]
		timeout              types.Optional[time.Duration]
		autoConnect          types.Optional[bool]
		parser               types.Optional[parser.Parser]
	}
)

// DefaultManagerOptions returns a new ManagerOptions instance with default values.
func DefaultManagerOptions() *ManagerOptions {
	return &ManagerOptions{}
}

// Assign copies all options from another ManagerOptionsInterface instance.
// If data is nil, it returns the current ManagerOptions instance.
func (s *ManagerOptions) Assign(data ManagerOptionsInterface) ManagerOptionsInterface {
	if data == nil {
		return s
	}

	s.EngineOptions.Assign(data)

	if data.GetRawForceNew() != nil {
		s.SetForceNew(data.ForceNew())
	}
	if data.GetRawMultiplex() != nil {
		s.SetMultiplex(data.Multiplex())
	}
	if data.GetRawPath() != nil {
		s.SetPath(data.Path())
	}
	if data.GetRawReconnection() != nil {
		s.SetReconnection(data.Reconnection())
	}
	if data.GetRawReconnectionAttempts() != nil {
		s.SetReconnectionAttempts(data.ReconnectionAttempts())
	}
	if data.GetRawReconnectionDelay() != nil {
		s.SetReconnectionDelay(data.ReconnectionDelay())
	}
	if data.GetRawReconnectionDelayMax() != nil {
		s.SetReconnectionDelayMax(data.ReconnectionDelayMax())
	}
	if data.GetRawRandomizationFactor() != nil {
		s.SetRandomizationFactor(data.RandomizationFactor())
	}
	if data.GetRawTimeout() != nil {
		s.SetTimeout(data.Timeout())
	}
	if data.GetRawAutoConnect() != nil {
		s.SetAutoConnect(data.AutoConnect())
	}
	if data.GetRawParser() != nil {
		s.SetParser(data.Parser())
	}

	return s
}

func (s *ManagerOptions) SetForceNew(forceNew bool) {
	s.forceNew = types.NewSome(forceNew)
}
func (s *ManagerOptions) GetRawForceNew() types.Optional[bool] {
	return s.forceNew
}
func (s *ManagerOptions) ForceNew() bool {
	if s.forceNew == nil {
		return false
	}

	return s.forceNew.Get()
}

func (s *ManagerOptions) SetMultiplex(multiplex bool) {
	s.multiplex = types.NewSome(multiplex)
}
func (s *ManagerOptions) GetRawMultiplex() types.Optional[bool] {
	return s.multiplex
}
func (s *ManagerOptions) Multiplex() bool {
	if s.multiplex == nil {
		return false
	}

	return s.multiplex.Get()
}

func (s *ManagerOptions) SetPath(path string) {
	s.path = types.NewSome(path)
}
func (s *ManagerOptions) GetRawPath() types.Optional[string] {
	return s.path
}
func (s *ManagerOptions) Path() string {
	if s.path == nil {
		return ""
	}

	return s.path.Get()
}

func (s *ManagerOptions) SetReconnection(reconnection bool) {
	s.reconnection = types.NewSome(reconnection)
}
func (s *ManagerOptions) GetRawReconnection() types.Optional[bool] {
	return s.reconnection
}
func (s *ManagerOptions) Reconnection() bool {
	if s.reconnection == nil {
		return false
	}

	return s.reconnection.Get()
}

func (s *ManagerOptions) SetReconnectionAttempts(reconnectionAttempts float64) {
	s.reconnectionAttempts = types.NewSome(reconnectionAttempts)
}
func (s *ManagerOptions) GetRawReconnectionAttempts() types.Optional[float64] {
	return s.reconnectionAttempts
}
func (s *ManagerOptions) ReconnectionAttempts() float64 {
	if s.reconnectionAttempts == nil {
		return 0
	}

	return s.reconnectionAttempts.Get()
}

func (s *ManagerOptions) SetReconnectionDelay(reconnectionDelay float64) {
	s.reconnectionDelay = types.NewSome(reconnectionDelay)
}
func (s *ManagerOptions) GetRawReconnectionDelay() types.Optional[float64] {
	return s.reconnectionDelay
}
func (s *ManagerOptions) ReconnectionDelay() float64 {
	if s.reconnectionDelay == nil {
		return 0
	}

	return s.reconnectionDelay.Get()
}

func (s *ManagerOptions) SetReconnectionDelayMax(reconnectionDelayMax float64) {
	s.reconnectionDelayMax = types.NewSome(reconnectionDelayMax)
}
func (s *ManagerOptions) GetRawReconnectionDelayMax() types.Optional[float64] {
	return s.reconnectionDelayMax
}
func (s *ManagerOptions) ReconnectionDelayMax() float64 {
	if s.reconnectionDelayMax == nil {
		return 0
	}

	return s.reconnectionDelayMax.Get()
}

func (s *ManagerOptions) SetRandomizationFactor(randomizationFactor float64) {
	s.randomizationFactor = types.NewSome(randomizationFactor)
}
func (s *ManagerOptions) GetRawRandomizationFactor() types.Optional[float64] {
	return s.randomizationFactor
}
func (s *ManagerOptions) RandomizationFactor() float64 {
	if s.randomizationFactor == nil {
		return 0
	}

	return s.randomizationFactor.Get()
}

func (s *ManagerOptions) SetTimeout(timeout time.Duration) {
	s.timeout = types.NewSome(timeout)
}
func (s *ManagerOptions) GetRawTimeout() types.Optional[time.Duration] {
	return s.timeout
}
func (s *ManagerOptions) Timeout() time.Duration {
	if s.timeout == nil {
		return 0
	}

	return s.timeout.Get()
}

func (s *ManagerOptions) SetAutoConnect(autoConnect bool) {
	s.autoConnect = types.NewSome(autoConnect)
}
func (s *ManagerOptions) GetRawAutoConnect() types.Optional[bool] {
	return s.autoConnect
}
func (s *ManagerOptions) AutoConnect() bool {
	if s.autoConnect == nil {
		return false
	}

	return s.autoConnect.Get()
}

func (s *ManagerOptions) SetParser(parser parser.Parser) {
	s.parser = types.NewSome(parser)
}
func (s *ManagerOptions) GetRawParser() types.Optional[parser.Parser] {
	return s.parser
}
func (s *ManagerOptions) Parser() parser.Parser {
	if s.parser == nil {
		return nil
	}

	return s.parser.Get()
}
