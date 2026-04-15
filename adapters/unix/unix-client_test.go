package unix

import (
	"context"
	"testing"
)

func TestNewUnixClient(t *testing.T) {
	t.Run("with valid context", func(t *testing.T) {
		ctx := context.Background()
		uc := &UnixClient{
			Context:    ctx,
			SocketPath: "/tmp/test.sock",
		}

		if uc.Context != ctx {
			t.Fatal("Context mismatch")
		}
		if uc.SocketPath != "/tmp/test.sock" {
			t.Fatal("SocketPath mismatch")
		}
	})

	t.Run("with nil context defaults to background", func(t *testing.T) {
		uc := NewUnixClient(context.Background(), "/tmp/test.sock")

		if uc == nil {
			t.Fatal("Expected non-nil UnixClient")
		}
		if uc.Context == nil {
			t.Fatal("Expected non-nil Context (should default to Background)")
		}
	})
}
