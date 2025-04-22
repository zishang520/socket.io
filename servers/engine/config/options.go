package config

type (
	OptionsInterface interface {
		AttachOptionsInterface
		ServerOptionsInterface
	}

	Options struct {
		AttachOptions
		ServerOptions
	}
)

func DefaultOptions() *Options {
	a := &Options{}
	return a
}

func (a *Options) Assign(data OptionsInterface) OptionsInterface {
	if data == nil {
		return a
	}

	a.AttachOptions.Assign(data)
	a.ServerOptions.Assign(data)

	return a
}
