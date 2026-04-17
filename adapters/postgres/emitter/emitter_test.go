package emitter

import (
	"testing"
)

func TestEmitter_Of(t *testing.T) {
	// Test Of with nil client - just testing namespace handling
	e := MakeEmitter()
	e.opts.SetKey(DefaultEmitterKey)
	e.opts.SetTableName(DefaultTableName)
	e.opts.SetPayloadThreshold(DefaultPayloadThreshold)
	e.broadcastOptions = &BroadcastOptions{
		Nsp:              "/",
		BroadcastChannel: DefaultEmitterKey + "#/",
		TableName:        DefaultTableName,
		PayloadThreshold: DefaultPayloadThreshold,
	}

	t.Run("with leading slash", func(t *testing.T) {
		ne := e.Of("/admin")
		if ne.nsp != "/admin" {
			t.Fatalf("Expected '/admin', got %s", ne.nsp)
		}
	})

	t.Run("without leading slash", func(t *testing.T) {
		ne := e.Of("admin")
		if ne.nsp != "/admin" {
			t.Fatalf("Expected '/admin', got %s", ne.nsp)
		}
	})
}

func TestEmitter_ServerSideEmit_WithAck(t *testing.T) {
	e := MakeEmitter()
	e.opts.SetKey(DefaultEmitterKey)
	e.opts.SetTableName(DefaultTableName)
	e.opts.SetPayloadThreshold(DefaultPayloadThreshold)
	e.broadcastOptions = &BroadcastOptions{
		Nsp:              "/",
		BroadcastChannel: DefaultEmitterKey + "#/",
		TableName:        DefaultTableName,
		PayloadThreshold: DefaultPayloadThreshold,
	}

	// ServerSideEmit with ack callback should return error
	err := e.ServerSideEmit("test", "data", func(args ...any) {})
	if err == nil {
		t.Fatal("Expected error for ack callback in ServerSideEmit")
	}
}

func TestEmitter_ChainedMethods(t *testing.T) {
	e := MakeEmitter()
	e.opts.SetKey(DefaultEmitterKey)
	e.opts.SetTableName(DefaultTableName)
	e.opts.SetPayloadThreshold(DefaultPayloadThreshold)
	e.broadcastOptions = &BroadcastOptions{
		Nsp:              "/",
		BroadcastChannel: DefaultEmitterKey + "#/",
		TableName:        DefaultTableName,
		PayloadThreshold: DefaultPayloadThreshold,
	}

	// Just verify these don't panic
	e.To("room1")
	e.In("room1")
	e.Except("room1")
	e.Volatile()
	e.Compress(false)
}
