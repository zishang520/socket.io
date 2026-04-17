package socket

import (
	"math"
	"testing"
	"time"
)

func newTestManager() *Manager {
	opts := DefaultManagerOptions()
	opts.SetAutoConnect(false)
	m := MakeManager()
	m.Construct("http://localhost:3000", opts)
	return m
}

func TestNewManagerDefaults(t *testing.T) {
	m := newTestManager()

	t.Run("Reconnection enabled", func(t *testing.T) {
		if !m.Reconnection() {
			t.Error("Expected Reconnection() to be true by default")
		}
	})

	t.Run("ReconnectionAttempts is Inf", func(t *testing.T) {
		if !math.IsInf(m.ReconnectionAttempts(), 1) {
			t.Errorf("Expected ReconnectionAttempts() = +Inf, got %v", m.ReconnectionAttempts())
		}
	})

	t.Run("ReconnectionDelay", func(t *testing.T) {
		if m.ReconnectionDelay() != DefaultReconnectionDelay {
			t.Errorf("ReconnectionDelay() = %v, want %v", m.ReconnectionDelay(), DefaultReconnectionDelay)
		}
	})

	t.Run("ReconnectionDelayMax", func(t *testing.T) {
		if m.ReconnectionDelayMax() != DefaultReconnectionDelayMax {
			t.Errorf("ReconnectionDelayMax() = %v, want %v", m.ReconnectionDelayMax(), DefaultReconnectionDelayMax)
		}
	})

	t.Run("RandomizationFactor", func(t *testing.T) {
		if m.RandomizationFactor() != DefaultRandomizationFactor {
			t.Errorf("RandomizationFactor() = %v, want %v", m.RandomizationFactor(), DefaultRandomizationFactor)
		}
	})

	t.Run("Timeout", func(t *testing.T) {
		timeout := m.Timeout()
		if timeout == nil {
			t.Fatal("Expected Timeout() to be non-nil")
		}
		if *timeout != DefaultTimeout {
			t.Errorf("Timeout() = %v, want %v", *timeout, DefaultTimeout)
		}
	})

	t.Run("Opts returns non-nil", func(t *testing.T) {
		if m.Opts() == nil {
			t.Error("Expected Opts() to be non-nil")
		}
	})

	t.Run("Engine is nil before connect", func(t *testing.T) {
		if m.Engine() != nil {
			t.Error("Expected Engine() to be nil before connect")
		}
	})
}

func TestManagerCustomOptions(t *testing.T) {
	opts := DefaultManagerOptions()
	opts.SetAutoConnect(false)
	opts.SetReconnectionDelay(2_000)
	opts.SetReconnectionDelayMax(10_000)
	opts.SetRandomizationFactor(0.3)
	opts.SetTimeout(30 * time.Second)
	opts.SetReconnection(false)
	opts.SetReconnectionAttempts(5)

	m := MakeManager()
	m.Construct("http://localhost:3000", opts)

	if m.ReconnectionDelay() != 2_000 {
		t.Errorf("ReconnectionDelay() = %v, want 2000", m.ReconnectionDelay())
	}
	if m.ReconnectionDelayMax() != 10_000 {
		t.Errorf("ReconnectionDelayMax() = %v, want 10000", m.ReconnectionDelayMax())
	}
	if m.RandomizationFactor() != 0.3 {
		t.Errorf("RandomizationFactor() = %v, want 0.3", m.RandomizationFactor())
	}
	timeout := m.Timeout()
	if timeout == nil || *timeout != 30*time.Second {
		t.Errorf("Timeout() = %v, want 30s", timeout)
	}
	if m.Reconnection() {
		t.Error("Expected Reconnection() to be false")
	}
	if m.ReconnectionAttempts() != 5 {
		t.Errorf("ReconnectionAttempts() = %v, want 5", m.ReconnectionAttempts())
	}
}

func TestManagerSetters(t *testing.T) {
	m := newTestManager()

	t.Run("SetReconnection", func(t *testing.T) {
		m.SetReconnection(false)
		if m.Reconnection() {
			t.Error("Expected Reconnection() to be false after SetReconnection(false)")
		}
		m.SetReconnection(true)
		if !m.Reconnection() {
			t.Error("Expected Reconnection() to be true after SetReconnection(true)")
		}
	})

	t.Run("SetReconnectionDelay", func(t *testing.T) {
		m.SetReconnectionDelay(3_000)
		if m.ReconnectionDelay() != 3_000 {
			t.Errorf("ReconnectionDelay() = %v, want 3000", m.ReconnectionDelay())
		}
	})

	t.Run("SetReconnectionDelayMax", func(t *testing.T) {
		m.SetReconnectionDelayMax(15_000)
		if m.ReconnectionDelayMax() != 15_000 {
			t.Errorf("ReconnectionDelayMax() = %v, want 15000", m.ReconnectionDelayMax())
		}
	})

	t.Run("SetRandomizationFactor", func(t *testing.T) {
		m.SetRandomizationFactor(0.7)
		if m.RandomizationFactor() != 0.7 {
			t.Errorf("RandomizationFactor() = %v, want 0.7", m.RandomizationFactor())
		}
	})

	t.Run("SetTimeout", func(t *testing.T) {
		m.SetTimeout(10 * time.Second)
		timeout := m.Timeout()
		if timeout == nil || *timeout != 10*time.Second {
			t.Errorf("Timeout() = %v, want 10s", timeout)
		}
	})

	t.Run("SetReconnectionAttempts", func(t *testing.T) {
		m.SetReconnectionAttempts(10)
		if m.ReconnectionAttempts() != 10 {
			t.Errorf("ReconnectionAttempts() = %v, want 10", m.ReconnectionAttempts())
		}
	})
}

func TestManagerDefaultPath(t *testing.T) {
	opts := DefaultManagerOptions()
	opts.SetAutoConnect(false)
	m := MakeManager()
	m.Construct("http://localhost:3000", opts)

	path := m.Opts().Path()
	if path != "/socket.io" {
		t.Errorf("Default path = %q, want %q", path, "/socket.io")
	}
}

func TestManagerCustomPath(t *testing.T) {
	opts := DefaultManagerOptions()
	opts.SetAutoConnect(false)
	opts.SetPath("/custom")
	m := MakeManager()
	m.Construct("http://localhost:3000", opts)

	path := m.Opts().Path()
	if path != "/custom" {
		t.Errorf("Path = %q, want %q", path, "/custom")
	}
}

func TestManagerNilOpts(t *testing.T) {
	m := MakeManager()
	// Should not panic with nil opts
	m.Construct("http://localhost:3000", nil)

	if m.Opts() == nil {
		t.Error("Expected Opts() to be non-nil even when constructed with nil")
	}
}
