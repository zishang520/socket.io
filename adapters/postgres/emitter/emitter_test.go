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

func TestEmitter_NilOptions(t *testing.T) {
	e := NewEmitter(nil, nil)

	if e.opts.ChannelPrefix() != DefaultChannelPrefix {
		t.Fatalf("expected channel prefix %q, got %q", DefaultChannelPrefix, e.opts.ChannelPrefix())
	}
	if e.broadcastOptions.Nsp != defaultNamespace {
		t.Fatalf("expected namespace %q, got %q", defaultNamespace, e.broadcastOptions.Nsp)
	}
}

func TestEmitter_ExplicitEmptyNamespace(t *testing.T) {
	e := NewEmitter(nil, nil, "")

	if e.broadcastOptions.Nsp != "" {
		t.Fatalf("expected empty namespace, got %q", e.broadcastOptions.Nsp)
	}
	if e.broadcastOptions.BroadcastChannel != DefaultChannelPrefix+"#" {
		t.Fatalf("expected channel %q, got %q", DefaultChannelPrefix+"#", e.broadcastOptions.BroadcastChannel)
	}
}

func TestEmitter_Of(t *testing.T) {
	opts := DefaultEmitterOptions()
	opts.SetChannelPrefix("custom")
	opts.SetTableName("custom_attachments")
	opts.SetPayloadThreshold(4_000)
	e := NewEmitter(nil, opts)

	t.Run("with leading slash", func(t *testing.T) {
		ne := e.Of("/admin")
		if ne.broadcastOptions.Nsp != "/admin" {
			t.Fatalf("namespace = %q, want /admin", ne.broadcastOptions.Nsp)
		}
		if ne.broadcastOptions.BroadcastChannel != "custom#/admin" {
			t.Fatalf("channel = %q, want custom#/admin", ne.broadcastOptions.BroadcastChannel)
		}
		if ne.broadcastOptions.TableName != "custom_attachments" || ne.broadcastOptions.PayloadThreshold != 4_000 {
			t.Fatal("Of() did not preserve emitter options")
		}
	})

	t.Run("without leading slash", func(t *testing.T) {
		ne := e.Of("admin")
		if ne.broadcastOptions.Nsp != "/admin" {
			t.Fatalf("namespace = %q, want /admin", ne.broadcastOptions.Nsp)
		}
	})
}

func TestEmitter_ServerSideEmit_WithAck(t *testing.T) {
	err := NewEmitter(nil, nil).ServerSideEmit("test", "data", func([]any, error) {})
	if err == nil || err.Error() != "Acknowledgements are not supported" {
		t.Fatalf("expected Node.js acknowledgement error, got %v", err)
	}
}
