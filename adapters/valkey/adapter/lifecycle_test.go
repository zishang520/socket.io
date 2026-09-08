package adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	miniredisserver "github.com/alicebob/miniredis/v2/server"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func newValkeyLifecycleServer(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	db := miniredis.RunT(t)
	for _, command := range []string{"SSUBSCRIBE", "SUNSUBSCRIBE"} {
		if err := db.Server().Register(command, func(peer *miniredisserver.Peer, receivedCommand string, channels []string) {
			count := 0
			if receivedCommand == "SSUBSCRIBE" {
				count = 1
			}
			for _, channel := range channels {
				peer.Block(func(writer *miniredisserver.Writer) {
					writer.WriteLen(3)
					writer.WriteBulk(strings.ToLower(receivedCommand))
					writer.WriteBulk(channel)
					writer.WriteInt(count)
				})
			}
			peer.Flush()
		}); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestValkeyAdaptersCloseWithParentContext(t *testing.T) {
	for _, kind := range []string{"classic", "sharded", "streams"} {
		for _, alreadyCanceled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/alreadyCanceled=%t", kind, alreadyCanceled), func(t *testing.T) {
				db := newValkeyLifecycleServer(t)
				raw := newValkeyRawClient(t, db.Addr())
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				client, err := valkey.NewValkeyClient(ctx, raw)
				if err != nil {
					t.Fatal(err)
				}
				if alreadyCanceled {
					cancel()
				}
				nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/lifecycle")
				var current socket.Adapter
				switch kind {
				case "classic":
					current = NewValkeyAdapter(nsp, client, nil)
				case "sharded":
					current = NewShardedValkeyAdapter(nsp, client, nil)
					if !alreadyCanceled {
						current.AddAll("socket", types.NewSet(socket.Room("room")))
					}
				case "streams":
					current = NewValkeyStreamsAdapter(nsp, client, nil)
				}
				t.Cleanup(current.Close)
				cancel()
				released := func() bool {
					switch a := current.(type) {
					case *valkeyAdapter:
						classicValkeyPubSubs.mu.Lock()
						_, registered := classicValkeyPubSubs.groups[client]
						classicValkeyPubSubs.mu.Unlock()
						return !registered && a.publisher.IsShuttingDown()
					case *shardedValkeyAdapter:
						a.dynamicMu.Lock()
						dynamicCount := len(a.dynamicPubSubs)
						a.dynamicMu.Unlock()
						_, publishErr := a.PublishAndReturnOffset(&adapter.ClusterMessage{Type: adapter.HEARTBEAT})
						return dynamicCount == 0 && errors.Is(publishErr, adapter.ErrAdapterClosed)
					case *valkeyStreamsAdapter:
						valkeyStreamsPollers.mu.Lock()
						_, registered := valkeyStreamsPollers.groups[a.streamPoller.key]
						valkeyStreamsPollers.mu.Unlock()
						_, publishErr := a.PublishAndReturnOffset(&adapter.ClusterMessage{Type: adapter.HEARTBEAT})
						return !registered && a.streamPoller.adapters.Len() == 0 && errors.Is(publishErr, adapter.ErrAdapterClosed)
					}
					return false
				}
				deadline := time.Now().Add(time.Second)
				for !released() {
					if alreadyCanceled || time.Now().After(deadline) {
						t.Fatal("parent cancellation did not release adapter resources")
					}
					time.Sleep(time.Millisecond)
				}
			})
		}
	}
}

func TestValkeyConstructionErrorHandlerCanClose(t *testing.T) {
	for _, kind := range []string{"sharded", "streams"} {
		t.Run(kind, func(t *testing.T) {
			db := newValkeyLifecycleServer(t)
			client := newValkeyAdapterTestClient(t, db.Addr())
			var current socket.Adapter
			if kind == "sharded" {
				sharded := MakeShardedValkeyAdapter()
				sharded.SetValkey(client)
				current = sharded
			} else {
				streams := MakeValkeyStreamsAdapter()
				streams.SetValkey(client)
				current = streams
			}
			t.Cleanup(current.Close)
			closed := make(chan struct{})
			if err := client.Once("error", func(...any) {
				current.Close()
				close(closed)
			}); err != nil {
				t.Fatal(err)
			}
			db.SetError("ERR injected subscription failure")
			current.Construct(socket.NewNamespace(socket.NewServer(nil, nil), "/construction-error"))
			select {
			case <-closed:
			case <-time.After(3 * time.Second):
				t.Fatal("construction error handler did not finish Close")
			}
			cluster := current.(adapter.ClusterAdapter)
			if _, err := cluster.PublishAndReturnOffset(&adapter.ClusterMessage{Type: adapter.HEARTBEAT}); !errors.Is(err, adapter.ErrAdapterClosed) {
				t.Fatalf("closed adapter accepted a publish: %v", err)
			}
			if streams, ok := current.(*valkeyStreamsAdapter); ok {
				valkeyStreamsPollers.mu.Lock()
				_, registered := valkeyStreamsPollers.groups[streams.streamPoller.key]
				valkeyStreamsPollers.mu.Unlock()
				if registered || streams.streamPoller.adapters.Len() != 0 {
					t.Fatal("construction error handler retained the stream poller")
				}
			}
		})
	}
}
