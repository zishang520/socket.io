package config

import (
	"time"
)

type (
	AttachOptionsInterface interface {
		SetPath(string)
		GetRawPath() *string
		Path() string

		SetDestroyUpgrade(bool)
		GetRawDestroyUpgrade() *bool
		DestroyUpgrade() bool

		SetDestroyUpgradeTimeout(time.Duration)
		GetRawDestroyUpgradeTimeout() *time.Duration
		DestroyUpgradeTimeout() time.Duration

		SetAddTrailingSlash(bool)
		GetRawAddTrailingSlash() *bool
		AddTrailingSlash() bool
	}

	AttachOptions struct {
		// name of the path to capture
		path *string

		// destroy unhandled upgrade requests
		destroyUpgrade *bool

		// milliseconds after which unhandled requests are ended
		destroyUpgradeTimeout *time.Duration

		// Whether we should add a trailing slash to the request path.
		addTrailingSlash *bool
	}
)

func DefaultAttachOptions() *AttachOptions {
	a := &AttachOptions{}
	return a
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
//
// Default: "/engine.io"
func (a *AttachOptions) SetPath(path string) {
	a.path = &path
}
func (a *AttachOptions) GetRawPath() *string {
	return a.path
}
func (a *AttachOptions) Path() string {
	if a.path == nil {
		return ""
	}

	return *a.path
}

// destroy unhandled upgrade requests
//
// Default: true
func (a *AttachOptions) SetDestroyUpgrade(destroyUpgrade bool) {
	a.destroyUpgrade = &destroyUpgrade
}
func (a *AttachOptions) GetRawDestroyUpgrade() *bool {
	return a.destroyUpgrade
}
func (a *AttachOptions) DestroyUpgrade() bool {
	if a.destroyUpgrade == nil {
		return false
	}

	return *a.destroyUpgrade
}

// milliseconds after which unhandled requests are ended
//
// Default: 1000
func (a *AttachOptions) SetDestroyUpgradeTimeout(destroyUpgradeTimeout time.Duration) {
	a.destroyUpgradeTimeout = &destroyUpgradeTimeout
}
func (a *AttachOptions) GetRawDestroyUpgradeTimeout() *time.Duration {
	return a.destroyUpgradeTimeout
}
func (a *AttachOptions) DestroyUpgradeTimeout() time.Duration {
	if a.destroyUpgradeTimeout == nil {
		return 0
	}

	return *a.destroyUpgradeTimeout
}

// Whether we should add a trailing slash to the request path.
//
// Default: true
func (a *AttachOptions) SetAddTrailingSlash(addTrailingSlash bool) {
	a.addTrailingSlash = &addTrailingSlash
}
func (a *AttachOptions) GetRawAddTrailingSlash() *bool {
	return a.addTrailingSlash
}
func (a *AttachOptions) AddTrailingSlash() bool {
	if a.addTrailingSlash == nil {
		return false
	}

	return *a.addTrailingSlash
}
