package socket

// OptionsInterface combines ManagerOptionsInterface and SocketOptionsInterface for unified configuration.
type (
	OptionsInterface interface {
		ManagerOptionsInterface
		SocketOptionsInterface
	}

	// Options holds both manager and socket options for a Socket.IO client.
	Options struct {
		ManagerOptions
		SocketOptions
	}
)

// DefaultOptions returns a new Options instance with default values.
func DefaultOptions() *Options {
	return &Options{}
}

// Assign copies all options from another OptionsInterface instance.
// If data is nil, it returns the current Options instance.
func (s *Options) Assign(data OptionsInterface) OptionsInterface {
	if data == nil {
		return s
	}

	s.ManagerOptions.Assign(data)
	s.SocketOptions.Assign(data)

	return s
}
