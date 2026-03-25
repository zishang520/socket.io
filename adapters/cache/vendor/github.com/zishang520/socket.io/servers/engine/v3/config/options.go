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
	return &Options{}
}

func (o *Options) Assign(data OptionsInterface) OptionsInterface {
	if data == nil {
		return o
	}

	o.AttachOptions.Assign(data)
	o.ServerOptions.Assign(data)

	return o
}
