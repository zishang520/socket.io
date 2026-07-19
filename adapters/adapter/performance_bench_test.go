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
		socketDetailsToResponses(details)
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

	message := &ClusterMessage{Type: BROADCAST}
	cluster.Publish(message)
	b.ReportAllocs()
	for b.Loop() {
		cluster.Publish(message)
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
