package engine

import "time"

const (
	// DefaultNetworkStatusCheckInterval is the interval between network online/offline status checks.
	DefaultNetworkStatusCheckInterval = 3_000 * time.Millisecond

	// DefaultWebTransportUpgradeDelay is the delay applied to non-WebTransport probe opens
	// to give WebTransport priority during transport upgrade.
	DefaultWebTransportUpgradeDelay = 200 * time.Millisecond
)
