// Package adapter defines types and interfaces for the MongoDB-based Socket.IO adapter implementation.
// It uses MongoDB Change Streams for inter-node communication in a clustered Socket.IO environment.
package adapter

import (
	"sync/atomic"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type (
	// ClusterMessage is an alias for adapter.ClusterMessage.
	ClusterMessage = adapter.ClusterMessage

	// ClusterResponse is an alias for adapter.ClusterResponse.
	ClusterResponse = adapter.ClusterResponse

	// BroadcastMessage is an alias for adapter.BroadcastMessage.
	BroadcastMessage = adapter.BroadcastMessage

	// SocketsJoinLeaveMessage is an alias for adapter.SocketsJoinLeaveMessage.
	SocketsJoinLeaveMessage = adapter.SocketsJoinLeaveMessage

	// DisconnectSocketsMessage is an alias for adapter.DisconnectSocketsMessage.
	DisconnectSocketsMessage = adapter.DisconnectSocketsMessage

	// FetchSocketsMessage is an alias for adapter.FetchSocketsMessage.
	FetchSocketsMessage = adapter.FetchSocketsMessage

	// FetchSocketsResponse is an alias for adapter.FetchSocketsResponse.
	FetchSocketsResponse = adapter.FetchSocketsResponse

	// ServerSideEmitMessage is an alias for adapter.ServerSideEmitMessage.
	ServerSideEmitMessage = adapter.ServerSideEmitMessage

	// ServerSideEmitResponse is an alias for adapter.ServerSideEmitResponse.
	ServerSideEmitResponse = adapter.ServerSideEmitResponse

	// BroadcastClientCount is an alias for adapter.BroadcastClientCount.
	BroadcastClientCount = adapter.BroadcastClientCount

	// BroadcastAck is an alias for adapter.BroadcastAck.
	BroadcastAck = adapter.BroadcastAck

	// MongoAdapter defines the interface for a MongoDB-based Socket.IO adapter.
	// It extends ClusterAdapterWithHeartbeat with MongoDB-specific functionality.
	MongoAdapter interface {
		adapter.ClusterAdapterWithHeartbeat

		// SetMongo configures the MongoDB client for the adapter.
		SetMongo(*mongo.MongoClient)

		// Cleanup registers a cleanup callback to be called when the adapter is closed.
		Cleanup(func())

		// OnEvent processes a change stream document from MongoDB.
		OnEvent(document *mongo.AdapterEvent)
	}
)

// MongoAdapterBuilder creates MongoDB adapters for Socket.IO namespaces.
// It manages the shared Change Stream connection across all namespace adapters.
type MongoAdapterBuilder struct {
	// Mongo is the MongoDB client used for operations.
	Mongo *mongo.MongoClient
	// Opts contains configuration options for the adapter.
	Opts MongoAdapterOptionsInterface

	namespaceToAdapters types.Map[string, MongoAdapter]
	listening           atomic.Bool
	isClosed            atomic.Bool
}

// New creates a new MongoAdapter for the given namespace.
// This method implements the socket.AdapterBuilder interface.
func (mb *MongoAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	options := DefaultMongoAdapterOptions()
	options.Assign(mb.Opts)

	// Apply defaults
	if options.GetRawHeartbeatInterval() == nil {
		options.SetHeartbeatInterval(DefaultHeartbeatInterval)
	}
	if options.GetRawHeartbeatTimeout() == nil {
		options.SetHeartbeatTimeout(DefaultHeartbeatTimeout)
	}

	adapterInstance := NewMongoAdapter(nsp, mb.Mongo, options)

	mb.namespaceToAdapters.Store(nsp.Name(), adapterInstance)

	// Start listening if not already
	if mb.listening.CompareAndSwap(false, true) {
		mb.isClosed.Store(false)
		go mb.startChangeStream()
	}

	// Register cleanup callback
	adapterInstance.Cleanup(func() {
		mb.namespaceToAdapters.Delete(nsp.Name())

		// If no more adapters, close the change stream
		hasAdapters := false
		mb.namespaceToAdapters.Range(func(_ string, _ MongoAdapter) bool {
			hasAdapters = true
			return false
		})
		if !hasAdapters {
			mb.isClosed.Store(true)
		}
	})

	return adapterInstance
}

// startChangeStream opens a MongoDB Change Stream and dispatches events
// to the appropriate namespace adapter.
func (mb *MongoAdapterBuilder) startChangeStream() {
	for !mb.isClosed.Load() {
		mb.watchChangeStream()

		if mb.isClosed.Load() {
			return
		}

		// Brief delay before reconnecting to avoid tight error loop
		// Matches Node.js behavior: setTimeout(() => initChangeStream(), 1000)
		time.Sleep(1 * time.Second)
	}
}

// watchChangeStream opens and processes a single Change Stream session.
// Returns when the stream is closed or encounters a non-recoverable error.
func (mb *MongoAdapterBuilder) watchChangeStream() {
	mongoLog.Debug("opening change stream")

	cs, err := mb.Mongo.Collection.Watch(mb.Mongo.Context, buildChangeStreamPipeline())
	if err != nil {
		mongoLog.Debug("failed to open change stream: %s", err.Error())
		mb.Mongo.Emit("error", err)
		return
	}
	defer func() { _ = cs.Close(mb.Mongo.Context) }()

	for cs.Next(mb.Mongo.Context) {
		if mb.isClosed.Load() {
			return
		}

		var event struct {
			OperationType string              `bson:"operationType"`
			FullDocument  *mongo.AdapterEvent `bson:"fullDocument"`
		}
		if err := cs.Decode(&event); err != nil {
			mongoLog.Debug("failed to decode change stream event: %s", err.Error())
			continue
		}

		if event.OperationType != "insert" {
			continue
		}

		doc := event.FullDocument
		if doc == nil {
			continue
		}

		// Route to the appropriate namespace adapter
		if adapterInstance, ok := mb.namespaceToAdapters.Load(doc.Nsp); ok {
			adapterInstance.OnEvent(doc)
		}
	}
	if err := cs.Err(); err != nil {
		if mb.Mongo.Context.Err() != nil {
			return // Context canceled, stop listening
		}
		mongoLog.Debug("change stream error: %s", err.Error())
		mb.Mongo.Emit("error", err)
	}
}
