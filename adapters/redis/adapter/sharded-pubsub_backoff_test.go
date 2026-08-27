package adapter

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	miniredisserver "github.com/alicebob/miniredis/v2/server"
	rds "github.com/redis/go-redis/v9"
)

func TestShardedPubSubBacksOffPersistentReceiveErrors(t *testing.T) {
	tests := []struct {
		name        string
		reply       string
		minAttempts int64
		maxAttempts int64
	}{
		{name: "NOPERM", reply: "NOPERM shard channel is not allowed", minAttempts: 3, maxAttempts: 6},
		{name: "ASK", reply: "ASK 1234 127.0.0.1:6379", minAttempts: 3, maxAttempts: 6},
		{name: "connection close", minAttempts: 2, maxAttempts: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := miniredis.RunT(t)
			var attempts atomic.Int64
			if err := server.Server().Register("SSUBSCRIBE", func(peer *miniredisserver.Peer, _ string, channels []string) {
				attempts.Add(1)
				if tt.reply == "" {
					peer.Block(func(writer *miniredisserver.Writer) {
						writer.WritePushLen(3)
						writer.WriteBulk("ssubscribe")
						writer.WriteBulk(channels[0])
						writer.WriteInt(1)
					})
					peer.Flush()
					peer.Close()
					return
				}
				peer.WriteError(tt.reply)
			}); err != nil {
				t.Fatal(err)
			}

			client := rds.NewClient(&rds.Options{Addr: server.Addr()})
			t.Cleanup(func() { _ = client.Close() })
			pubSub := newShardedPubSub(context.Background(), client, nil)
			t.Cleanup(pubSub.Close)
			pubSub.newSubscription(func([]byte, string) {}).Subscribe("socket.io#/#backoff#")
			if err := pubSub.flush(context.Background()); err != nil {
				t.Fatal(err)
			}

			waitForBackoffCondition(t, time.Second, func() bool { return attempts.Load() >= 2 })
			time.Sleep(650 * time.Millisecond)
			if got := attempts.Load(); got < tt.minAttempts || got > tt.maxAttempts {
				t.Fatalf("SSUBSCRIBE attempts after persistent %s = %d, want %d..%d", tt.name, got, tt.minAttempts, tt.maxAttempts)
			}
			if tt.reply == "" {
				waitForBackoffCondition(t, time.Second, func() bool { return attempts.Load() >= 3 })
			}

			closed := make(chan struct{})
			go func() {
				pubSub.Close()
				close(closed)
			}()
			select {
			case <-closed:
			case <-time.After(time.Second):
				t.Fatal("Close waited for the retry backoff")
			}
		})
	}
}

func TestShardedPubSubReceiveBackoffIsSharedAcrossPools(t *testing.T) {
	current := &shardedPubSub{}
	now := time.Now()

	// These calls represent errors from two different owner pools. Only the
	// first error in the manager-wide burst may rebuild immediately.
	if delay := current.receiveBackoff(now); delay != 0 {
		t.Fatalf("first receive failure delay = %s, want immediate retry", delay)
	}
	for failures := uint8(2); failures <= shardedPubSubMaxFailureCount+2; failures++ {
		now = now.Add(time.Millisecond)
		delay := current.receiveBackoff(now)
		min, max := shardedBackoffBounds(failures)
		if delay < min || delay > max {
			t.Fatalf("failure %d delay = %s, want %s..%s", failures, delay, min, max)
		}
	}

	now = now.Add(shardedPubSubStableAfter)
	if delay := current.receiveBackoff(now); delay != 0 {
		t.Fatalf("delay after a stable connection = %s, want immediate retry", delay)
	}
}

func shardedBackoffBounds(failures uint8) (time.Duration, time.Duration) {
	if failures > shardedPubSubMaxFailureCount {
		failures = shardedPubSubMaxFailureCount
	}
	delay := shardedPubSubBackoffBase
	for attempt := uint8(2); attempt < failures && delay < shardedPubSubBackoffMax; attempt++ {
		delay *= 2
	}
	if delay > shardedPubSubBackoffMax {
		delay = shardedPubSubBackoffMax
	}
	return delay - delay/4, delay
}

func waitForBackoffCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for !condition() {
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatal("timed out waiting for sharded Pub/Sub retry")
		}
	}
}
