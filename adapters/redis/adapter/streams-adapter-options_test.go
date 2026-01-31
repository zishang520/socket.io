package adapter

import (
	"testing"
	"time"
)

func TestDefaultRedisStreamsAdapterOptions(t *testing.T) {
	opts := DefaultRedisStreamsAdapterOptions()

	if opts == nil {
		t.Fatal("Expected non-nil options")
	}

	t.Run("default values are nil", func(t *testing.T) {
		if opts.GetRawStreamName() != nil {
			t.Fatal("Expected nil RawStreamName by default")
		}
		if opts.GetRawMaxLen() != nil {
			t.Fatal("Expected nil RawMaxLen by default")
		}
		if opts.GetRawReadCount() != nil {
			t.Fatal("Expected nil RawReadCount by default")
		}
		if opts.GetRawSessionKeyPrefix() != nil {
			t.Fatal("Expected nil RawSessionKeyPrefix by default")
		}
		if opts.GetRawHeartbeatInterval() != nil {
			t.Fatal("Expected nil RawHeartbeatInterval by default")
		}
		if opts.GetRawHeartbeatTimeout() != nil {
			t.Fatal("Expected nil RawHeartbeatTimeout by default")
		}
	})
}

func TestRedisStreamsAdapterOptions_StreamName(t *testing.T) {
	opts := DefaultRedisStreamsAdapterOptions()

	t.Run("empty by default", func(t *testing.T) {
		if opts.StreamName() != "" {
			t.Fatalf("Expected empty, got %s", opts.StreamName())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		opts.SetStreamName("my-stream")
		if opts.StreamName() != "my-stream" {
			t.Fatalf("Expected 'my-stream', got %s", opts.StreamName())
		}
	})
}

func TestRedisStreamsAdapterOptions_MaxLen(t *testing.T) {
	opts := DefaultRedisStreamsAdapterOptions()

	t.Run("zero by default", func(t *testing.T) {
		if opts.MaxLen() != 0 {
			t.Fatalf("Expected 0, got %d", opts.MaxLen())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		opts.SetMaxLen(10000)
		if opts.MaxLen() != 10000 {
			t.Fatalf("Expected 10000, got %d", opts.MaxLen())
		}
	})
}

func TestRedisStreamsAdapterOptions_ReadCount(t *testing.T) {
	opts := DefaultRedisStreamsAdapterOptions()

	t.Run("zero by default", func(t *testing.T) {
		if opts.ReadCount() != 0 {
			t.Fatalf("Expected 0, got %d", opts.ReadCount())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		opts.SetReadCount(100)
		if opts.ReadCount() != 100 {
			t.Fatalf("Expected 100, got %d", opts.ReadCount())
		}
	})
}

func TestRedisStreamsAdapterOptions_SessionKeyPrefix(t *testing.T) {
	opts := DefaultRedisStreamsAdapterOptions()

	t.Run("empty by default", func(t *testing.T) {
		if opts.SessionKeyPrefix() != "" {
			t.Fatalf("Expected empty, got %s", opts.SessionKeyPrefix())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		opts.SetSessionKeyPrefix("sio:session:")
		if opts.SessionKeyPrefix() != "sio:session:" {
			t.Fatalf("Expected 'sio:session:', got %s", opts.SessionKeyPrefix())
		}
	})
}

func TestRedisStreamsAdapterOptions_HeartbeatInterval(t *testing.T) {
	opts := DefaultRedisStreamsAdapterOptions()

	t.Run("zero by default", func(t *testing.T) {
		if opts.HeartbeatInterval() != 0 {
			t.Fatalf("Expected 0, got %v", opts.HeartbeatInterval())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		interval := 5 * time.Second
		opts.SetHeartbeatInterval(interval)
		if opts.HeartbeatInterval() != interval {
			t.Fatalf("Expected %v, got %v", interval, opts.HeartbeatInterval())
		}
	})
}

func TestRedisStreamsAdapterOptions_HeartbeatTimeout(t *testing.T) {
	opts := DefaultRedisStreamsAdapterOptions()

	t.Run("zero by default", func(t *testing.T) {
		if opts.HeartbeatTimeout() != 0 {
			t.Fatalf("Expected 0, got %v", opts.HeartbeatTimeout())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		var timeout int64 = 10000 // milliseconds
		opts.SetHeartbeatTimeout(timeout)
		if opts.HeartbeatTimeout() != timeout {
			t.Fatalf("Expected %v, got %v", timeout, opts.HeartbeatTimeout())
		}
	})
}

func TestRedisStreamsAdapterOptions_Assign(t *testing.T) {
	t.Run("assign nil", func(t *testing.T) {
		opts := DefaultRedisStreamsAdapterOptions()
		result := opts.Assign(nil)
		if result != opts {
			t.Fatal("Expected same instance when assigning nil")
		}
	})

	t.Run("assign all fields", func(t *testing.T) {
		source := DefaultRedisStreamsAdapterOptions()
		source.SetStreamName("src-stream")
		source.SetMaxLen(5000)
		source.SetReadCount(50)
		source.SetSessionKeyPrefix("prefix:")
		source.SetHeartbeatInterval(3 * time.Second)
		source.SetHeartbeatTimeout(15000) // milliseconds

		target := DefaultRedisStreamsAdapterOptions()
		target.Assign(source)

		if target.StreamName() != "src-stream" {
			t.Fatalf("Expected 'src-stream', got %s", target.StreamName())
		}
		if target.MaxLen() != 5000 {
			t.Fatalf("Expected 5000, got %d", target.MaxLen())
		}
		if target.ReadCount() != 50 {
			t.Fatalf("Expected 50, got %d", target.ReadCount())
		}
		if target.SessionKeyPrefix() != "prefix:" {
			t.Fatalf("Expected 'prefix:', got %s", target.SessionKeyPrefix())
		}
		if target.HeartbeatInterval() != 3*time.Second {
			t.Fatalf("Expected 3s, got %v", target.HeartbeatInterval())
		}
		if target.HeartbeatTimeout() != 15000 {
			t.Fatalf("Expected 15000, got %v", target.HeartbeatTimeout())
		}
	})

	t.Run("partial assign preserves existing values", func(t *testing.T) {
		source := DefaultRedisStreamsAdapterOptions()
		source.SetStreamName("new-stream")

		target := DefaultRedisStreamsAdapterOptions()
		target.SetMaxLen(1000)
		target.SetReadCount(25)
		target.Assign(source)

		if target.StreamName() != "new-stream" {
			t.Fatalf("Expected 'new-stream', got %s", target.StreamName())
		}
		// Original values should be preserved
		if target.MaxLen() != 1000 {
			t.Fatalf("Expected 1000 to be preserved, got %d", target.MaxLen())
		}
		if target.ReadCount() != 25 {
			t.Fatalf("Expected 25 to be preserved, got %d", target.ReadCount())
		}
	})
}

func TestDefaultStreamOptionsConstants(t *testing.T) {
	tests := []struct {
		name     string
		constant int64
		expected int64
	}{
		{"DefaultStreamMaxLen", DefaultStreamMaxLen, 10000},
		{"DefaultStreamReadCount", DefaultStreamReadCount, 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.constant != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, tt.constant)
			}
		})
	}
}
