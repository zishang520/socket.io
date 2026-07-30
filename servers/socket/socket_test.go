package socket

import (
	"testing"
	"time"
)

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
