package engine

import (
	"testing"
	"time"

	"github.com/zishang520/socket.io/servers/engine/v3/transports"
)

func TestDefaultWSBufferSizes(t *testing.T) {
	if DefaultWSReadBufferSize != 1024 {
		t.Errorf("DefaultWSReadBufferSize = %d, want 1024", DefaultWSReadBufferSize)
	}
	if DefaultWSWriteBufferSize != 1024 {
		t.Errorf("DefaultWSWriteBufferSize = %d, want 1024", DefaultWSWriteBufferSize)
	}
}

func TestDefaultUpgradeCheckInterval(t *testing.T) {
	expected := 100 * time.Millisecond
	if DefaultUpgradeCheckInterval != expected {
		t.Errorf("DefaultUpgradeCheckInterval = %v, want %v", DefaultUpgradeCheckInterval, expected)
	}
}

func TestDefaultPollingCloseTimeout(t *testing.T) {
	expected := 30_000 * time.Millisecond
	if transports.DefaultPollingCloseTimeout != expected {
		t.Errorf("DefaultPollingCloseTimeout = %v, want %v", transports.DefaultPollingCloseTimeout, expected)
	}
}
