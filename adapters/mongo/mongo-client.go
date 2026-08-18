// Package mongo provides MongoDB client wrapper for Socket.IO MongoDB adapter.
// This package offers a unified interface for MongoDB operations with event handling support
// using Change Streams for pub/sub communication.
package mongo

import (
	"context"
	"errors"

	"github.com/zishang520/socket.io/v3/pkg/types"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ErrMongoCollectionRequired is returned when no MongoDB collection is provided.
var ErrMongoCollectionRequired = errors.New("mongo: collection is required")

// MongoClient wraps a mongo.Collection and provides context management
// and event emitting capabilities for the Socket.IO MongoDB adapter.
//
// The client uses a dedicated MongoDB collection for pub/sub communication.
// Documents are inserted for publishing and Change Streams are used for subscribing.
//
// The client supports error event emission, which allows higher-level components
// to handle MongoDB-related errors gracefully. Its collection and context are
// immutable after construction. The zero value is not usable; create clients
// with NewMongoClient.
type MongoClient struct {
	types.EventEmitter

	collection *mongo.Collection
	ctx        context.Context
}

// Collection returns the MongoDB collection used for pub/sub communication.
func (m *MongoClient) Collection() *mongo.Collection {
	return m.collection
}

// Context returns the context controlling MongoDB operations and subscriptions.
func (m *MongoClient) Context() context.Context {
	return m.ctx
}

// NewMongoClient creates a new MongoClient with the given context and MongoDB collection.
//
// Parameters:
//   - ctx: The context that controls the lifecycle of MongoDB operations.
//     When canceled, all subscriptions and pending operations will be terminated.
//   - collection: A mongo.Collection instance that handles the actual MongoDB communication.
//     The collection should be either a capped collection or have a TTL index.
//
// Returns:
//   - A pointer to the initialized MongoClient instance, or an error when the
//     configuration is invalid.
//
// Example:
//
//	client, _ := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
//	collection := client.Database("mydb").Collection("socket.io-adapter-events")
//	mongoClient, err := NewMongoClient(context.Background(), collection)
func NewMongoClient(ctx context.Context, collection *mongo.Collection) (*MongoClient, error) {
	if collection == nil {
		return nil, ErrMongoCollectionRequired
	}
	if ctx == nil {
		ctx = context.Background()
	}

	return &MongoClient{
		EventEmitter: types.NewEventEmitter(),
		collection:   collection,
		ctx:          ctx,
	}, nil
}
