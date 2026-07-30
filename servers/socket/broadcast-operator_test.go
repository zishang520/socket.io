package socket

import (
	"reflect"
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type broadcastAckAdapter struct {
	Adapter
	response []any
}

func (*broadcastAckAdapter) ServerCount() int64 {
	return 1
}

func (a *broadcastAckAdapter) BroadcastWithAck(_ *parser.Packet, _ *BroadcastOptions, clientCount func(uint64), ack Ack) {
	clientCount(1)
	ack(a.response, nil)
}

func TestBroadcastOperatorKeepsFirstAckValue(t *testing.T) {
	for _, test := range []struct {
		name   string
		args   []any
		single bool
		want   []any
	}{
		{name: "multiple arguments", args: []any{"first", "ignored"}, want: []any{"first"}},
		{name: "array value", args: []any{[]any{"first", "second"}}, want: []any{[]any{"first", "second"}}},
		{name: "empty arguments", want: []any{nil}},
		{name: "single response", args: []any{"response"}, single: true, want: []any{"response"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			timeout := time.Hour.Milliseconds()
			operator := NewBroadcastOperator(
				&broadcastAckAdapter{response: test.args},
				nil,
				nil,
				&BroadcastFlags{
					Timeout:              &timeout,
					ExpectSingleResponse: test.single,
				},
			)
			var response []any

			err := operator.Emit("event", func(args []any, err error) {
				if err != nil {
					t.Errorf("unexpected acknowledgement error: %v", err)
				}
				response = args
			})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(response, test.want) {
				t.Fatalf("acknowledgement = %#v, want %#v", response, test.want)
			}
		})
	}
}

func TestBroadcastOperatorTo(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	result := op.To("room1")
	if result == nil {
		t.Fatal("Expected To() to return a non-nil BroadcastOperator")
	}
	if !result.rooms.Has("room1") {
		t.Error("Expected rooms to contain 'room1'")
	}
	// Original should not be modified
	if op.rooms.Has("room1") {
		t.Error("Original BroadcastOperator should not be modified")
	}
}

func TestBroadcastOperatorToMultiple(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	result := op.To("room1", "room2")
	if result.rooms.Len() != 2 {
		t.Errorf("Expected 2 rooms, got %d", result.rooms.Len())
	}
	if !result.rooms.Has("room1") || !result.rooms.Has("room2") {
		t.Error("Expected rooms to contain both 'room1' and 'room2'")
	}
}

func TestBroadcastOperatorToChaining(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	result := op.To("room1").To("room2")
	if result.rooms.Len() != 2 {
		t.Errorf("Expected 2 rooms after chaining, got %d", result.rooms.Len())
	}
}

func TestBroadcastOperatorIn(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	result := op.In("room1")
	if !result.rooms.Has("room1") {
		t.Error("Expected In() to target 'room1'")
	}
}

func TestBroadcastOperatorExcept(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	result := op.Except("room1")
	if result == nil {
		t.Fatal("Expected Except() to return a non-nil BroadcastOperator")
	}
	if !result.exceptRooms.Has("room1") {
		t.Error("Expected exceptRooms to contain 'room1'")
	}
	if op.exceptRooms.Has("room1") {
		t.Error("Original BroadcastOperator should not be modified")
	}
}

func TestBroadcastOperatorExceptMultiple(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	result := op.Except("room1").Except("room2")
	if result.exceptRooms.Len() != 2 {
		t.Errorf("Expected 2 except rooms, got %d", result.exceptRooms.Len())
	}
}

func TestBroadcastOperatorCompress(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	result := op.Compress(true)
	if result.flags.Compress == nil || *result.flags.Compress != true {
		t.Error("Expected Compress flag to be true")
	}

	result2 := op.Compress(false)
	if result2.flags.Compress == nil || *result2.flags.Compress != false {
		t.Error("Expected Compress flag to be false")
	}

	// Original should not be modified
	if op.flags.Compress != nil {
		t.Error("Original BroadcastOperator flags should not be modified")
	}
}

func TestBroadcastOperatorVolatile(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	result := op.Volatile()
	if !result.flags.Volatile {
		t.Error("Expected Volatile flag to be true")
	}
	if op.flags.Volatile {
		t.Error("Original BroadcastOperator flags should not be modified")
	}
}

func TestBroadcastOperatorLocal(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	result := op.Local()
	if !result.flags.Local {
		t.Error("Expected Local flag to be true")
	}
	if op.flags.Local {
		t.Error("Original BroadcastOperator flags should not be modified")
	}
}

func TestBroadcastOperatorTimeout(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	timeout := 5 * time.Second
	result := op.Timeout(timeout)
	if result.flags.Timeout == nil || *result.flags.Timeout != timeout.Milliseconds() {
		t.Errorf("Expected Timeout to be %v", timeout)
	}
	if op.flags.Timeout != nil {
		t.Error("Original BroadcastOperator flags should not be modified")
	}
}

func TestBroadcastOperatorImmutability(t *testing.T) {
	rooms := types.NewSet[Room]("initial")
	except := types.NewSet[Room]("excluded")
	flags := &BroadcastFlags{
		WriteOptions: WriteOptions{Volatile: true},
	}

	op := NewBroadcastOperator(nil, rooms, except, flags)

	// Chain operations and verify original is unchanged
	_ = op.To("new-room").Except("new-except").Compress(false).Volatile().Local()

	if op.rooms.Len() != 1 || !op.rooms.Has("initial") {
		t.Error("Original rooms should not be modified")
	}
	if op.exceptRooms.Len() != 1 || !op.exceptRooms.Has("excluded") {
		t.Error("Original exceptRooms should not be modified")
	}
}

func TestBroadcastOperatorEmitReservedEvent(t *testing.T) {
	op := MakeBroadcastOperator()
	op.Construct(nil, nil, nil, nil)

	// Reserved events should return an error
	reservedEvents := []string{"connect", "connect_error", "disconnect", "disconnecting", "newListener", "removeListener"}
	for _, ev := range reservedEvents {
		err := op.Emit(ev)
		if err == nil {
			t.Errorf("Expected error when emitting reserved event %q", ev)
		}
	}
}

func TestNewBroadcastOperatorWithNilParams(t *testing.T) {
	op := NewBroadcastOperator(nil, nil, nil, nil)
	if op == nil {
		t.Fatal("Expected non-nil BroadcastOperator")
	}
	if op.rooms == nil {
		t.Error("Expected rooms to be initialized")
	}
	if op.exceptRooms == nil {
		t.Error("Expected exceptRooms to be initialized")
	}
	if op.flags == nil {
		t.Error("Expected flags to be initialized")
	}
}
