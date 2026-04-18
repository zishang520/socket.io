// Package adapter provides a MongoDB-based adapter implementation for Socket.IO clustering.
// It uses MongoDB Change Streams for pub/sub communication between nodes.
// The document format is compatible with the Node.js @socket.io/mongo-adapter package,
// allowing mixed Go and Node.js deployments.
package adapter

import (
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongod "go.mongodb.org/mongo-driver/v2/mongo"
)

// mongoLog is the logger for the MongoDB adapter.
var mongoLog = log.NewLog("socket.io-mongo")

// mongoAdapter implements the MongoAdapter interface using MongoDB Change Streams.
// It extends ClusterAdapterWithHeartbeat with MongoDB-specific functionality for
// message publishing and notification handling.
type mongoAdapter struct {
	adapter.ClusterAdapterWithHeartbeat

	mongoClient       *mongo.MongoClient
	opts              *MongoAdapterOptions
	addCreatedAtField bool
	cleanupFunc       types.Callable // Cleanup callback for resource management
}

// MakeMongoAdapter creates a new uninitialized mongoAdapter.
// Call Construct() to complete initialization before use.
func MakeMongoAdapter() MongoAdapter {
	a := &mongoAdapter{
		ClusterAdapterWithHeartbeat: adapter.MakeClusterAdapterWithHeartbeat(),
		opts:                        DefaultMongoAdapterOptions(),
		cleanupFunc:                 nil,
	}

	a.Prototype(a)

	return a
}

// NewMongoAdapter creates and initializes a new MongoDB adapter.
// This is the preferred way to create a MongoDB adapter instance.
func NewMongoAdapter(nsp socket.Namespace, client *mongo.MongoClient, opts any) MongoAdapter {
	a := MakeMongoAdapter()

	a.SetMongo(client)
	a.SetOpts(opts)
	a.Construct(nsp)

	return a
}

// SetMongo sets the MongoDB client for the adapter.
func (a *mongoAdapter) SetMongo(client *mongo.MongoClient) {
	a.mongoClient = client
}

// SetOpts sets the configuration options for the adapter.
// Options are merged with the parent ClusterAdapterWithHeartbeat options.
func (a *mongoAdapter) SetOpts(opts any) {
	a.ClusterAdapterWithHeartbeat.SetOpts(opts)

	if options, ok := opts.(MongoAdapterOptionsInterface); ok {
		a.opts.Assign(options)
		a.addCreatedAtField = options.AddCreatedAtField()
	}
}

// Construct initializes the MongoDB adapter for the given namespace.
// This method must be called before using the adapter.
func (a *mongoAdapter) Construct(nsp socket.Namespace) {
	a.ClusterAdapterWithHeartbeat.Construct(nsp)
}

// DoPublish publishes a cluster message to other nodes by inserting a document into MongoDB.
// Returns the hex-encoded ObjectID of the inserted document as the offset,
// matching the Node.js adapter's publish() return value.
func (a *mongoAdapter) DoPublish(message *ClusterMessage) (adapter.Offset, error) {
	mongoLog.Debug("publishing message of type %d", message.Type)

	doc := bson.D{
		{Key: "uid", Value: string(message.Uid)},
		{Key: "nsp", Value: message.Nsp},
		{Key: "type", Value: message.Type},
	}

	if message.Data != nil {
		doc = append(doc, bson.E{Key: "data", Value: message.Data})
	}

	if a.addCreatedAtField {
		doc = append(doc, bson.E{Key: "createdAt", Value: time.Now()})
	}

	result, err := a.mongoClient.Collection.InsertOne(a.mongoClient.Context, doc)
	if err != nil {
		return "", err
	}

	// Convert the inserted ID to hex string offset, matching Node.js:
	// result.insertedId.toString("hex")
	if oid, ok := result.InsertedID.(bson.ObjectID); ok {
		return adapter.Offset(oid.Hex()), nil
	}

	return "", nil
}

