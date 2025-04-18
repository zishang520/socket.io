package socket

import "time"

// SocketOptionsInterface defines the interface for accessing and modifying Socket options.
// It provides methods for authentication, retry logic, and acknowledgement timeouts.
type (
	SocketOptionsInterface interface {
		// GetRawAuth returns the raw authentication data.
		GetRawAuth() map[string]any

		// Auth returns the authentication data that will be sent with the connection.
		Auth() map[string]any

		// SetAuth sets the authentication data for the socket connection.
		SetAuth(map[string]any)

		// GetRawRetries returns the raw retry count.
		GetRawRetries() *float64

		// Retries returns the maximum number of retries for packet delivery.
		Retries() float64

		// SetRetries sets the maximum number of retries for packet delivery.
		SetRetries(float64)

		// GetRawAckTimeout returns the raw acknowledgement timeout value.
		GetRawAckTimeout() *time.Duration

		// AckTimeout returns the timeout duration for acknowledgements.
		AckTimeout() time.Duration

		// SetAckTimeout sets the timeout duration for waiting for acknowledgements.
		SetAckTimeout(time.Duration)
	}

	// SocketOptions defines configuration options for individual Socket.IO sockets.
	// These options control the behavior of a specific namespace connection.
	SocketOptions struct {
		auth       map[string]any
		retries    *float64
		ackTimeout *time.Duration
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
	s.auth = auth
}

// GetRawAuth returns the raw authentication data configuration.
func (s *SocketOptions) GetRawAuth() map[string]any {
	return s.auth
}

// Auth returns the authentication data for the socket connection.
func (s *SocketOptions) Auth() map[string]any {
	return s.auth
}

// SetRetries sets the maximum number of retries for packet delivery
//
// Parameters:
//   - retries: The maximum number of retries
func (s *SocketOptions) SetRetries(retries float64) {
	s.retries = &retries
}

// GetRawRetries returns the raw retry count
func (s *SocketOptions) GetRawRetries() *float64 {
	return s.retries
}

// Retries returns the maximum number of retries for packet delivery
func (s *SocketOptions) Retries() float64 {
	if retries := s.retries; retries != nil {
		return *retries
	}

	return 0
}

// SetAckTimeout sets how long to wait for an acknowledgement before timing out.
//
// Parameters:
//   - d: The timeout duration
func (s *SocketOptions) SetAckTimeout(ackTimeout time.Duration) {
	s.ackTimeout = &ackTimeout
}

// GetRawAckTimeout returns the raw acknowledgement timeout setting.
func (s *SocketOptions) GetRawAckTimeout() *time.Duration {
	return s.ackTimeout
}

// AckTimeout returns the current acknowledgement timeout duration.
func (s *SocketOptions) AckTimeout() time.Duration {
	if ackTimeout := s.ackTimeout; ackTimeout != nil {
		return *ackTimeout
	}

	return 0
}
