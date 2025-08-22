package types

type (
	Void = struct{}

	Callable = func()

	HttpCompression struct {
		Threshold int `json:"threshold,omitempty" msgpack:"threshold,omitempty"`
	}

	PerMessageDeflate struct {
		Threshold int `json:"threshold,omitempty" msgpack:"threshold,omitempty"`
	}
)

var (
	NULL Void
)

// noCopy may be added to structs which must not be copied
// after the first use.
//
// See https://golang.org/issues/8005#issuecomment-190753527
// for details.
//
// Note that it must not be embedded, due to the Lock and Unlock methods.
type noCopy struct{}

// Lock is a no-op used by -copylocks checker from `go vet`.
func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}
