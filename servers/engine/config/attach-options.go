// Package config provides attach options for configuring how the Engine.IO server is attached to an HTTP server.
package config

import (
	"time"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	AttachOptionsInterface interface {
		SetPath(string)
		GetRawPath() types.Optional[string]
		Path() string

		SetDestroyUpgrade(bool)
		GetRawDestroyUpgrade() types.Optional[bool]
		DestroyUpgrade() bool

		SetDestroyUpgradeTimeout(time.Duration)
		GetRawDestroyUpgradeTimeout() types.Optional[time.Duration]
		DestroyUpgradeTimeout() time.Duration

		SetAddTrailingSlash(bool)
		GetRawAddTrailingSlash() types.Optional[bool]
		AddTrailingSlash() bool
	}

	AttachOptions struct {
		// name of the path to capture
		path types.Optional[string]

		// destroy unhandled upgrade requests
		destroyUpgrade types.Optional[bool]

		// milliseconds after which unhandled requests are ended
		destroyUpgradeTimeout types.Optional[time.Duration]

		// Whether we should add a trailing slash to the request path.
		addTrailingSlash types.Optional[bool]
	}
)

func DefaultAttachOptions() *AttachOptions {
	return &AttachOptions{}
}

func (a *AttachOptions) Assign(data AttachOptionsInterface) AttachOptionsInterface {
	if data == nil {
		return a
	}

	if data.GetRawPath() != nil {
		a.SetPath(data.Path())
	}

	if data.GetRawDestroyUpgradeTimeout() != nil {
		a.SetDestroyUpgradeTimeout(data.DestroyUpgradeTimeout())
	}

	if data.GetRawDestroyUpgrade() != nil {
		a.SetDestroyUpgrade(data.DestroyUpgrade())
	}

	if data.GetRawAddTrailingSlash() != nil {
		a.SetAddTrailingSlash(data.AddTrailingSlash())
	}

	return a
}

// name of the path to capture
func (a *AttachOptions) SetPath(path string) {
	a.path = types.NewSome(path)
}
func (a *AttachOptions) GetRawPath() types.Optional[string] {
	return a.path
}
func (a *AttachOptions) Path() string {
	if a.path == nil {
		return ""
	}

	return a.path.Get()
}

// destroy unhandled upgrade requests
func (a *AttachOptions) SetDestroyUpgrade(destroyUpgrade bool) {
	a.destroyUpgrade = types.NewSome(destroyUpgrade)
}
func (a *AttachOptions) GetRawDestroyUpgrade() types.Optional[bool] {
	return a.destroyUpgrade
}
func (a *AttachOptions) DestroyUpgrade() bool {
	if a.destroyUpgrade == nil {
		return false
	}

	return a.destroyUpgrade.Get()
}

// milliseconds after which unhandled requests are ended
func (a *AttachOptions) SetDestroyUpgradeTimeout(destroyUpgradeTimeout time.Duration) {
	a.destroyUpgradeTimeout = types.NewSome(destroyUpgradeTimeout)
}
func (a *AttachOptions) GetRawDestroyUpgradeTimeout() types.Optional[time.Duration] {
	return a.destroyUpgradeTimeout
}
func (a *AttachOptions) DestroyUpgradeTimeout() time.Duration {
	if a.destroyUpgradeTimeout == nil {
		return 0
	}

	return a.destroyUpgradeTimeout.Get()
}

// Whether we should add a trailing slash to the request path.
func (a *AttachOptions) SetAddTrailingSlash(addTrailingSlash bool) {
	a.addTrailingSlash = types.NewSome(addTrailingSlash)
}
func (a *AttachOptions) GetRawAddTrailingSlash() types.Optional[bool] {
	return a.addTrailingSlash
}
func (a *AttachOptions) AddTrailingSlash() bool {
	if a.addTrailingSlash == nil {
		return false
	}

	return a.addTrailingSlash.Get()
}
