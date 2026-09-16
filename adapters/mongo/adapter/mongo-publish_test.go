package adapter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	base "github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	mongod "go.mongodb.org/mongo-driver/v2/mongo"
)

func TestMongoPublishErrorHandlerCanPublishSynchronously(t *testing.T) {
	current, cancel := newMongoPublishTestAdapter(t)
	returned := make(chan error, 1)
	if listenerErr := current.mongoCollection.On("error", func(args ...any) {
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

func TestMongoPublishAndCloseDrainsAcceptedResponses(t *testing.T) {
	for _, failPreparation := range []bool{false, true} {
		name := "send error"
		if failPreparation {
			name = "preparation error"
		}
		t.Run(name, func(t *testing.T) {
			current, cancel := newMongoPublishTestAdapter(t)
			started, release := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			t.Cleanup(unblock)
			current.responses.Enqueue(func() { close(started); <-release })
			<-started
			cleaned := make(chan struct{})
			current.Cleanup(func() { close(cleaned) })
			failure := make(chan error, 1)
			if err := current.mongoCollection.On("error", func(args ...any) {
				publishErr, _ := args[0].(error)
				failure <- publishErr
			}); err != nil {
				t.Fatal(err)
			}
			cancel()
			message := &ClusterMessage{Type: mongo.HEARTBEAT}
			if failPreparation {
				message.Type = mongo.SERVER_SIDE_EMIT
				message.Data = &ServerSideEmitMessage{Packet: []any{"event", make(chan int)}}
			}
			returned := make(chan struct{})
			go func() { current.PublishAndClose(message); close(returned) }()
			select {
			case <-returned:
			case <-time.After(time.Second):
				t.Fatal("PublishAndClose waited for a pending response")
			}
			current.Close() // A second close must not release resources ahead of the final task.
			if _, err := current.PublishAndReturnOffset(message); !errors.Is(err, base.ErrAdapterClosed) {
				t.Fatalf("publish after closing = %v", err)
			}
			select {
			case <-cleaned:
				t.Fatal("cleanup overtook an accepted response")
			case <-failure:
				t.Fatal("final publication overtook an accepted response")
			case <-time.After(20 * time.Millisecond):
			}
			unblock()
			select {
			case err := <-failure:
				if err == nil {
					t.Fatal("final publication unexpectedly succeeded")
				}
				if failPreparation && errors.Is(err, context.Canceled) {
					t.Fatal("invalid payload was not rejected during preparation")
				}
				if !failPreparation && !errors.Is(err, context.Canceled) {
					t.Fatalf("final send error = %v, want context.Canceled", err)
				}
			case <-time.After(time.Second):
				t.Fatal("final publication did not report its error")
			}
			select {
			case <-cleaned:
			case <-time.After(time.Second):
				t.Fatal("final publication did not release resources")
			}
		})
	}
}

// Measures the canceled send path without requiring a database server.
func BenchmarkMongoPublishAndReturnOffsetCanceled(b *testing.B) {
	current, cancel := newMongoPublishTestAdapter(b)
	cancel()
	message := &ClusterMessage{Type: mongo.HEARTBEAT}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := current.PublishAndReturnOffset(message); !errors.Is(err, context.Canceled) {
			b.Fatal(err)
		}
	}
}

func newMongoPublishTestAdapter(t testing.TB) (*mongoAdapter, context.CancelFunc) {
	t.Helper()
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
	t.Cleanup(cancel)
	client, err := mongo.NewMongoClient(ctx, driverClient.Database("socket_io_publish_test").Collection("events"))
	if err != nil {
		t.Fatal(err)
	}
	opts := DefaultMongoAdapterOptions()
	opts.SetHeartbeatInterval(time.Hour)
	current := NewMongoAdapter(socket.NewServer(nil, nil).Sockets(), client, opts).(*mongoAdapter)
	t.Cleanup(current.Close)
	return current, cancel
}
