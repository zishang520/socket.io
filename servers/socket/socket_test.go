package socket

import (
	"testing"
	"time"
)

func TestSocketDefaultData(t *testing.T) {
	socket := MakeSocket()
	t.Cleanup(socket.taskQueue.Close)

	data, ok := socket.Data().(map[string]any)
	if !ok || len(data) != 0 {
		t.Fatalf("Data() = %#v, want an empty map", socket.Data())
	}
	data["socket"] = true
	other := MakeSocket()
	t.Cleanup(other.taskQueue.Close)
	if len(other.Data().(map[string]any)) != 0 {
		t.Fatal("default socket data is shared between sockets")
	}

	socket.SetData(nil)
	if socket.Data() != nil {
		t.Fatalf("Data() = %#v after SetData(nil), want nil", socket.Data())
	}
}

func TestSocketNewBroadcastOperatorConsumesFlags(t *testing.T) {
	socket := MakeSocket()
	t.Cleanup(socket.taskQueue.Close)
	socket.id = "socket"
	socket.adapter = newTestAdapter()
	timeout := time.Second
	socket.Compress(false).Volatile().Timeout(timeout)

	operator := socket.newBroadcastOperator()
	if operator.flags.Compress == nil || *operator.flags.Compress {
		t.Fatal("Expected compression to be disabled")
	}
	if !operator.flags.Volatile || operator.flags.Timeout == nil || *operator.flags.Timeout != timeout.Milliseconds() {
		t.Fatal("Expected broadcast flags to be preserved")
	}
	if !operator.exceptRooms.Has(Room(socket.id)) {
		t.Fatal("Expected sender to be excluded")
	}

	next := socket.newBroadcastOperator()
	if next.flags.Compress != nil || next.flags.Volatile || next.flags.Timeout != nil {
		t.Fatal("Expected flags to be reset after use")
	}
}
