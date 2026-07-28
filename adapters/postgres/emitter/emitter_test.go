package emitter

import (
	"testing"
)

func TestEmitter_NodeDefaults(t *testing.T) {
	opts := DefaultEmitterOptions()
	opts.SetChannelPrefix("")
	opts.SetTableName("")
	opts.SetPayloadThreshold(0)

	e := NewEmitter(nil, opts)
	if e.opts.ChannelPrefix() != DefaultChannelPrefix {
		t.Fatalf("expected channel prefix %q, got %q", DefaultChannelPrefix, e.opts.ChannelPrefix())
	}
	if e.opts.TableName() != DefaultTableName {
		t.Fatalf("expected table name %q, got %q", DefaultTableName, e.opts.TableName())
	}
	if e.opts.PayloadThreshold() != DefaultPayloadThreshold {
		t.Fatalf("expected payload threshold %d, got %d", DefaultPayloadThreshold, e.opts.PayloadThreshold())
	}
}

func TestEmitter_Of(t *testing.T) {
	// Test Of with nil client - just testing namespace handling
	e := MakeEmitter()
	e.opts.SetChannelPrefix(DefaultChannelPrefix)
	e.opts.SetTableName(DefaultTableName)
	e.opts.SetPayloadThreshold(DefaultPayloadThreshold)
	e.broadcastOptions = &BroadcastOptions{
		Nsp:              "/",
		BroadcastChannel: DefaultChannelPrefix + "#/",
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
	e.opts.SetChannelPrefix(DefaultChannelPrefix)
	e.opts.SetTableName(DefaultTableName)
	e.opts.SetPayloadThreshold(DefaultPayloadThreshold)
	e.broadcastOptions = &BroadcastOptions{
		Nsp:              "/",
		BroadcastChannel: DefaultChannelPrefix + "#/",
		TableName:        DefaultTableName,
		PayloadThreshold: DefaultPayloadThreshold,
	}

	// ServerSideEmit with ack callback should return error
	err := e.ServerSideEmit("test", "data", func([]any, error) {})
	if err == nil || err.Error() != "Acknowledgements are not supported" {
		t.Fatalf("expected Node.js acknowledgement error, got %v", err)
	}
}

func TestEmitter_ChainedMethods(t *testing.T) {
	e := MakeEmitter()
	e.opts.SetChannelPrefix(DefaultChannelPrefix)
	e.opts.SetTableName(DefaultTableName)
	e.opts.SetPayloadThreshold(DefaultPayloadThreshold)
	e.broadcastOptions = &BroadcastOptions{
		Nsp:              "/",
		BroadcastChannel: DefaultChannelPrefix + "#/",
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
