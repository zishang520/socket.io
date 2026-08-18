package mongo

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

func TestNewMongoClient(t *testing.T) {
	t.Run("with valid context and collection", func(t *testing.T) {
		ctx := context.Background()
		collection := new(mongo.Collection)
		mc, err := NewMongoClient(ctx, collection)
		if err != nil {
			t.Fatal(err)
		}

		if mc.Collection() != collection {
			t.Fatal("Collection mismatch")
		}
		if mc.Context() != ctx {
			t.Fatal("Context mismatch")
		}
	})

	t.Run("with nil context defaults to background", func(t *testing.T) {
		mc, err := NewMongoClient(nil, new(mongo.Collection)) //nolint:staticcheck // Verify the nil-context fallback.
		if err != nil {
			t.Fatal(err)
		}

		if mc == nil {
			t.Fatal("Expected non-nil MongoClient")
		}
		if mc.Context() == nil {
			t.Fatal("Expected non-nil Context (should default to Background)")
		}
	})

	t.Run("requires collection", func(t *testing.T) {
		mc, err := NewMongoClient(context.Background(), nil)
		if mc != nil {
			t.Fatal("Expected nil MongoClient")
		}
		if !errors.Is(err, ErrMongoCollectionRequired) {
			t.Fatalf("error = %v, want %v", err, ErrMongoCollectionRequired)
		}
	})
}
