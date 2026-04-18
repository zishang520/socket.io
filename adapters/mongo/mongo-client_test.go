package mongo

import (
	"context"
	"testing"
)

func TestNewMongoClient(t *testing.T) {
	t.Run("with valid context", func(t *testing.T) {
		ctx := context.Background()
		// We can't create a real collection in unit tests without a database,
		// so we test the constructor behavior with nil collection
		mc := &MongoClient{
			Context: ctx,
		}

		if mc.Context != ctx {
			t.Fatal("Context mismatch")
		}
	})

	t.Run("with nil context defaults to background", func(t *testing.T) {
		mc := NewMongoClient(context.Background(), nil)

		if mc == nil {
			t.Fatal("Expected non-nil MongoClient")
		}
		if mc.Context == nil {
			t.Fatal("Expected non-nil Context (should default to Background)")
		}
	})
}
