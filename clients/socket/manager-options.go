package socket

import (
	"time"

	"github.com/zishang520/socket.io/clients/engine/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

// ManagerOptionsInterface defines the configuration interface for a Socket.IO Manager.
// It extends EngineOptionsInterface and provides additional options for reconnection, multiplexing, timeouts, and parser selection.
type (
	EngineOptionsInterface = engine.SocketOptionsInterface
	EngineOptions          = engine.SocketOptions

	// ManagerOptionsInterface defines the configuration interface for a Socket.IO Manager.
	ManagerOptionsInterface interface {
		EngineOptionsInterface

		GetRawForceNew() *bool
		ForceNew() bool
		SetForceNew(bool)

		GetRawMultiplex() *bool
		Multiplex() bool
		SetMultiplex(bool)

		GetRawPath() *string
		Path() string
		SetPath(string)

		GetRawReconnection() *bool
		Reconnection() bool
		SetReconnection(bool)

		GetRawReconnectionAttempts() *float64
		ReconnectionAttempts() float64
		SetReconnectionAttempts(float64)

		GetRawReconnectionDelay() *float64
		ReconnectionDelay() float64
		SetReconnectionDelay(float64)

		GetRawReconnectionDelayMax() *float64
		ReconnectionDelayMax() float64
		SetReconnectionDelayMax(float64)

		GetRawRandomizationFactor() *float64
		RandomizationFactor() float64
		SetRandomizationFactor(float64)

		GetRawTimeout() *time.Duration
		Timeout() time.Duration
		SetTimeout(time.Duration)

		GetRawAutoConnect() *bool
		AutoConnect() bool
		SetAutoConnect(bool)

		GetRawParser() parser.Parser
		Parser() parser.Parser
		SetParser(parser.Parser)
	}

	// ManagerOptions holds configuration for a Socket.IO Manager instance.
	ManagerOptions struct {
		EngineOptions

		forceNew             *bool
		multiplex            *bool
		path                 *string
		reconnection         *bool
		reconnectionAttempts *float64
		reconnectionDelay    *float64
		reconnectionDelayMax *float64
		randomizationFactor  *float64
		timeout              *time.Duration
		autoConnect          *bool
		parser               parser.Parser
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

func (s *ManagerOptions) GetRawForceNew() *bool {
	return s.forceNew
}
func (s *ManagerOptions) ForceNew() bool {
	if s.forceNew == nil {
		return false
	}

	return *s.forceNew
}
func (s *ManagerOptions) SetForceNew(forceNew bool) {
	s.forceNew = &forceNew
}

func (s *ManagerOptions) GetRawMultiplex() *bool {
	return s.multiplex
}
func (s *ManagerOptions) Multiplex() bool {
	if s.multiplex == nil {
		return false
	}

	return *s.multiplex
}
func (s *ManagerOptions) SetMultiplex(multiplex bool) {
	s.multiplex = &multiplex
}

func (s *ManagerOptions) GetRawPath() *string {
	return s.path
}
func (s *ManagerOptions) Path() string {
	if s.path == nil {
		return ""
	}

	return *s.path
}
func (s *ManagerOptions) SetPath(path string) {
	s.path = &path
}

func (s *ManagerOptions) GetRawReconnection() *bool {
	return s.reconnection
}
func (s *ManagerOptions) Reconnection() bool {
	if s.reconnection == nil {
		return false
	}

	return *s.reconnection
}
func (s *ManagerOptions) SetReconnection(reconnection bool) {
	s.reconnection = &reconnection
}

func (s *ManagerOptions) GetRawReconnectionAttempts() *float64 {
	return s.reconnectionAttempts
}
func (s *ManagerOptions) ReconnectionAttempts() float64 {
	if s.reconnectionAttempts == nil {
		return 0
	}

	return *s.reconnectionAttempts
}
func (s *ManagerOptions) SetReconnectionAttempts(reconnectionAttempts float64) {
	s.reconnectionAttempts = &reconnectionAttempts
}

func (s *ManagerOptions) GetRawReconnectionDelay() *float64 {
	return s.reconnectionDelay
}
func (s *ManagerOptions) ReconnectionDelay() float64 {
	if s.reconnectionDelay == nil {
		return 0
	}

	return *s.reconnectionDelay
}
func (s *ManagerOptions) SetReconnectionDelay(reconnectionDelay float64) {
	s.reconnectionDelay = &reconnectionDelay
}

func (s *ManagerOptions) GetRawReconnectionDelayMax() *float64 {
	return s.reconnectionDelayMax
}
func (s *ManagerOptions) ReconnectionDelayMax() float64 {
	if s.reconnectionDelayMax == nil {
		return 0
	}

	return *s.reconnectionDelayMax
}
func (s *ManagerOptions) SetReconnectionDelayMax(reconnectionDelayMax float64) {
	s.reconnectionDelayMax = &reconnectionDelayMax
}

func (s *ManagerOptions) GetRawRandomizationFactor() *float64 {
	return s.randomizationFactor
}
func (s *ManagerOptions) RandomizationFactor() float64 {
	if s.randomizationFactor == nil {
		return 0
	}

	return *s.randomizationFactor
}
func (s *ManagerOptions) SetRandomizationFactor(randomizationFactor float64) {
	s.randomizationFactor = &randomizationFactor
}

func (s *ManagerOptions) GetRawTimeout() *time.Duration {
	return s.timeout
}
func (s *ManagerOptions) Timeout() time.Duration {
	if s.timeout == nil {
		return 0
	}

	return *s.timeout
}
func (s *ManagerOptions) SetTimeout(timeout time.Duration) {
	s.timeout = &timeout
}

func (s *ManagerOptions) GetRawAutoConnect() *bool {
	return s.autoConnect
}
func (s *ManagerOptions) AutoConnect() bool {
	if s.autoConnect == nil {
		return false
	}

	return *s.autoConnect
}
func (s *ManagerOptions) SetAutoConnect(autoConnect bool) {
	s.autoConnect = &autoConnect
}

func (s *ManagerOptions) GetRawParser() parser.Parser {
	return s.parser
}
func (s *ManagerOptions) Parser() parser.Parser {
	return s.parser
}
func (s *ManagerOptions) SetParser(parser parser.Parser) {
	s.parser = parser
}
