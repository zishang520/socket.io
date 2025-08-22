package socket

import (
	"time"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// SocketOptionsInterface defines the interface for accessing and modifying Socket options.
// It provides methods for authentication, retry logic, and acknowledgement timeouts.
type (
	SocketOptionsInterface interface {
		SetAuth(map[string]any)
		GetRawAuth() types.Optional[map[string]any]
		Auth() map[string]any

		SetRetries(float64)
		GetRawRetries() types.Optional[float64]
		Retries() float64

		SetAckTimeout(time.Duration)
		GetRawAckTimeout() types.Optional[time.Duration]
		AckTimeout() time.Duration
	}

	// SocketOptions defines configuration options for individual Socket.IO sockets.
	// These options control the behavior of a specific namespace connection.
	SocketOptions struct {
		auth       types.Optional[map[string]any]
		retries    types.Optional[float64]
		ackTimeout types.Optional[time.Duration]
	}
)

// DefaultSocketOptions creates a new SocketOptions instance with default values.
// Use this function to create a base configuration that can be customized.
func DefaultSocketOptions() *SocketOptions {
	return &SocketOptions{}
}

// Assign copies all options from another SocketOptionsInterface instance.
// If data is nil, it returns the current SocketOptions instance.
func (s *SocketOptions) Assign(data SocketOptionsInterface) SocketOptionsInterface {
	if data == nil {
		return s
	}

	if data.GetRawAuth() != nil {
		s.SetAuth(data.Auth())
	}
	if data.GetRawRetries() != nil {
		s.SetRetries(data.Retries())
	}
	if data.GetRawAckTimeout() != nil {
		s.SetAckTimeout(data.AckTimeout())
	}

	return s
}

// SetAuth configures the authentication data to be sent with the connection.
//
// Parameters:
//   - auth: A map containing authentication credentials or tokens
func (s *SocketOptions) SetAuth(auth map[string]any) {
	s.auth = types.NewSome(auth)
}
func (s *SocketOptions) GetRawAuth() types.Optional[map[string]any] {
	return s.auth
}
func (s *SocketOptions) Auth() map[string]any {
	if s.auth == nil {
		return nil
	}

	return s.auth.Get()
}

// SetRetries sets the maximum number of retries for packet delivery
//
// Parameters:
//   - retries: The maximum number of retries
func (s *SocketOptions) SetRetries(retries float64) {
	s.retries = types.NewSome(retries)
}
func (s *SocketOptions) GetRawRetries() types.Optional[float64] {
	return s.retries
}
func (s *SocketOptions) Retries() float64 {
	if s.retries == nil {
		return 0
	}

	return s.retries.Get()
}

// SetAckTimeout sets how long to wait for an acknowledgement before timing out.
//
// Parameters:
//   - d: The timeout duration
func (s *SocketOptions) SetAckTimeout(ackTimeout time.Duration) {
	s.ackTimeout = types.NewSome(ackTimeout)
}
func (s *SocketOptions) GetRawAckTimeout() types.Optional[time.Duration] {
	return s.ackTimeout
}
func (s *SocketOptions) AckTimeout() time.Duration {
	if s.ackTimeout == nil {
		return 0
	}

	return s.ackTimeout.Get()
}
