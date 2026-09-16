package adapter

import (
	"bytes"
	"github.com/alicebob/miniredis/v2"
	miniserver "github.com/alicebob/miniredis/v2/server"
	"reflect"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestPreparedPublishFreezesRoutingAndPayload(t *testing.T) {
	db := miniredis.RunT(t)
	sent := make(chan []string, 8)
	db.Server().SetPreHook(func(peer *miniserver.Peer, command string, args ...string) bool {
		if command != "PUBLISH" && command != "SPUBLISH" {
			return false
		}
		sent <- append([]string{command}, args...)
		peer.WriteInt(0)
		return true
	})

	raw := newValkeyRawClient(t, db.Addr())
	client := newStreamsValkeyClient(t, raw, nil)
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/test")
	sharded := MakeShardedValkeyAdapter().(*shardedValkeyAdapter)
	sharded.SetValkey(client)
	sharded.ClusterAdapter.Construct(nsp)
	sharded.channel = "channel#"
	sharded.opts.SetSubscriptionMode(valkey.DynamicSubscriptionMode)
	t.Cleanup(sharded.ClusterAdapter.Close)
	streams := MakeValkeyStreamsAdapter().(*valkeyStreamsAdapter)
	streams.SetValkey(client)
	streams.ClusterAdapter.Construct(nsp)
	streams.streamName = "stream"
	streams.publicChannel = "events"
	streams.opts.SetMaxLen(100)
	streams.opts.SetChannelPrefix("prefix")
	t.Cleanup(streams.ClusterAdapter.Close)
	for _, test := range []struct {
		name, channel          string
		response, ack, durable bool
		current                adapter.ClusterAdapter
	}{
		{name: "sharded broadcast", channel: "channel#room#", current: sharded},
		{name: "sharded response", channel: "channel#requester#", response: true, current: sharded},
		{name: "streams broadcast ack", channel: "events", ack: true, current: streams},
		{name: "streams response", channel: "prefix#/test#requester#", response: true, current: streams},
		{name: "streams durable", durable: true, current: streams},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := []byte("before")
			message := &adapter.ClusterMessage{Uid: "sender", Nsp: "/test", Type: adapter.BROADCAST,
				Data: &adapter.BroadcastMessage{Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", value}}, Opts: &adapter.PacketOptions{Rooms: []socket.Room{"room"}, Except: []socket.Room{}}}}
			if test.ack {
				message.Data.(*adapter.BroadcastMessage).RequestId = new("request")
			}
			if test.response {
				message.Type = adapter.BROADCAST_ACK
				message.Data = &adapter.BroadcastAck{RequestId: "request", Packet: value}
			}
			var expected []byte
			var expectedStream valkey.RawClusterMessage
			var err error
			if test.durable {
				expectedStream, err = valkey.EncodeStreamMessage(message, false)
			} else if test.current == streams {
				expected, err = adapter.EncodeClusterMessageMsgpack(message)
			} else {
				expected, err = adapter.EncodeClusterMessage(message)
			}
			if err != nil {
				t.Fatal(err)
			}
			var publish adapter.PublishFunc
			if test.response {
				publish, err = test.current.PreparePublishResponse("requester", message)
			} else {
				publish, err = test.current.PreparePublish(message)
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(sent) != 0 || db.Exists("stream") {
				t.Fatal("preparation performed network publishing")
			}
			copy(value, "after!")
			message.Uid = "changed"
			if data, ok := message.Data.(*adapter.BroadcastMessage); ok {
				data.Opts.Rooms[0] = "other"
				data.RequestId = nil
			}
			offset, err := publish()
			if err != nil {
				t.Fatal(err)
			}
			if test.durable {
				entries, err := db.Stream("stream")
				if err != nil || len(entries) != 1 {
					t.Fatalf("stream entries=%v err=%v", entries, err)
				}
				if offset == "" || string(offset) != entries[0].ID {
					t.Fatalf("offset=%q entries=%v", offset, entries)
				}
				actual := valkey.RawClusterMessage{}
				for i := 0; i < len(entries[0].Values); i += 2 {
					actual[entries[0].Values[i]] = entries[0].Values[i+1]
				}
				if !reflect.DeepEqual(actual, expectedStream) {
					t.Fatalf("stream changed: got=%v want=%v", actual, expectedStream)
				}
			} else {
				actual := <-sent
				if offset != "" || actual[1] != test.channel || !bytes.Equal([]byte(actual[2]), expected) {
					t.Fatalf("publish changed: offset=%q channel=%q payload=%x", offset, actual[1], actual[2])
				}
			}
		})
	}
}
