package socket

import (
	"testing"
	"time"
)

func TestDefaultConstants(t *testing.T) {
	t.Run("DefaultTimeout", func(t *testing.T) {
		if DefaultTimeout != 20_000*time.Millisecond {
			t.Errorf("DefaultTimeout = %v, want %v", DefaultTimeout, 20_000*time.Millisecond)
		}
	})

	t.Run("DefaultReconnectionDelay", func(t *testing.T) {
		if DefaultReconnectionDelay != 1_000 {
			t.Errorf("DefaultReconnectionDelay = %v, want %v", DefaultReconnectionDelay, 1_000.0)
		}
	})

	t.Run("DefaultReconnectionDelayMax", func(t *testing.T) {
		if DefaultReconnectionDelayMax != 5_000 {
			t.Errorf("DefaultReconnectionDelayMax = %v, want %v", DefaultReconnectionDelayMax, 5_000.0)
		}
	})

	t.Run("DefaultRandomizationFactor", func(t *testing.T) {
		if DefaultRandomizationFactor != 0.5 {
			t.Errorf("DefaultRandomizationFactor = %v, want %v", DefaultRandomizationFactor, 0.5)
		}
	})

	t.Run("DelayMaxGreaterThanDelay", func(t *testing.T) {
		if DefaultReconnectionDelayMax <= DefaultReconnectionDelay {
			t.Errorf("DefaultReconnectionDelayMax (%v) should be > DefaultReconnectionDelay (%v)",
				DefaultReconnectionDelayMax, DefaultReconnectionDelay)
		}
	})

	t.Run("RandomizationFactorInRange", func(t *testing.T) {
		if DefaultRandomizationFactor < 0 || DefaultRandomizationFactor > 1 {
			t.Errorf("DefaultRandomizationFactor = %v, should be in [0, 1]", DefaultRandomizationFactor)
		}
	})
}
