package adapter

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	mongod "go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoPublishErrorHandlerCanPublishSynchronously(t *testing.T) {
	driverClient, err := mongod.Connect()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if disconnectErr := driverClient.Disconnect(ctx); disconnectErr != nil {
			t.Error(disconnectErr)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := mongo.NewMongoClient(ctx, driverClient.Database("socket_io_publish_test").Collection("events"))
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultMongoAdapterOptions()
	opts.SetHeartbeatInterval(time.Hour)
	current := NewMongoAdapter(socket.NewServer(nil, nil).Sockets(), client, opts)
	t.Cleanup(current.Close)
	returned := make(chan error, 1)
	if listenerErr := client.On("error", func(args ...any) {
		if publishErr, ok := args[0].(error); !ok || !errors.Is(publishErr, context.Canceled) {
			returned <- errors.New("unexpected asynchronous publication error")
			return
		}
		_, publishErr := current.PublishAndReturnOffset(&ClusterMessage{Type: mongo.HEARTBEAT})
		returned <- publishErr
	}); listenerErr != nil {
		t.Fatal(listenerErr)
	}
	// Cancellation makes both driver operations fail without requiring a server.
	cancel()
	current.Publish(&ClusterMessage{Type: mongo.HEARTBEAT})
	select {
	case publishErr := <-returned:
		if !errors.Is(publishErr, context.Canceled) {
			t.Fatalf("reentrant publication returned %v", publishErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("error handler blocked while publishing synchronously")
	}
}
