package socket

import (
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
)

type countedBroadcastAdapter struct {
	Adapter
	count int64
	err   error
	calls int
}

type repeatingBroadcastAckAdapter struct {
	*broadcastAckAdapter
	ack Ack
}

func (a *repeatingBroadcastAckAdapter) BroadcastWithAck(_ *parser.Packet, _ *BroadcastOptions, count func(uint64), ack Ack) {
	a.ack = ack
	count(1)
	ack([]any{"first"}, nil)
}

func TestBroadcastAckCompletionCanReenter(t *testing.T) {
	adapter := &repeatingBroadcastAckAdapter{broadcastAckAdapter: new(broadcastAckAdapter)}
	var calls atomic.Int64
	done := make(chan struct{})
	go func() {
		_ = NewBroadcastOperator(adapter, nil, nil, nil).Timeout(time.Second).Emit("event", func(_ []any, _ error) {
			calls.Add(1)
			adapter.ack(nil, errors.New("late duplicate error"))
		})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcast completion deadlocked on a repeated adapter callback")
	}
	if calls.Load() != 1 {
		t.Fatalf("completion called %d times, want once", calls.Load())
	}
}

func (a *countedBroadcastAdapter) ServerCount() (int64, error) {
	a.calls++
	return a.count, a.err
}

func TestBroadcastOperatorLocalAckSkipsServerCount(t *testing.T) {
	for _, test := range []struct {
		name    string
		local   Adapter
		wantLen int
	}{
		{"empty", newTestAdapter(), 0},
		{"one response", &broadcastAckAdapter{response: []any{"ok"}}, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			adapter := &countedBroadcastAdapter{Adapter: test.local, count: 2, err: errors.New("backend unavailable")}
			result := make(chan struct {
				values []any
				err    error
			}, 1)
			operator := NewBroadcastOperator(adapter, nil, nil, nil).Local().Timeout(time.Second)
			if err := operator.Emit("event", func(values []any, err error) {
				result <- struct {
					values []any
					err    error
				}{values, err}
			}); err != nil {
				t.Fatal(err)
			}
			select {
			case response := <-result:
				if response.err != nil || len(response.values) != test.wantLen {
					t.Fatalf("local ACK = %v, %v; want %d responses", response.values, response.err, test.wantLen)
				}
			default:
				t.Fatal("local ACK did not complete immediately")
			}
			if adapter.calls != 0 {
				t.Fatalf("ServerCount called %d times for a local broadcast", adapter.calls)
			}
		})
	}
}

type delayedAckRegistration struct {
	Adapter
	client *Socket
}

func (a *delayedAckRegistration) BroadcastWithAck(packet *parser.Packet, _ *BroadcastOptions, count func(uint64), ack Ack) {
	time.Sleep(20 * time.Millisecond)
	packet.Id = new(uint64(17))
	a.client.Acks().Store(*packet.Id, ack)
	count(1)
}

func TestBroadcastOperatorTimeoutBeforeAckRegistration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		local := newTestAdapter()
		client := MakeSocket()
		local.Nsp().Sockets().Store("client", client)
		adapter := &delayedAckRegistration{Adapter: local, client: client}
		var calls atomic.Int64
		operator := NewBroadcastOperator(adapter, nil, nil, nil).Timeout(10 * time.Millisecond)
		if err := operator.Emit("event", func(_ []any, err error) {
			calls.Add(1)
			if err == nil {
				t.Error("ACK succeeded after its deadline")
			}
		}); err != nil {
			t.Fatal(err)
		}
		if calls.Load() != 1 || client.Acks().Len() != 0 {
			t.Fatalf("callbacks/late ACK registrations = %d/%d, want 1/0", calls.Load(), client.Acks().Len())
		}
	})
}

func TestBroadcastOperatorTimeoutAfterIdAssignment(t *testing.T) {
	adapter := &countedBroadcastAdapter{Adapter: newTestAdapter(), count: 2}
	result := make(chan error, 1)
	if err := NewBroadcastOperator(adapter, nil, nil, nil).Timeout(time.Millisecond).Emit("event", func(_ []any, err error) {
		result <- err
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("missing node response did not time out")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout callback did not run")
	}
}
