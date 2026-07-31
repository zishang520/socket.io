package adapter

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	rds "github.com/redis/go-redis/v9"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// newShardedTestRedisClient returns a RedisClient wired to the local test
// Redis (same instance the emitter tests use), skipping the test when it is
// not reachable or does not support sharded Pub/Sub (Redis < 7.0).
func newShardedTestRedisClient(t *testing.T) *redis.RedisClient {
	t.Helper()

	client := rds.NewClient(&rds.Options{
		Addr:     "localhost:6379",
		Password: "root",
	})
	if err := client.Ping(context.Background()).Err(); err != nil {
		client.Close()
		t.Skipf("redis is not available at localhost:6379: %v", err)
	}
	if err := client.SPublish(context.Background(), "sharded-adapter-probe", "probe").Err(); err != nil {
		client.Close()
		t.Skipf("redis at localhost:6379 does not support sharded Pub/Sub: %v", err)
	}

	t.Cleanup(func() { client.Close() })
	return redis.NewRedisClient(context.Background(), client)
}

// newShardedTestAdapter builds a namespace adapter backed by the sharded
// Redis adapter, using a per-test channel prefix to isolate concurrent runs.
func newShardedTestAdapter(t *testing.T, redisClient *redis.RedisClient, prefix string) socket.Adapter {
	t.Helper()

	opts := DefaultShardedRedisAdapterOptions()
	opts.SetChannelPrefix(prefix)

	io := socket.NewServer(nil, nil)
	io.SetAdapter(&ShardedRedisAdapterBuilder{Redis: redisClient, Opts: opts})
	return io.Of("/", nil).Adapter()
}

// waitFor polls cond every 50ms until it returns true or the timeout expires.
func waitFor(timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}

func shardedChannels(t *testing.T, c rds.UniversalClient, pattern string) []string {
	t.Helper()
	channels, err := c.PubSubShardChannels(context.Background(), pattern).Result()
	if err != nil {
		t.Fatalf("PubSubShardChannels(%q): %v", pattern, err)
	}
	return channels
}

func shardedNumSub(t *testing.T, c rds.UniversalClient, channel string) int64 {
	t.Helper()
	counts, err := c.PubSubShardNumSub(context.Background(), channel).Result()
	if err != nil {
		t.Fatalf("PubSubShardNumSub(%q): %v", channel, err)
	}
	return counts[channel]
}

// TestShardedRedisAdapterStandalone exercises the desired-state subscription
// reconciler against a real Redis instance: static channel setup, dynamic
// room subscribe/unsubscribe, cross-instance publishing, connection sharing,
// and convergence under create/delete churn.
func TestShardedRedisAdapterStandalone(t *testing.T) {
	redisClient1 := newShardedTestRedisClient(t)
	redisClient2 := newShardedTestRedisClient(t)
	probe := newShardedTestRedisClient(t)

	prefix := fmt.Sprintf("sio-test-%d", time.Now().UnixNano())
	mainChannel := prefix + "#/#"
	dynamicChannel := func(room string) string { return prefix + "#/#" + room + "#" }

	adapter1 := newShardedTestAdapter(t, redisClient1, prefix)
	adapter2 := newShardedTestAdapter(t, redisClient2, prefix)

	t.Run("StaticChannels", func(t *testing.T) {
		if !waitFor(5*time.Second, func() bool {
			return shardedNumSub(t, probe.Client, mainChannel) == 2
		}) {
			t.Fatalf("expected 2 subscribers on %q, got %d", mainChannel, shardedNumSub(t, probe.Client, mainChannel))
		}
		if count := adapter1.ServerCount(); count != 2 {
			t.Fatalf("ServerCount() = %d, want 2", count)
		}
	})

	sid := socket.SocketId("aaaaaaaaaaaaaaaaaaaa") // 20 chars: a private (socket id) room

	t.Run("DynamicRoomSubscription", func(t *testing.T) {
		adapter1.AddAll(sid, types.NewSet(socket.Room(sid), socket.Room("test-room")))

		if !waitFor(5*time.Second, func() bool {
			return shardedNumSub(t, probe.Client, dynamicChannel("test-room")) == 1
		}) {
			t.Fatalf("dynamic channel %q was not subscribed", dynamicChannel("test-room"))
		}
		if n := shardedNumSub(t, probe.Client, dynamicChannel(string(sid))); n != 0 {
			t.Fatalf("private room %q must not get a dynamic channel, got %d subscribers", sid, n)
		}
	})

	t.Run("CrossInstanceBroadcast", func(t *testing.T) {
		pubSub := probe.Client.SSubscribe(context.Background(), dynamicChannel("test-room"))
		defer pubSub.Close()
		time.Sleep(100 * time.Millisecond)

		adapter2.Broadcast(&parser.Packet{
			Type: parser.EVENT,
			Data: []any{"hello", "world"},
		}, &socket.BroadcastOptions{
			Rooms: types.NewSet(socket.Room("test-room")),
		})

		select {
		case <-pubSub.Channel():
		case <-time.After(3 * time.Second):
			t.Fatal("broadcast to a single room was not published on its dynamic channel")
		}
	})

	t.Run("ConnectionSharing", func(t *testing.T) {
		rooms := types.NewSet[socket.Room]()
		for i := range 20 {
			rooms.Add(socket.Room(fmt.Sprintf("bulk-room-%02d", i)))
		}
		adapter1.AddAll(sid, rooms)

		if !waitFor(5*time.Second, func() bool {
			return len(shardedChannels(t, probe.Client, prefix+"#/#bulk-room-*")) == 20
		}) {
			t.Fatalf("expected 20 dynamic channels, got %d", len(shardedChannels(t, probe.Client, prefix+"#/#bulk-room-*")))
		}
	})

	t.Run("ChurnConvergence", func(t *testing.T) {
		var wg sync.WaitGroup
		for g := range 8 {
			wg.Add(1)
			go func(g int) {
				defer wg.Done()
				churnSid := socket.SocketId(fmt.Sprintf("churn-sid-%02d-aaaaaa", g))
				for i := range 20 {
					room := socket.Room(fmt.Sprintf("churn-room-%d", i%4))
					adapter1.AddAll(churnSid, types.NewSet(room))
					adapter2.AddAll(churnSid, types.NewSet(room))
					adapter1.DelAll(churnSid)
					adapter2.DelAll(churnSid)
				}
			}(g)
		}
		wg.Wait()

		if !waitFor(10*time.Second, func() bool {
			return len(shardedChannels(t, probe.Client, prefix+"#/#churn-room-*")) == 0
		}) {
			t.Fatalf("leaked churn subscriptions: %v", shardedChannels(t, probe.Client, prefix+"#/#churn-room-*"))
		}
	})

	t.Run("UnsubscribeOnDelete", func(t *testing.T) {
		adapter1.DelAll(sid)

		if !waitFor(10*time.Second, func() bool {
			return len(shardedChannels(t, probe.Client, prefix+"#/#test-room#")) == 0 &&
				len(shardedChannels(t, probe.Client, prefix+"#/#bulk-room-*")) == 0
		}) {
			t.Fatal("dynamic channels were not unsubscribed after the rooms were deleted")
		}
	})

	t.Run("CloseReleasesEverything", func(t *testing.T) {
		adapter1.Close()
		adapter2.Close()

		if !waitFor(5*time.Second, func() bool {
			return len(shardedChannels(t, probe.Client, prefix+"#*")) == 0
		}) {
			t.Fatalf("channels still subscribed after Close: %v", shardedChannels(t, probe.Client, prefix+"#*"))
		}
	})
}
