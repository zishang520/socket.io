package adapter

import (
	"testing"
)

func TestCacheStreamsAdapterOptions_Defaults(t *testing.T) {
	opts := DefaultCacheStreamsAdapterOptions()
	if opts.StreamName() != "" {
		t.Errorf("expected empty StreamName, got %q", opts.StreamName())
	}
	if opts.MaxLen() != 0 {
		t.Errorf("expected 0 MaxLen, got %d", opts.MaxLen())
	}
	if opts.ReadCount() != 0 {
		t.Errorf("expected 0 ReadCount, got %d", opts.ReadCount())
	}
	if opts.SessionKeyPrefix() != "" {
		t.Errorf("expected empty SessionKeyPrefix, got %q", opts.SessionKeyPrefix())
	}
}

func TestCacheStreamsAdapterOptions_Assign(t *testing.T) {
	src := DefaultCacheStreamsAdapterOptions()
	src.SetStreamName("my-stream")
	src.SetMaxLen(5000)
	src.SetReadCount(50)
	src.SetSessionKeyPrefix("sess:")

	dst := DefaultCacheStreamsAdapterOptions()
	dst.Assign(src)

	if dst.StreamName() != "my-stream" {
		t.Errorf("StreamName: got %q, want %q", dst.StreamName(), "my-stream")
	}
	if dst.MaxLen() != 5000 {
		t.Errorf("MaxLen: got %d, want 5000", dst.MaxLen())
	}
	if dst.ReadCount() != 50 {
		t.Errorf("ReadCount: got %d, want 50", dst.ReadCount())
	}
	if dst.SessionKeyPrefix() != "sess:" {
		t.Errorf("SessionKeyPrefix: got %q, want %q", dst.SessionKeyPrefix(), "sess:")
	}
}

func TestCacheStreamsAdapterOptions_AssignNil(t *testing.T) {
	opts := DefaultCacheStreamsAdapterOptions()
	result := opts.Assign(nil)
	if result == nil {
		t.Error("Assign(nil) should return the options, not nil")
	}
}

func TestDefaultStreamConstants(t *testing.T) {
	if DefaultStreamName != "socket.io" {
		t.Errorf("DefaultStreamName = %q, want %q", DefaultStreamName, "socket.io")
	}
	if DefaultStreamMaxLen != 10_000 {
		t.Errorf("DefaultStreamMaxLen = %d, want 10000", DefaultStreamMaxLen)
	}
	if DefaultStreamReadCount != 100 {
		t.Errorf("DefaultStreamReadCount = %d, want 100", DefaultStreamReadCount)
	}
	if DefaultSessionKeyPrefix != "sio:session:" {
		t.Errorf("DefaultSessionKeyPrefix = %q, want %q", DefaultSessionKeyPrefix, "sio:session:")
	}
}