// DoPublishResponse publishes a response message to the cluster.
// This is used for request-response patterns between nodes.
func (a *mongoAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *ClusterResponse) error {
	_, err := a.DoPublish(response)
	return err
}

// OnEvent processes a change stream document from MongoDB.
// It decodes the BSON data based on message type and routes it to OnMessage.
func (a *mongoAdapter) OnEvent(document *mongo.AdapterEvent) {
	// The change stream pipeline already filters by operationType=insert,
	// but we still check uid to filter out our own messages.
	if document.Uid == a.Uid() {
		return
	}

	mongoLog.Debug("new event of type %d from %s", document.Type, document.Uid)

	// Build ClusterMessage with decoded data
	message := &ClusterMessage{
		Uid:  document.Uid,
		Nsp:  document.Nsp,
		Type: document.Type,
	}

	// Decode the data field based on message type (two-pass decoding)
	if document.Data.Type != 0 {
		data, err := a.decodeBsonData(document.Type, document.Data)
		if err != nil {
			mongoLog.Debug("failed to decode data for type %d: %s", document.Type, err.Error())
			return
		}
		message.Data = data
	}

	// The offset is the hex string of the ObjectID, matching Node.js:
	// result.insertedId.toString("hex")
	offset := adapter.Offset(document.ID.Hex())
	a.OnMessage(message, offset)
}

// Cleanup registers a cleanup callback to be called when the adapter is closed.
func (a *mongoAdapter) Cleanup(cleanup func()) {
	a.cleanupFunc = cleanup
}

// Close releases resources and invokes the registered cleanup callback.
func (a *mongoAdapter) Close() {
	defer a.ClusterAdapterWithHeartbeat.Close()

	if a.cleanupFunc != nil {
		a.cleanupFunc()
	}
}

// buildChangeStreamPipeline creates the MongoDB aggregation pipeline for the Change Stream.
// Unlike the Node.js version which filters by uid != self.uid, we handle self-message
// filtering in OnMessage() to avoid pipeline rebuilds on uid changes.
func buildChangeStreamPipeline() mongod.Pipeline {
	return mongod.Pipeline{
		{{Key: "$match", Value: bson.D{
			{Key: "operationType", Value: "insert"},
		}}},
	}
}

// decodeBsonData deserializes a BSON data value based on the message type.
func (a *mongoAdapter) decodeBsonData(messageType adapter.MessageType, rawData bson.RawValue) (any, error) {
	target := allocateTarget(messageType)
	if target == nil {
		return nil, nil
	}

	if err := rawData.Unmarshal(target); err != nil {
		return nil, err
	}

	return target, nil
}

// allocateTarget returns a pointer to the appropriate struct for the given message type.
func allocateTarget(messageType adapter.MessageType) any {
	switch messageType {
	case adapter.INITIAL_HEARTBEAT, adapter.HEARTBEAT, adapter.ADAPTER_CLOSE:
		return nil
	case adapter.BROADCAST:
		return &BroadcastMessage{}
	case adapter.SOCKETS_JOIN, adapter.SOCKETS_LEAVE:
		return &SocketsJoinLeaveMessage{}
	case adapter.DISCONNECT_SOCKETS:
		return &DisconnectSocketsMessage{}
	case adapter.FETCH_SOCKETS:
		return &FetchSocketsMessage{}
	case adapter.FETCH_SOCKETS_RESPONSE:
		return &FetchSocketsResponse{}
	case adapter.SERVER_SIDE_EMIT:
		return &ServerSideEmitMessage{}
	case adapter.SERVER_SIDE_EMIT_RESPONSE:
		return &ServerSideEmitResponse{}
	case adapter.BROADCAST_CLIENT_COUNT:
		return &BroadcastClientCount{}
	case adapter.BROADCAST_ACK:
		return &BroadcastAck{}
	default:
		return nil
	}
}
