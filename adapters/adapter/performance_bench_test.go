package adapter

import (
	"testing"
	"time"

	"github.com/zishang520/socket.io/servers/socket/v3"
)

func BenchmarkDecodeOptions(b *testing.B) {
	opts := &PacketOptions{
		Rooms:  []socket.Room{"room1", "room2", "room3"},
		Except: []socket.Room{"room4", "room5"},
		Flags:  &socket.BroadcastFlags{},
	}

	b.ReportAllocs()
	for b.Loop() {
		DecodeOptions(opts)
	}
}

func BenchmarkSocketDetailsToResponses(b *testing.B) {
	details := make([]socket.SocketDetails, 100)
	for i := range details {
		details[i] = NewRemoteSocket(&SocketResponse{
			Id:    socket.SocketId("socket"),
			Rooms: []socket.Room{"room1", "room2"},
		})
	}

	b.ReportAllocs()
	for b.Loop() {
		SocketDetailsToResponses(details)
	}
}

func BenchmarkHeartbeatPublish(b *testing.B) {
	opts := DefaultClusterAdapterOptions()
	opts.SetHeartbeatInterval(time.Hour)

	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/bench")
	cluster := NewClusterAdapterWithHeartbeat(nsp, opts).(*clusterAdapterWithHeartbeat)
	cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
	cluster.cleanupTimer.Load().Stop()
	b.Cleanup(cluster.Close)

	message := newTestBroadcastClusterMessage()
	cluster.Publish(message)
	b.ReportAllocs()
	for b.Loop() {
		cluster.Publish(message)
	}
}

func BenchmarkClusterPublishAndReturnOffset(b *testing.B) {
	nsp := socket.NewNamespace(socket.NewServer(nil, nil), "/bench")
	cluster := NewClusterAdapter(nsp)
	cluster.Prototype(&testClusterAdapter{ClusterAdapter: cluster})
	b.Cleanup(cluster.Close)
	message := &ClusterMessage{Type: HEARTBEAT}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := cluster.PublishAndReturnOffset(message); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkClusterAdapterIgnoreSelf(b *testing.B) {
	adapter := MakeClusterAdapter().(*clusterAdapter)
	adapter.uid = "node"
	message := &ClusterMessage{Uid: "node"}

	b.ReportAllocs()
	for b.Loop() {
		adapter.OnMessage(message, "")
	}
}

func BenchmarkClusterMessageEncoding(b *testing.B) {
	opts := &PacketOptions{Rooms: []socket.Room{"room"}, Except: []socket.Room{}, Flags: &socket.BroadcastFlags{}}
	for _, test := range []struct {
		name string
		kind MessageType
		data any
	}{
		{"native", SERVER_SIDE_EMIT_RESPONSE, &ServerSideEmitResponse{RequestId: "request", Packet: map[string]any{"count": int64(9007199254740993), "values": []any{"hello", true}}}},
		{"custom codec", SERVER_SIDE_EMIT_RESPONSE, &ServerSideEmitResponse{RequestId: "request", Packet: &snapshotTaggedValue{Value: "hello"}}},
		{"server emit", SERVER_SIDE_EMIT, &ServerSideEmitMessage{Packet: []any{"event", "hello"}}},
		{"broadcast ack", BROADCAST_ACK, &BroadcastAck{RequestId: "request", Packet: "hello"}},
		{"join", SOCKETS_JOIN, &SocketsJoinLeaveMessage{Opts: opts, Rooms: []socket.Room{"target"}}},
		{"fetch", FETCH_SOCKETS, &FetchSocketsMessage{Opts: opts, RequestId: "request"}},
	} {
		b.Run(test.name, func(b *testing.B) {
			message := &ClusterMessage{Type: test.kind, Data: test.data}
			b.ReportAllocs()
			for b.Loop() {
				if _, err := EncodeClusterMessage(message); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
