package socket

import (
	"testing"
	"time"
)

func TestDefaultConstants(t *testing.T) {
	t.Run("DefaultConnectTimeout", func(t *testing.T) {
		if DefaultConnectTimeout != 45_000*time.Millisecond {
			t.Errorf("DefaultConnectTimeout = %v, want %v", DefaultConnectTimeout, 45_000*time.Millisecond)
		}
	})

	t.Run("DefaultMaxDisconnectionDuration", func(t *testing.T) {
		expected := int64(2 * 60 * 1000)
		if DefaultMaxDisconnectionDuration != expected {
			t.Errorf("DefaultMaxDisconnectionDuration = %v, want %v", DefaultMaxDisconnectionDuration, expected)
		}
	})

	t.Run("DefaultSessionCleanupInterval", func(t *testing.T) {
		if DefaultSessionCleanupInterval != 60_000*time.Millisecond {
			t.Errorf("DefaultSessionCleanupInterval = %v, want %v", DefaultSessionCleanupInterval, 60_000*time.Millisecond)
		}
	})
}

func TestSessionCleanupIntervalConfig(t *testing.T) {
	t.Run("default value is zero (unset)", func(t *testing.T) {
		recovery := DefaultConnectionStateRecovery()
		if recovery.GetRawSessionCleanupInterval() != nil {
			t.Error("Expected default GetRawSessionCleanupInterval to be nil")
		}
		if recovery.SessionCleanupInterval() != 0 {
			t.Errorf("Expected default SessionCleanupInterval to be 0, got %v", recovery.SessionCleanupInterval())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		recovery := DefaultConnectionStateRecovery()
		interval := 30 * time.Second

		recovery.SetSessionCleanupInterval(interval)

		raw := recovery.GetRawSessionCleanupInterval()
		if raw == nil {
			t.Fatal("Expected GetRawSessionCleanupInterval to return non-nil value")
		}
		if raw.Get() != interval {
			t.Errorf("GetRawSessionCleanupInterval() = %v, want %v", raw.Get(), interval)
		}
		if recovery.SessionCleanupInterval() != interval {
			t.Errorf("SessionCleanupInterval() = %v, want %v", recovery.SessionCleanupInterval(), interval)
		}
	})

	t.Run("Assign copies SessionCleanupInterval", func(t *testing.T) {
		source := DefaultConnectionStateRecovery()
		source.SetSessionCleanupInterval(45 * time.Second)
		source.SetMaxDisconnectionDuration(300_000)
		source.SetSkipMiddlewares(true)

		target := DefaultConnectionStateRecovery()
		target.Assign(source)

		if target.SessionCleanupInterval() != 45*time.Second {
			t.Errorf("Assign did not copy SessionCleanupInterval: got %v, want %v", target.SessionCleanupInterval(), 45*time.Second)
		}
		if target.MaxDisconnectionDuration() != 300_000 {
			t.Errorf("Assign did not copy MaxDisconnectionDuration: got %v, want %v", target.MaxDisconnectionDuration(), int64(300_000))
		}
		if !target.SkipMiddlewares() {
			t.Error("Assign did not copy SkipMiddlewares")
		}
	})

	t.Run("Assign with nil does nothing", func(t *testing.T) {
		recovery := DefaultConnectionStateRecovery()
		recovery.SetSessionCleanupInterval(10 * time.Second)

		result := recovery.Assign(nil)
		if result != recovery {
			t.Error("Assign(nil) should return the receiver")
		}
		if recovery.SessionCleanupInterval() != 10*time.Second {
			t.Error("Assign(nil) should not modify existing values")
		}
	})

	t.Run("Assign does not overwrite with unset value", func(t *testing.T) {
		target := DefaultConnectionStateRecovery()
		target.SetSessionCleanupInterval(20 * time.Second)

		source := DefaultConnectionStateRecovery()
		// source.SessionCleanupInterval is not set

		target.Assign(source)
		if target.SessionCleanupInterval() != 20*time.Second {
			t.Errorf("Assign should not overwrite with unset value: got %v, want %v", target.SessionCleanupInterval(), 20*time.Second)
		}
	})
}
