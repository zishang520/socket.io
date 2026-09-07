package adapter

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongod "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestMongoChangeStreamResumesCachedToken(t *testing.T) {
	for _, startOption := range []string{"default", "startAfter", "startAtOperationTime"} {
		t.Run(startOption, func(t *testing.T) {
			collection := newRecoveryCollection(t, false)
			ctx := collection.Context()
			cursor, err := collection.Collection().Watch(ctx, mongod.Pipeline{})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cursor.Close(ctx) })
			insertRecoveryEvent(t, collection, "/", mongo.HEARTBEAT, nil)
			if !cursor.Next(ctx) {
				t.Fatalf("initial change stream did not receive the marker: %v", cursor.Err())
			}
			token := slices.Clone(cursor.ResumeToken())
			seconds, increment := cursor.Current.Lookup("clusterTime").Timestamp()
			if closeErr := cursor.Close(ctx); closeErr != nil {
				t.Fatal(closeErr)
			}

			watchOptions := options.ChangeStream()
			switch startOption {
			case "startAfter":
				watchOptions.SetStartAfter(token)
			case "startAtOperationTime":
				watchOptions.SetStartAtOperationTime(&bson.Timestamp{T: seconds, I: increment})
			}
			current := newRecoveryAdapter(t, collection, "/", false).(*mongoAdapter)
			builder := &MongoAdapterBuilder{
				Mongo:            collection,
				uid:              current.uid,
				changeStreamOpts: watchOptions,
				resumeToken:      token,
			}
			builder.adapters.Store("/", current)
			// This heartbeat must be replayed from the cached token, even though
			// it was published before the replacement stream opened.
			insertRecoveryEvent(t, collection, "/", mongo.HEARTBEAT, nil)
			errors := captureErrors(t, collection)
			streamCtx, cancel := context.WithCancel(ctx)
			done := make(chan struct{})
			go func() {
				defer close(done)
				builder.initChangeStream(streamCtx)
			}()
			t.Cleanup(func() {
				cancel()
				<-done
			})

			ticker := time.NewTicker(5 * time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case streamErr := <-errors:
					t.Fatalf("resuming change stream failed: %v", streamErr)
				case <-ticker.C:
					if _, received := current.nodesMap.Load("remote"); received {
						return
					}
				case <-ctx.Done():
					t.Fatal("resumed change stream did not replay the missed heartbeat")
				}
			}
		})
	}
}
