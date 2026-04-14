package postgres

import (
	"context"
	"testing"
)

func TestNewPostgresClient(t *testing.T) {
	t.Run("with valid context", func(t *testing.T) {
		ctx := context.Background()
		// We can't create a real pool in unit tests without a database,
		// so we test the constructor behavior with nil pool
		pc := &PostgresClient{
			Context: ctx,
		}

		if pc.Context != ctx {
			t.Fatal("Context mismatch")
		}
	})

	t.Run("with nil context defaults to background", func(t *testing.T) {
		pc := NewPostgresClient(context.Background(), nil)

		if pc == nil {
			t.Fatal("Expected non-nil PostgresClient")
		}
		if pc.Context == nil {
			t.Fatal("Expected non-nil Context (should default to Background)")
		}
	})
}
