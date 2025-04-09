package socket

type (
	OptionsInterface interface {
		ManagerOptionsInterface
		SocketOptionsInterface
	}

	Options struct {
		ManagerOptions
		SocketOptions
	}
)

func DefaultOptions() *Options {
	return &Options{}
}

func (s *Options) Assign(data OptionsInterface) OptionsInterface {
	if data == nil {
		return s
	}

	s.ManagerOptions.Assign(data)
	s.SocketOptions.Assign(data)

	return s
}
