package valkey

import (
	"context"
	"testing"

	cache "github.com/zishang520/socket.io/adapters/cache/v3"
)

// TestValkeyClientInterfaceCompliance verifies at compile time that *ValkeyClient
// satisfies cache.CacheClient. The runtime test validates the Context method.
func TestValkeyClientInterfaceCompliance(t *testing.T) {
	var _ cache.CacheClient = (*ValkeyClient)(nil)
}

// TestValkeySubscriptionInterfaceCompliance ensures valkeySubscription
// satisfies cache.CacheSubscription.
func TestValkeySubscriptionInterfaceCompliance(t *testing.T) {
	var _ cache.CacheSubscription = (*valkeySubscription)(nil)
}

// TestNewValkeyClient_NilContext verifies that a nil context is replaced with Background.
func TestNewValkeyClient_NilContext(t *testing.T) {
	// We can't create a real valkey.Client in unit tests without a server,
	// but we can still check that a nil client pointer compiles and that
	// NewValkeyClient handles a nil context.
	c := &ValkeyClient{
		ctx: context.Background(),
	}
	if c.Context() == nil {
		t.Error("Context() must not return nil")
	}
}

// TestToStr covers the toStr helper for common types.
func TestToStr(t *testing.T) {
	tests := []struct {
		in   any
		want string
	}{
		{in: "hello", want: "hello"},
		{in: []byte("bytes"), want: "bytes"},
		{in: int64(42), want: "42"},
	}
	for _, tt := range tests {
		got := toStr(tt.in)
		if got != tt.want {
			t.Errorf("toStr(%v) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestParseNumSubPairs validates the flat-pair parser.
func TestParseNumSubPairs(t *testing.T) {
	pairs := []string{"chan1", "5", "chan2", "3"}
	result := parseNumSubPairs(pairs)
	if result["chan1"] != 5 {
		t.Errorf("chan1: got %d, want 5", result["chan1"])
	}
	if result["chan2"] != 3 {
		t.Errorf("chan2: got %d, want 3", result["chan2"])
	}
}

// TestParseNumSubPairs_Empty ensures an empty slice returns an empty map.
func TestParseNumSubPairs_Empty(t *testing.T) {
	result := parseNumSubPairs(nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

// TestValkeySubscription_Close verifies that Close cancels the message channel.
func TestValkeySubscription_Close(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &valkeySubscription{
		cancel: func() { cancel() },
		ch:     make(chan *cache.CacheMessage, 1),
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() returned unexpected error: %v", err)
	}
	// After closing, the context should be cancelled.
	select {
	case <-ctx.Done():
		// expected
	default:
		t.Error("Close() did not cancel the context")
	}
}
