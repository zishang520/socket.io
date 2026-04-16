// Package mongo provides MongoDB client wrapper for Socket.IO MongoDB adapter.
// This package offers a unified interface for MongoDB operations with event handling support
// using Change Streams for pub/sub communication.
package mongo

import (
	"context"

	"github.com/zishang520/socket.io/v3/pkg/types"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// MongoClient wraps a mongo.Collection and provides context management
// and event emitting capabilities for the Socket.IO MongoDB adapter.
//
// The client uses a dedicated MongoDB collection for pub/sub communication.
// Documents are inserted for publishing and Change Streams are used for subscribing.
//
// The client supports error event emission, which allows higher-level components
// to handle MongoDB-related errors gracefully.
type MongoClient struct {
	types.EventEmitter

	// Collection is the MongoDB collection used for pub/sub communication.
	// All adapter events are stored as documents in this collection.
	Collection *mongo.Collection

	// Context is the context used for MongoDB operations.
	// This context controls the lifecycle of subscriptions and operations.
	Context context.Context
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
//   - A pointer to the initialized MongoClient instance.
//
// Example:
//
//	client, _ := mongo.Connect(options.Client().ApplyURI("mongodb://localhost:27017"))
//	collection := client.Database("mydb").Collection("socket.io-adapter-events")
//	mongoClient := NewMongoClient(context.Background(), collection)
func NewMongoClient(ctx context.Context, collection *mongo.Collection) *MongoClient {
	if ctx == nil {
		ctx = context.Background()
	}

	return &MongoClient{
		EventEmitter: types.NewEventEmitter(),
		Collection:   collection,
		Context:      ctx,
	}
}
