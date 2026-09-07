package adapter

import (
	"bytes"
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"go.mongodb.org/mongo-driver/v2/bson"
	mongod "go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func newRecoveryCollection(t *testing.T, capped bool, clientOpts ...*options.ClientOptions) *mongo.MongoClient {
	t.Helper()
	uri := os.Getenv("SOCKET_IO_MONGO_TEST_URI")
	if uri == "" {
		t.Skip("set SOCKET_IO_MONGO_TEST_URI to run MongoDB integration tests")
	}
	client, err := mongod.Connect(append([]*options.ClientOptions{options.Client().ApplyURI(uri)}, clientOpts...)...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if disconnectErr := client.Disconnect(ctx); disconnectErr != nil {
			t.Error(disconnectErr)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	database := client.Database("socket_io_recovery_test_" + bson.NewObjectID().Hex())
	createOptions := options.CreateCollection()
	if capped {
		createOptions.SetCapped(true).SetSizeInBytes(1 << 20)
	}
	if createErr := database.CreateCollection(ctx, "events", createOptions); createErr != nil {
		t.Fatal(createErr)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()
		if dropErr := database.Drop(cleanupCtx); dropErr != nil {
			t.Error(dropErr)
		}
	})
	collection, err := mongo.NewMongoClient(ctx, database.Collection("events"))
	if err != nil {
		t.Fatal(err)
	}
	return collection
}

func newRecoveryAdapter(t *testing.T, collection *mongo.MongoClient, nsp string, capped bool) MongoAdapter {
	t.Helper()
	opts := DefaultMongoAdapterOptions()
	opts.SetAddCreatedAtField(!capped)
	current := NewMongoAdapter(socket.NewServer(nil, nil).Of(nsp, nil), collection, opts)
	t.Cleanup(func() {
		current.Close()
		// Finish asynchronous writes before the test drops its collection.
		current.(*mongoAdapter).publisher.Close()
		current.(*mongoAdapter).responses.Close()
	})
	return current
}

func insertRecoveryEvent(t *testing.T, collection *mongo.MongoClient, nsp string, eventType mongo.EventType, data any) bson.ObjectID {
	t.Helper()
	id := bson.NewObjectID()
	_, err := collection.Collection().InsertOne(collection.Context(), bson.D{
		{Key: "_id", Value: id},
		{Key: "uid", Value: "remote"},
		{Key: "nsp", Value: nsp},
		{Key: "type", Value: eventType},
		{Key: "data", Value: data},
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func recoveryBroadcast(data ...any) bson.M {
	return bson.M{
		"packet": bson.M{"type": parser.EVENT, "data": data},
		"opts":   bson.M{"rooms": bson.A{}, "except": bson.A{}, "flags": bson.M{}},
	}
}

func recoverySession(sid string) bson.M {
	return bson.M{"sid": sid, "pid": "pid", "rooms": bson.A{"room"}, "data": bson.M{"owner": sid}}
}

func TestMongoRecoveryNamespaceIsolation(t *testing.T) {
	for _, capped := range []bool{false, true} {
		name := "ttl"
		if capped {
			name = "capped"
		}
		t.Run(name, func(t *testing.T) {
			collection := newRecoveryCollection(t, capped)
			public := newRecoveryAdapter(t, collection, "/public", capped)
			private := newRecoveryAdapter(t, collection, "/private", capped)
			publicOffset := insertRecoveryEvent(t, collection, "/public", mongo.BROADCAST, recoveryBroadcast("marker"))
			privateOffset := insertRecoveryEvent(t, collection, "/private", mongo.BROADCAST, recoveryBroadcast("marker"))
			insertRecoveryEvent(t, collection, "/private", mongo.SESSION, recoverySession("private-sid"))

			if _, err := public.RestoreSession("pid", publicOffset.Hex()); !errors.Is(err, errSessionOrOffsetNotFound) {
				t.Fatalf("foreign namespace session: %v", err)
			}
			insertRecoveryEvent(t, collection, "/public", mongo.SESSION, recoverySession("public-sid"))
			for _, offset := range []string{privateOffset.Hex(), bson.NewObjectID().Hex(), "invalid"} {
				if _, err := public.RestoreSession("pid", offset); err == nil {
					t.Fatalf("accepted invalid or foreign offset %q", offset)
				}
			}
			for _, test := range []struct {
				current MongoAdapter
				offset  bson.ObjectID
				sid     socket.SocketId
			}{
				{public, publicOffset, "public-sid"},
				{private, privateOffset, "private-sid"},
			} {
				restored, err := test.current.RestoreSession("pid", test.offset.Hex())
				if err != nil {
					t.Fatalf("restore %s after rejected attempts: %v", test.sid, err)
				}
				if restored.Sid != test.sid || restored.Data.(map[string]any)["owner"] != string(test.sid) {
					t.Fatalf("restored wrong namespace session: %#v", restored.SessionToPersist)
				}
			}
		})
	}
}

func TestMongoRecoveryReplaysRecoverablePacketsWithOffsets(t *testing.T) {
	collection := newRecoveryCollection(t, false)
	current := newRecoveryAdapter(t, collection, "/", false)
	offset := insertRecoveryEvent(t, collection, "/", mongo.BROADCAST, recoveryBroadcast("marker"))
	insertRecoveryEvent(t, collection, "/", mongo.SESSION, recoverySession("sid"))
	first := insertRecoveryEvent(t, collection, "/", mongo.BROADCAST, recoveryBroadcast("event", "business-value"))
	binaryData := []byte{0, 1, 2, 255}
	second := insertRecoveryEvent(t, collection, "/", mongo.BROADCAST, recoveryBroadcast("binary", bson.Binary{Data: binaryData}))

	volatile := recoveryBroadcast("volatile")
	volatile["opts"].(bson.M)["flags"] = bson.M{"volatile": true}
	withAck := recoveryBroadcast("ack")
	withAck["requestId"] = "request"
	withPacketID := recoveryBroadcast("packet-id")
	withPacketID["packet"].(bson.M)["id"] = 1
	nonEvent := recoveryBroadcast("non-event")
	nonEvent["packet"].(bson.M)["type"] = parser.ACK
	otherRoom := recoveryBroadcast("other-room")
	otherRoom["opts"].(bson.M)["rooms"] = bson.A{"other"}
	excluded := recoveryBroadcast("excluded")
	excluded["opts"].(bson.M)["except"] = bson.A{"room"}
	for _, data := range []bson.M{volatile, withAck, withPacketID, nonEvent, otherRoom, excluded} {
		insertRecoveryEvent(t, collection, "/", mongo.BROADCAST, data)
	}
	insertRecoveryEvent(t, collection, "/other", mongo.BROADCAST, recoveryBroadcast("other-namespace"))

	restored, err := current.RestoreSession("pid", offset.Hex())
	if err != nil {
		t.Fatal(err)
	}
	want := []any{
		[]any{"event", "business-value", first.Hex()},
		[]any{"binary", binaryData, second.Hex()},
	}
	if !reflect.DeepEqual(restored.MissedPackets, want) {
		t.Fatalf("missed packets = %#v, want %#v", restored.MissedPackets, want)
	}
	encoded := parser.NewEncoder().Encode(&parser.Packet{Type: parser.EVENT, Data: restored.MissedPackets[1]})
	if len(encoded) != 2 || !bytes.Equal(encoded[1].Bytes(), binaryData) {
		t.Fatalf("restored binary packet did not produce a binary attachment: %#v", encoded)
	}

	lastPacket := restored.MissedPackets[len(restored.MissedPackets)-1].([]any)
	lastOffset := lastPacket[len(lastPacket)-1].(string)
	insertRecoveryEvent(t, collection, "/", mongo.SESSION, recoverySession("sid"))
	latest := insertRecoveryEvent(t, collection, "/", mongo.BROADCAST, recoveryBroadcast("latest"))
	restoredAgain, err := current.RestoreSession("pid", lastOffset)
	if err != nil {
		t.Fatal(err)
	}
	if want := []any{[]any{"latest", latest.Hex()}}; !reflect.DeepEqual(restoredAgain.MissedPackets, want) {
		t.Fatalf("second recovery = %#v, want %#v", restoredAgain.MissedPackets, want)
	}
}
