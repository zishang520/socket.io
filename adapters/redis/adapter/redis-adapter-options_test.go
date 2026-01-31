package adapter

import (
	"testing"
	"time"
)

func TestDefaultRedisAdapterOptions(t *testing.T) {
	opts := DefaultRedisAdapterOptions()

	if opts == nil {
		t.Fatal("Expected non-nil options")
	}

	t.Run("default values", func(t *testing.T) {
		if opts.GetRawKey() != nil {
			t.Fatal("Expected nil RawKey by default")
		}
		if opts.GetRawParser() != nil {
			t.Fatal("Expected nil RawParser by default")
		}
		if opts.GetRawRequestsTimeout() != nil {
			t.Fatal("Expected nil RawRequestsTimeout by default")
		}
		if opts.GetRawPublishOnSpecificResponseChannel() != nil {
			t.Fatal("Expected nil RawPublishOnSpecificResponseChannel by default")
		}
	})
}

func TestRedisAdapterOptions_Key(t *testing.T) {
	opts := DefaultRedisAdapterOptions()

	t.Run("empty by default", func(t *testing.T) {
		if opts.Key() != "" {
			t.Fatalf("Expected empty key, got %s", opts.Key())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		opts.SetKey("my-prefix")
		if opts.Key() != "my-prefix" {
			t.Fatalf("Expected 'my-prefix', got %s", opts.Key())
		}
		if opts.GetRawKey() == nil {
			t.Fatal("Expected non-nil RawKey after SetKey")
		}
	})
}

func TestRedisAdapterOptions_RequestsTimeout(t *testing.T) {
	opts := DefaultRedisAdapterOptions()

	t.Run("zero by default", func(t *testing.T) {
		if opts.RequestsTimeout() != 0 {
			t.Fatalf("Expected 0, got %v", opts.RequestsTimeout())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		timeout := 10 * time.Second
		opts.SetRequestsTimeout(timeout)
		if opts.RequestsTimeout() != timeout {
			t.Fatalf("Expected %v, got %v", timeout, opts.RequestsTimeout())
		}
		if opts.GetRawRequestsTimeout() == nil {
			t.Fatal("Expected non-nil RawRequestsTimeout after set")
		}
	})
}

func TestRedisAdapterOptions_PublishOnSpecificResponseChannel(t *testing.T) {
	opts := DefaultRedisAdapterOptions()

	t.Run("false by default", func(t *testing.T) {
		if opts.PublishOnSpecificResponseChannel() {
			t.Fatal("Expected false by default")
		}
	})

	t.Run("set to true", func(t *testing.T) {
		opts.SetPublishOnSpecificResponseChannel(true)
		if !opts.PublishOnSpecificResponseChannel() {
			t.Fatal("Expected true after SetPublishOnSpecificResponseChannel(true)")
		}
	})

	t.Run("set to false", func(t *testing.T) {
		opts.SetPublishOnSpecificResponseChannel(false)
		if opts.PublishOnSpecificResponseChannel() {
			t.Fatal("Expected false after SetPublishOnSpecificResponseChannel(false)")
		}
	})
}

func TestRedisAdapterOptions_Assign(t *testing.T) {
	t.Run("assign nil", func(t *testing.T) {
		opts := DefaultRedisAdapterOptions()
		result := opts.Assign(nil)
		if result != opts {
			t.Fatal("Expected same instance when assigning nil")
		}
	})

	t.Run("assign values", func(t *testing.T) {
		source := DefaultRedisAdapterOptions()
		source.SetKey("source-key")
		source.SetRequestsTimeout(3 * time.Second)
		source.SetPublishOnSpecificResponseChannel(true)

		target := DefaultRedisAdapterOptions()
		target.Assign(source)

		if target.Key() != "source-key" {
			t.Fatalf("Expected 'source-key', got %s", target.Key())
		}
		if target.RequestsTimeout() != 3*time.Second {
			t.Fatalf("Expected 3s, got %v", target.RequestsTimeout())
		}
		if !target.PublishOnSpecificResponseChannel() {
			t.Fatal("Expected true")
		}
	})

	t.Run("partial assign", func(t *testing.T) {
		source := DefaultRedisAdapterOptions()
		source.SetKey("partial-key")

		target := DefaultRedisAdapterOptions()
		target.SetRequestsTimeout(5 * time.Second)
		target.Assign(source)

		if target.Key() != "partial-key" {
			t.Fatalf("Expected 'partial-key', got %s", target.Key())
		}
		// Original value should be preserved
		if target.RequestsTimeout() != 5*time.Second {
			t.Fatalf("Expected 5s to be preserved, got %v", target.RequestsTimeout())
		}
	})
}

func TestDefaultRequestsTimeout(t *testing.T) {
	expected := 5000 * time.Millisecond
	if DefaultRequestsTimeout != expected {
		t.Fatalf("Expected DefaultRequestsTimeout to be %v, got %v", expected, DefaultRequestsTimeout)
	}
}
