package adapter

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func TestMongoPublishPreservesInsertionOrder(t *testing.T) {
	firstStarted := make(chan struct{})
	laterStarted := make(chan struct{}, 1)
	allowInsert := make(chan struct{})
	release := sync.OnceFunc(func() { close(allowInsert) })
	var first atomic.Bool
	monitor := &event.CommandMonitor{Started: func(ctx context.Context, command *event.CommandStartedEvent) {
		if command.CommandName != "insert" {
			return
		}
		if first.CompareAndSwap(false, true) {
			close(firstStarted)
			select {
			case <-allowInsert:
			case <-ctx.Done():
			}
			return
		}
		select {
		case laterStarted <- struct{}{}:
		default:
		}
	}}
	collection := newRecoveryCollection(t, false, options.Client().SetMonitor(monitor))
	current := newRecoveryAdapter(t, collection, "/", false).(*mongoAdapter)
	current.heartbeatInterval = time.Hour
	t.Cleanup(release)

	rooms := []socket.Room{"original"}
	current.Publish(&ClusterMessage{
		Type: mongo.SOCKETS_JOIN,
		Data: &SocketsJoinLeaveMessage{Opts: adapter.EncodeOptions(nil), Rooms: rooms},
	})
	rooms[0] = "changed"
	select {
	case <-firstStarted:
	case <-collection.Context().Done():
		t.Fatal("first insert did not start")
	}

	type publishResult struct {
		offset adapter.Offset
		err    error
	}
	published := make(chan publishResult, 1)
	go func() {
		offset, err := current.PublishAndReturnOffset(&ClusterMessage{
			Type: mongo.BROADCAST,
			Data: &BroadcastMessage{
				Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
				Opts:   adapter.EncodeOptions(nil),
			},
		})
		published <- publishResult{offset: offset, err: err}
	}()
	select {
	case <-laterStarted:
		t.Fatal("broadcast insert overtook the blocked join insert")
	case <-time.After(100 * time.Millisecond):
	}
	release()
	var result publishResult
	select {
	case result = <-published:
		if result.err != nil {
			t.Fatal(result.err)
		}
	case <-collection.Context().Done():
		t.Fatal("queued broadcast did not complete")
	}

	cursor, err := collection.Collection().Find(collection.Context(), bson.D{}, options.Find().SetSort(bson.D{{Key: "$natural", Value: 1}}))
	if err != nil {
		t.Fatal(err)
	}
	var documents []mongo.AdapterEvent
	if cursorErr := cursor.All(collection.Context(), &documents); cursorErr != nil {
		t.Fatal(cursorErr)
	}
	if len(documents) != 2 || documents[0].Type != mongo.SOCKETS_JOIN || documents[1].Type != mongo.BROADCAST {
		t.Fatalf("unexpected insertion order: %#v", documents)
	}
	if result.offset != adapter.Offset(documents[1].ID.Hex()) {
		t.Fatalf("offset = %q, want broadcast ID %q", result.offset, documents[1].ID.Hex())
	}
	data, err := mongo.UnmarshalAdapterData(mongo.SOCKETS_JOIN, documents[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	if got := data.(*SocketsJoinLeaveMessage).Rooms; len(got) != 1 || got[0] != "original" {
		t.Fatalf("queued publication did not preserve its payload: %v", got)
	}
}

func TestMongoCloseDrainsAcceptedPublications(t *testing.T) {
	firstStarted := make(chan struct{})
	allowInsert := make(chan struct{})
	release := sync.OnceFunc(func() { close(allowInsert) })
	completed := make(chan struct{}, 3)
	var first atomic.Bool
	monitor := &event.CommandMonitor{
		Started: func(ctx context.Context, command *event.CommandStartedEvent) {
			if command.CommandName == "insert" && first.CompareAndSwap(false, true) {
				close(firstStarted)
				select {
				case <-allowInsert:
				case <-ctx.Done():
				}
			}
		},
		Succeeded: func(_ context.Context, command *event.CommandSucceededEvent) {
			if command.CommandName == "insert" {
				completed <- struct{}{}
			}
		},
	}
	collection := newRecoveryCollection(t, false, options.Client().SetMonitor(monitor))
	current := newRecoveryAdapter(t, collection, "/", false).(*mongoAdapter)
	current.heartbeatInterval = time.Hour
	t.Cleanup(release)
	current.Publish(&ClusterMessage{Type: mongo.SOCKETS_JOIN})
	select {
	case <-firstStarted:
	case <-collection.Context().Done():
		t.Fatal("first insert did not start")
	}
	current.Publish(&ClusterMessage{Type: mongo.BROADCAST})
	closed := make(chan struct{})
	go func() {
		current.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close waited for a blocked publication")
	}

	rejected := make(chan error, 1)
	go func() {
		_, err := current.PublishAndReturnOffset(&ClusterMessage{Type: mongo.HEARTBEAT})
		rejected <- err
	}()
	select {
	case err := <-rejected:
		if !errors.Is(err, adapter.ErrAdapterClosed) {
			t.Fatalf("publication after Close returned %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("publication after Close blocked")
	}
	release()
	for range 2 {
		select {
		case <-completed:
		case <-collection.Context().Done():
			t.Fatal("Close discarded an accepted publication")
		}
	}
	if count, err := collection.Collection().CountDocuments(collection.Context(), bson.D{}); err != nil || count != 2 {
		t.Fatalf("stored publications = %d, error = %v; want 2", count, err)
	}
}
