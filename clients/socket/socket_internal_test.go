package socket

import (
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

func TestRegisterAckCallbackTimeoutRemovesOnlyMatchingBufferedPacket(t *testing.T) {
	s := MakeSocket()
	s._opts = DefaultSocketOptions()

	matchingId := uint64(1)
	otherId := uint64(2)
	matching := &Packet{Packet: &parser.Packet{Id: &matchingId}}
	other := &Packet{Packet: &parser.Packet{Id: &otherId}}
	withoutAck := &Packet{Packet: &parser.Packet{}}

	s.sendBuffer.Push(matching, other, withoutAck)

	timedOut := make(chan error, 1)
	timeout := 10 * time.Millisecond
	s._registerAckCallback(matchingId, func(_ []any, err error) {
		timedOut <- err
	}, &timeout)

	select {
	case err := <-timedOut:
		if err == nil {
			t.Fatal("expected timeout error")
		}
	case <-time.After(time.Second):
		t.Fatal("ack callback did not time out")
	}

	if _, ok := s.acks.Load(matchingId); ok {
		t.Fatal("expected timed out ack to be deleted")
	}

	remaining := s.sendBuffer.All()
	if len(remaining) != 2 {
		t.Fatalf("expected 2 buffered packets after timeout, got %d", len(remaining))
	}
	if remaining[0] != other || remaining[1] != withoutAck {
		t.Fatalf("unexpected remaining packets: %#v", remaining)
	}
}

func TestRegisterAckCallbackClearsTimeoutOnAck(t *testing.T) {
	s := MakeSocket()
	s._opts = DefaultSocketOptions()

	id := uint64(1)
	called := make(chan error, 1)
	timeout := time.Second
	s._registerAckCallback(id, func(_ []any, err error) {
		called <- err
	}, &timeout)

	ack, ok := s.acks.Load(id)
	if !ok {
		t.Fatal("expected ack callback to be stored")
	}
	ack([]any{"ok"}, nil)

	select {
	case err := <-called:
		if err != nil {
			t.Fatalf("expected nil ack error, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ack callback was not called")
	}
}
