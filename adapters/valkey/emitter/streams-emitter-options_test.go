package emitter

import (
	"context"
	"reflect"
	"testing"

	"github.com/zishang520/socket.io/adapters/valkey/v3"
)

var _ ValkeyStreamsEmitterOptionsInterface = (*ValkeyStreamsEmitterOptions)(nil)

func TestValkeyStreamsEmitterOptions(t *testing.T) {
	opts := DefaultValkeyStreamsEmitterOptions()
	if opts.GetRawStreamName() != nil || opts.GetRawStreamCount() != nil || opts.GetRawMaxLen() != nil {
		t.Fatal("default options contain explicitly set values")
	}
	var typedNil *ValkeyStreamsEmitterOptions
	if result := opts.Assign(typedNil); result != opts {
		t.Fatal("typed nil assignment did not preserve the target")
	}

	values := DefaultValkeyStreamsEmitterOptions()
	values.SetStreamName("")
	values.SetStreamCount(0)
	values.SetMaxLen(0)
	opts.Assign(values)
	if opts.GetRawStreamName() == nil || opts.StreamName() != "" ||
		opts.GetRawStreamCount() == nil || opts.StreamCount() != 0 ||
		opts.GetRawMaxLen() == nil || opts.MaxLen() != 0 {
		t.Fatal("explicit zero values were not preserved")
	}
}

func TestValkeyStreamsEmitterDefaultsAndChaining(t *testing.T) {
	emitter := NewValkeyStreamsEmitter(nil, nil)
	if emitter.opts.StreamName() != DefaultStreamName || emitter.opts.StreamCount() != DefaultStreamCount ||
		emitter.opts.MaxLen() != DefaultStreamMaxLen {
		t.Fatalf("options = %q, %d, %d", emitter.opts.StreamName(), emitter.opts.StreamCount(), emitter.opts.MaxLen())
	}

	result, ok := emitter.newBroadcastOperator().To("room").Except("excluded").Volatile().Compress(false).(*ValkeyStreamsBroadcastOperator)
	if !ok {
		t.Fatal("chaining changed the Valkey Streams operator type")
	}
	if !result.rooms.Has("room") || !result.exceptRooms.Has("excluded") ||
		!result.flags.Volatile || result.flags.Compress == nil || *result.flags.Compress {
		t.Fatal("chaining lost selection or flags")
	}
}

func TestValkeyStreamsEmitterNodeWireAndRouting(t *testing.T) {
	valkeyClient, _ := newEmitterTestClient(t)
	opts := DefaultValkeyStreamsEmitterOptions()
	opts.SetStreamName("events")
	opts.SetStreamCount(5)
	opts.SetMaxLen(100)
	emitter := NewValkeyStreamsEmitter(valkeyClient, opts).Of("/namespace-0")

	if err := emitter.Emit("event", []byte{1, 2}); err != nil {
		t.Fatal(err)
	}
	streamName := valkey.StreamNameForNamespace("events", "/namespace-0", 5)
	entries, err := valkeyClient.XRangeN(context.Background(), streamName, "-", "+", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	message, err := valkey.DecodeStreamMessage(valkey.RawClusterMessage(entries[0].FieldValues))
	if err != nil {
		t.Fatal(err)
	}
	data, ok := message.Data.(*BroadcastMessage)
	if !ok || message.Uid != "emitter" || message.Nsp != "/namespace-0" || data.Packet == nil ||
		data.Packet.Nsp != "/namespace-0" || !reflect.DeepEqual(data.Packet.Data.([]any)[1], []byte{1, 2}) {
		t.Fatalf("message = %#v, data = %#v", message, message.Data)
	}
}
