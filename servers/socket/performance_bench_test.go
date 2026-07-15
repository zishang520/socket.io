package socket

import (
	"strconv"
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func BenchmarkNewBroadcastOperator(b *testing.B) {
	rooms := types.NewSet[Room]("room")
	except := types.NewSet[Room]("except")
	flags := &BroadcastFlags{}

	b.ReportAllocs()
	for b.Loop() {
		op := NewBroadcastOperator(nil, rooms, except, flags)
		if op.rooms != rooms || op.exceptRooms != except || op.flags != flags {
			b.Fatal("broadcast operator did not preserve its inputs")
		}
	}
}

func BenchmarkBroadcastOperatorTo(b *testing.B) {
	op := NewBroadcastOperator(nil, nil, nil, nil)

	b.ReportAllocs()
	for b.Loop() {
		result := op.To("room")
		if !result.rooms.Has("room") {
			b.Fatal("target room is missing")
		}
	}
}

func BenchmarkAdapterAddAllExisting(b *testing.B) {
	adapter := newTestAdapter()
	rooms := types.NewSet[Room]("room")
	adapter.AddAll("socket", rooms)

	b.ReportAllocs()
	for b.Loop() {
		adapter.AddAll("socket", rooms)
	}
}

func BenchmarkAdapterApplyNoFilters(b *testing.B) {
	adapter := newTestAdapter().(*adapter)
	socket := &Socket{id: "socket"}
	socket.connected.Store(true)
	adapter.nsp.Sockets().Store(socket.id, socket)
	adapter.AddAll(socket.id, types.NewSet[Room]("room"))
	callback := func(*Socket) {}

	b.ReportAllocs()
	for b.Loop() {
		adapter.apply(nil, callback)
	}
}

func BenchmarkAdapterApplySingleRoom(b *testing.B) {
	adapter := newTestAdapter().(*adapter)
	socket := &Socket{id: "socket"}
	socket.connected.Store(true)
	adapter.nsp.Sockets().Store(socket.id, socket)
	adapter.AddAll(socket.id, types.NewSet[Room]("room"))
	opts := &BroadcastOptions{
		Rooms:  types.NewSet[Room]("room"),
		Except: types.NewSet[Room](),
	}
	callback := func(*Socket) {}

	b.ReportAllocs()
	for b.Loop() {
		adapter.apply(opts, callback)
	}
}

func BenchmarkAdapterEncodeEvent(b *testing.B) {
	adapter := newTestAdapter().(*adapter)
	packet := &parser.Packet{
		Type: parser.EVENT,
		Nsp:  "/",
		Data: []any{"event", "payload"},
	}
	packetOpts := &WriteOptions{}

	b.ReportAllocs()
	for b.Loop() {
		if encoded := adapter._encode(packet, packetOpts); len(encoded) != 1 {
			b.Fatalf("encoded packets = %d, want 1", len(encoded))
		}
	}
}

func BenchmarkBroadcastEmitNoRecipients(b *testing.B) {
	operator := NewBroadcastOperator(newTestAdapter(), nil, nil, nil)

	b.ReportAllocs()
	for b.Loop() {
		if err := operator.Emit("event", "payload"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSocketJoinExistingRoom(b *testing.B) {
	socket := &Socket{id: "socket", adapter: newTestAdapter()}
	socket.canJoin.Store(true)
	socket.Join("room")

	b.ReportAllocs()
	for b.Loop() {
		socket.Join("room")
	}
}

func BenchmarkSocketNewBroadcastOperator(b *testing.B) {
	socket := MakeSocket()
	b.Cleanup(socket.taskQueue.Close)
	socket.id = "socket"
	socket.adapter = newTestAdapter()

	b.ReportAllocs()
	for b.Loop() {
		op := socket.newBroadcastOperator()
		if !op.exceptRooms.Has(Room(socket.id)) {
			b.Fatal("sender is not excluded")
		}
	}
}

func BenchmarkSessionAwareAdapterRestore(b *testing.B) {
	adapter := newTestSessionAwareAdapter().(*sessionAwareAdapter)
	b.Cleanup(adapter.Close)
	adapter.maxDisconnectionDuration = (24 * time.Hour).Milliseconds()
	rooms := types.NewSet[Room]("room1", "room2", "room3", "room4", "room5")
	now := time.Now().UnixMilli()
	adapter.sessions.Store("pid", &SessionWithTimestamp{
		SessionToPersist: &SessionToPersist{Pid: "pid", Rooms: rooms},
		DisconnectedAt:   now,
	})
	for i := range 1_000 {
		adapter.packets.Push(&PersistedPacket{
			Id:        strconv.Itoa(i),
			EmittedAt: now,
			Data:      i,
			Opts: &BroadcastOptions{
				Rooms:  types.NewSet[Room]("room1"),
				Except: types.NewSet[Room](),
			},
		})
	}

	b.ReportAllocs()
	for b.Loop() {
		session, err := adapter.RestoreSession("pid", "0")
		if err != nil || session == nil || len(session.MissedPackets) != 999 {
			b.Fatalf("RestoreSession() = (%v, %v)", session, err)
		}
	}
}
