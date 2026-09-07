// Package adapter defines types and interfaces for the MongoDB-based Socket.IO adapter implementation.
// It uses MongoDB Change Streams for inter-node communication in a clustered Socket.IO environment.
package adapter

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongod "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
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

	// MongoAdapter keeps the cluster adapter API while implementing MongoDB's
	// standalone heartbeat and session protocol.
	MongoAdapter interface {
		adapter.ClusterAdapter

		// SetOpts configures adapter options.
		SetOpts(any)

		// SetMongo configures the MongoDB client for the adapter.
		SetMongo(*mongo.MongoClient)

		// Cleanup registers a cleanup callback to be called when the adapter is closed.
		Cleanup(func())

		// OnEvent processes a change stream document from MongoDB.
		OnEvent(document *mongo.AdapterEvent)
	}

	changeStreamEvent struct {
		OperationType string              `bson:"operationType"`
		FullDocument  *mongo.AdapterEvent `bson:"fullDocument"`
	}
)

// MongoAdapterBuilder creates MongoDB adapters for Socket.IO namespaces.
// It manages the shared Change Stream and routes events to namespace adapters.
type MongoAdapterBuilder struct {
	Mongo *mongo.MongoClient
	Opts  MongoAdapterOptionsInterface

	mu               sync.Mutex
	adapters         types.Map[string, MongoAdapter]
	uid              adapter.ServerId
	cancel           context.CancelFunc
	changeStreamOpts *options.ChangeStreamOptionsBuilder
	resumeToken      bson.Raw
}

// New creates a MongoAdapter for the given namespace.
func (mb *MongoAdapterBuilder) New(nsp socket.Namespace) socket.Adapter {
	name := nsp.Name()
	mb.mu.Lock()
	opts := DefaultMongoAdapterOptions()
	opts.Assign(mb.Opts)
	if mb.uid == "" {
		mb.uid = opts.Uid()
		if mb.uid == "" {
			mb.uid = adapter.ServerId(adapter.RandomId())
		}
		if !utils.IsNil(mb.Opts) {
			mb.Opts.SetUid(mb.uid)
		}
		mb.changeStreamOpts = opts.ChangeStreamOptions()
	}
	opts.SetUid(mb.uid)
	var ctx context.Context
	if mb.cancel == nil {
		ctx, mb.cancel = context.WithCancel(mb.Mongo.Context())
	}
	adapterInstance := NewMongoAdapter(nsp, mb.Mongo, opts)
	mb.adapters.Store(name, adapterInstance)
	mb.mu.Unlock()
	stopContextClose := context.AfterFunc(mb.Mongo.Context(), adapterInstance.Close)

	adapterInstance.Cleanup(func() {
		stopContextClose()
		mb.mu.Lock()
		defer mb.mu.Unlock()

		if !mb.adapters.CompareAndDelete(name, adapterInstance) || mb.adapters.Len() != 0 {
			return
		}

		if mb.cancel != nil {
			mb.cancel()
		}
		mb.cancel = nil
	})
	if ctx != nil {
		go mb.initChangeStream(ctx)
	}

	return adapterInstance
}

func (mb *MongoAdapterBuilder) initChangeStream(ctx context.Context) {
	initialHeartbeatSent := false
	for {
		mongoLog.Debug("opening change stream")

		mb.mu.Lock()
		changeStreamOpts := mb.changeStreamOpts
		resumeToken := slices.Clone(mb.resumeToken)
		uid := mb.uid
		mb.mu.Unlock()

		watchOptions := options.ChangeStream()
		if changeStreamOpts != nil {
			watchOptions.Opts = append(watchOptions.Opts, changeStreamOpts.List()...)
		}
		if len(resumeToken) != 0 {
			watchOptions.SetStartAfter(nil).SetStartAtOperationTime(nil).SetResumeAfter(resumeToken)
		}

		changeStream, err := mb.Mongo.Collection().Watch(ctx, mongod.Pipeline{
			{{Key: "$match", Value: bson.D{
				{Key: "fullDocument.uid", Value: bson.D{{Key: "$ne", Value: uid}}},
			}}},
		}, watchOptions)
		if err == nil {
			// New stays non-blocking, so repeat the initial heartbeat once the cursor can receive replies.
			if !initialHeartbeatSent {
				var adapterInstances []MongoAdapter
				mb.mu.Lock()
				if ctx.Err() == nil {
					adapterInstances = mb.adapters.Values()
					initialHeartbeatSent = true
				}
				mb.mu.Unlock()
				for _, adapterInstance := range adapterInstances {
					adapterInstance.Publish(&ClusterMessage{Type: mongo.INITIAL_HEARTBEAT})
				}
			}

			var event changeStreamEvent
			for changeStream.Next(ctx) {
				event = changeStreamEvent{}
				if decodeErr := changeStream.Decode(&event); decodeErr != nil {
					if operationType, ok := changeStream.Current.Lookup("operationType").StringValueOK(); ok && operationType == "insert" {
						token := changeStream.ResumeToken()
						mb.mu.Lock()
						if ctx.Err() == nil && len(token) != 0 {
							mb.resumeToken = append(mb.resumeToken[:0], token...)
						}
						mb.mu.Unlock()
					}
					mongoLog.Debug("failed to decode change stream event: %s", decodeErr.Error())
					mb.Mongo.Emit("error", decodeErr)
					continue
				}
				if event.OperationType != "insert" {
					continue
				}

				token := changeStream.ResumeToken()
				mb.mu.Lock()
				active := ctx.Err() == nil
				if active && len(token) != 0 {
					mb.resumeToken = append(mb.resumeToken[:0], token...)
				}
				document := event.FullDocument
				var adapterInstance MongoAdapter
				var found bool
				if document != nil {
					adapterInstance, found = mb.adapters.Load(document.Nsp)
				}
				mb.mu.Unlock()
				if active && found {
					adapterInstance.OnEvent(document)
				}
			}
			err = changeStream.Err()
			closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			_ = changeStream.Close(closeCtx)
			cancel()
		}

		if ctx.Err() != nil {
			return
		}
		if err != nil {
			var serverError mongod.ServerError
			if errors.As(err, &serverError) && !serverError.HasErrorLabel("ResumableChangeStreamError") {
				mb.mu.Lock()
				if ctx.Err() == nil {
					mb.changeStreamOpts = nil
					mb.resumeToken = nil
				}
				mb.mu.Unlock()
			}
			mongoLog.Debug("change stream error: %s", err.Error())
			mb.Mongo.Emit("error", err)
		}

		timer := time.NewTimer(time.Second)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return
		}
	}
}
