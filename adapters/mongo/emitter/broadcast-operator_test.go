package emitter

import (
	"testing"
)

func TestBroadcastOperatorIsImmutable(t *testing.T) {
	base := NewBroadcastOperator(nil, &BroadcastOptions{Nsp: "/"}, nil, nil, nil)
	to := base.To("room-1").(*BroadcastOperator)
	except := to.Except("room-2").(*BroadcastOperator)
	compressed := except.Compress(false).(*BroadcastOperator)
	volatile := compressed.Volatile().(*BroadcastOperator)

	if base.rooms.Len() != 0 || base.exceptRooms.Len() != 0 {
		t.Fatal("base operator was mutated")
	}
	if !to.rooms.Has("room-1") || to.exceptRooms.Len() != 0 {
		t.Fatal("To() did not copy only the room set")
	}
	if !except.rooms.Has("room-1") || !except.exceptRooms.Has("room-2") {
		t.Fatal("Except() did not preserve the previous selection")
	}
	if except.flags.Compress != nil || compressed.flags.Compress == nil || *compressed.flags.Compress {
		t.Fatal("Compress(false) did not copy the flags")
	}
	if compressed.flags.Volatile || !volatile.flags.Volatile {
		t.Fatal("Volatile() did not copy the flags")
	}
}

func TestReservedEventIsRejected(t *testing.T) {
	op := NewBroadcastOperator(nil, nil, nil, nil, nil)
	if err := op.Emit("connect"); err == nil || err.Error() != `"connect" is a reserved event name` {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServerSideEmitRejectsAck(t *testing.T) {
	op := NewBroadcastOperator(nil, nil, nil, nil, nil)
	ack := func([]any, error) {}
	err := op.ServerSideEmit("event", ack)
	if err == nil || err.Error() != "Acknowledgements are not supported" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEmitterNamespaceDefaultsMatchNode(t *testing.T) {
	root := NewEmitter(nil, nil)
	if root.broadcastOptions.Nsp != "/" {
		t.Fatalf("default namespace = %q, want %q", root.broadcastOptions.Nsp, "/")
	}

	empty := NewEmitter(nil, nil, "")
	if empty.broadcastOptions.Nsp != "" {
		t.Fatalf("explicit namespace = %q, want an empty string", empty.broadcastOptions.Nsp)
	}

	if namespace := empty.Of("").broadcastOptions.Nsp; namespace != "/" {
		t.Fatalf("Of(\"\") namespace = %q, want %q", namespace, "/")
	}
}
