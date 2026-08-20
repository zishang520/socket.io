package emitter

import "testing"

func TestRedisStreamsEmitterOptions(t *testing.T) {
	opts := DefaultRedisStreamsEmitterOptions()
	if opts.GetRawStreamName() != nil || opts.GetRawStreamCount() != nil || opts.GetRawMaxLen() != nil {
		t.Fatal("default options contain explicitly set values")
	}

	values := DefaultRedisStreamsEmitterOptions()
	values.SetStreamName("")
	values.SetStreamCount(0)
	values.SetMaxLen(0)
	opts.Assign(values)
	if opts.GetRawStreamName() == nil || opts.StreamName() != "" {
		t.Fatal("explicit empty stream name was not preserved")
	}
	if opts.GetRawStreamCount() == nil || opts.StreamCount() != 0 {
		t.Fatal("explicit zero stream count was not preserved")
	}
	if opts.GetRawMaxLen() == nil || opts.MaxLen() != 0 {
		t.Fatal("explicit zero max length was not preserved")
	}
}

func TestRedisStreamsEmitterOptionsDefaults(t *testing.T) {
	emitter := NewRedisStreamsEmitter(nil, nil)
	if emitter.opts.StreamName() != DefaultStreamName || emitter.opts.StreamCount() != DefaultStreamCount ||
		emitter.opts.MaxLen() != DefaultStreamMaxLen {
		t.Fatalf("options = %q, %d, %d", emitter.opts.StreamName(), emitter.opts.StreamCount(), emitter.opts.MaxLen())
	}

	opts := DefaultRedisStreamsEmitterOptions()
	opts.SetStreamName("")
	opts.SetStreamCount(0)
	opts.SetMaxLen(0)
	emitter = NewRedisStreamsEmitter(nil, opts)
	if emitter.opts.StreamName() != "" || emitter.opts.StreamCount() != 0 || emitter.opts.MaxLen() != 0 {
		t.Fatalf("explicit options = %q, %d, %d", emitter.opts.StreamName(), emitter.opts.StreamCount(), emitter.opts.MaxLen())
	}
	scoped := emitter.Of("/chat")
	if scoped.opts.StreamName() != "" || scoped.opts.StreamCount() != 0 || scoped.opts.MaxLen() != 0 {
		t.Fatalf("scoped options = %q, %d, %d", scoped.opts.StreamName(), scoped.opts.StreamCount(), scoped.opts.MaxLen())
	}
}
