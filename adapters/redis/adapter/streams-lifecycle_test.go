package adapter

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestRedisStreamsParentCancellationReleasesResources(t *testing.T) {
	for _, alreadyCanceled := range []bool{false, true} {
		name := "after construction"
		if alreadyCanceled {
			name = "before construction"
		}
		t.Run(name, func(t *testing.T) {
			db := miniredis.RunT(t)
			raw := rds.NewClient(&rds.Options{Addr: db.Addr()})
			t.Cleanup(func() { _ = raw.Close() })
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if alreadyCanceled {
				cancel()
			}
			client := mustRedisClient(t, ctx, raw)
			server := socket.NewServer(nil, nil)
			first := NewRedisStreamsAdapter(socket.NewNamespace(server, "/first"), client, nil).(*redisStreamsAdapter)
			second := NewRedisStreamsAdapter(socket.NewNamespace(server, "/second"), client, nil).(*redisStreamsAdapter)
			t.Cleanup(first.Close)
			t.Cleanup(second.Close)
			cancel()

			deadline := time.Now().Add(time.Second)
			for {
				redisStreamsPollers.mu.Lock()
				_, pollerExists := redisStreamsPollers.groups[first.streamPoller.key]
				redisStreamsPollers.mu.Unlock()
				redisPubSubs.mu.Lock()
				_, pubSubExists := redisPubSubs.groups[redisPubSubCacheKey{server: server, client: client}]
				redisPubSubs.mu.Unlock()
				if !pollerExists && !pubSubExists {
					break
				}
				if time.Now().After(deadline) {
					t.Fatalf("canceled adapters retained resources: poller=%t, pubsub=%t", pollerExists, pubSubExists)
				}
				time.Sleep(time.Millisecond)
			}
			if first.streamPoller.adapters.Len() != 0 || second.streamPoller.adapters.Len() != 0 {
				t.Fatal("canceled pollers retained namespace adapters")
			}
		})
	}
}

func TestRedisStreamsConstructionErrorHandlerCanClose(t *testing.T) {
	raw := rds.NewClient(&rds.Options{
		MaxRetries: -1,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, net.ErrClosed
		},
	})
	t.Cleanup(func() { _ = raw.Close() })
	client := mustRedisClient(t, t.Context(), raw)
	current := MakeRedisStreamsAdapter().(*redisStreamsAdapter)
	current.SetRedis(client)
	t.Cleanup(current.Close)
	closed := make(chan struct{})
	if err := client.Once("error", func(...any) {
		current.Close()
		close(closed)
	}); err != nil {
		t.Fatal(err)
	}
	server := socket.NewServer(nil, nil)
	current.Construct(socket.NewNamespace(server, "/construction-error"))
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("construction error handler did not finish closing the adapter")
	}
	redisStreamsPollers.mu.Lock()
	_, pollerExists := redisStreamsPollers.groups[current.streamPoller.key]
	redisStreamsPollers.mu.Unlock()
	redisPubSubs.mu.Lock()
	_, pubSubExists := redisPubSubs.groups[redisPubSubCacheKey{server: server, client: client}]
	redisPubSubs.mu.Unlock()
	if pollerExists || pubSubExists || current.streamPoller.adapters.Len() != 0 {
		t.Fatalf("construction error handler retained resources: poller=%t, pubsub=%t, adapters=%d",
			pollerExists, pubSubExists, current.streamPoller.adapters.Len())
	}
}

func TestRedisStreamsRejectsReadOnlyPrimaryBeforeStarting(t *testing.T) {
	for name, options := range map[string]*rds.ClusterOptions{
		"read only": {ReadOnly: true}, "latency": {RouteByLatency: true}, "random": {RouteRandomly: true},
	} {
		t.Run(name, func(t *testing.T) {
			options.Addrs = []string{"127.0.0.1:7000"}
			raw := rds.NewClusterClient(options)
			t.Cleanup(func() { _ = raw.Close() })
			client := mustRedisClient(t, t.Context(), raw)
			var reported error
			if err := client.On("error", func(args ...any) { reported, _ = args[0].(error) }); err != nil {
				t.Fatal(err)
			}
			current := NewRedisStreamsAdapter(socket.NewNamespace(socket.NewServer(nil, nil), "/"), client, nil).(*redisStreamsAdapter)
			t.Cleanup(current.Close)
			if !errors.Is(reported, redis.ErrReadOnlyRedisClient) {
				t.Fatalf("configuration error = %v", reported)
			}
			if current.streamPoller != nil || current.pubSub != nil || current.shardedPubSub != nil {
				t.Fatal("invalid configuration started shared resources")
			}
			if _, err := current.PublishAndReturnOffset(&adapter.ClusterMessage{Type: adapter.HEARTBEAT}); !errors.Is(err, adapter.ErrAdapterClosed) {
				t.Fatalf("invalid adapter accepted a publish: %v", err)
			}
		})
	}
}
