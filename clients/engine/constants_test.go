package engine

import (
	"testing"
	"time"
)

func TestDefaultConstants(t *testing.T) {
	t.Run("DefaultNetworkStatusCheckInterval", func(t *testing.T) {
		expected := 3_000 * time.Millisecond
		if DefaultNetworkStatusCheckInterval != expected {
			t.Errorf("DefaultNetworkStatusCheckInterval = %v, want %v", DefaultNetworkStatusCheckInterval, expected)
		}
	})

	t.Run("DefaultWebTransportUpgradeDelay", func(t *testing.T) {
		expected := 200 * time.Millisecond
		if DefaultWebTransportUpgradeDelay != expected {
			t.Errorf("DefaultWebTransportUpgradeDelay = %v, want %v", DefaultWebTransportUpgradeDelay, expected)
		}
	})
}

func TestStopNetworkMonitoring(t *testing.T) {
	// stopNetworkMonitoring should not panic even if called multiple times
	stopNetworkMonitoring()
	stopNetworkMonitoring()
}
