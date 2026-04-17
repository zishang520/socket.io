package socket

import "time"

const (
	// DefaultTimeout is the default connection timeout for the Manager.
	DefaultTimeout = 20_000 * time.Millisecond

	// DefaultReconnectionDelay is the default initial reconnection delay in milliseconds.
	DefaultReconnectionDelay float64 = 1_000

	// DefaultReconnectionDelayMax is the default maximum reconnection delay in milliseconds.
	DefaultReconnectionDelayMax float64 = 5_000

	// DefaultRandomizationFactor is the default jitter factor applied to reconnection delays.
	DefaultRandomizationFactor float64 = 0.5
)
