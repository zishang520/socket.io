package adapter

import (
	"testing"
	"time"
)

func TestDefaultPostgresAdapterOptions(t *testing.T) {
	opts := DefaultPostgresAdapterOptions()

	if opts == nil {
		t.Fatal("Expected non-nil options")
	}

	t.Run("default values are nil", func(t *testing.T) {
		if opts.GetRawKey() != nil {
			t.Fatal("Expected nil RawKey by default")
		}
		if opts.GetRawTableName() != nil {
			t.Fatal("Expected nil RawTableName by default")
		}
		if opts.GetRawPayloadThreshold() != nil {
			t.Fatal("Expected nil RawPayloadThreshold by default")
		}
		if opts.GetRawCleanupInterval() != nil {
			t.Fatal("Expected nil RawCleanupInterval by default")
		}
		if opts.GetRawHeartbeatInterval() != nil {
			t.Fatal("Expected nil RawHeartbeatInterval by default")
		}
		if opts.GetRawHeartbeatTimeout() != nil {
			t.Fatal("Expected nil RawHeartbeatTimeout by default")
		}
		if opts.GetRawErrorHandler() != nil {
			t.Fatal("Expected nil RawErrorHandler by default")
		}
	})
}

func TestPostgresAdapterOptions_Key(t *testing.T) {
	opts := DefaultPostgresAdapterOptions()

	t.Run("empty by default", func(t *testing.T) {
		if opts.Key() != "" {
			t.Fatalf("Expected empty, got %s", opts.Key())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		opts.SetKey("custom-prefix")
		if opts.Key() != "custom-prefix" {
			t.Fatalf("Expected 'custom-prefix', got %s", opts.Key())
		}
	})
}

func TestPostgresAdapterOptions_TableName(t *testing.T) {
	opts := DefaultPostgresAdapterOptions()

	t.Run("empty by default", func(t *testing.T) {
		if opts.TableName() != "" {
			t.Fatalf("Expected empty, got %s", opts.TableName())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		opts.SetTableName("custom_table")
		if opts.TableName() != "custom_table" {
			t.Fatalf("Expected 'custom_table', got %s", opts.TableName())
		}
	})
}

func TestPostgresAdapterOptions_PayloadThreshold(t *testing.T) {
	opts := DefaultPostgresAdapterOptions()

	t.Run("zero by default", func(t *testing.T) {
		if opts.PayloadThreshold() != 0 {
			t.Fatalf("Expected 0, got %d", opts.PayloadThreshold())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		opts.SetPayloadThreshold(4000)
		if opts.PayloadThreshold() != 4000 {
			t.Fatalf("Expected 4000, got %d", opts.PayloadThreshold())
		}
	})
}

func TestPostgresAdapterOptions_CleanupInterval(t *testing.T) {
	opts := DefaultPostgresAdapterOptions()

	t.Run("zero by default", func(t *testing.T) {
		if opts.CleanupInterval() != 0 {
			t.Fatalf("Expected 0, got %d", opts.CleanupInterval())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		opts.SetCleanupInterval(60000)
		if opts.CleanupInterval() != 60000 {
			t.Fatalf("Expected 60000, got %d", opts.CleanupInterval())
		}
	})
}

func TestPostgresAdapterOptions_HeartbeatInterval(t *testing.T) {
	opts := DefaultPostgresAdapterOptions()

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

func TestPostgresAdapterOptions_HeartbeatTimeout(t *testing.T) {
	opts := DefaultPostgresAdapterOptions()

	t.Run("zero by default", func(t *testing.T) {
		if opts.HeartbeatTimeout() != 0 {
			t.Fatalf("Expected 0, got %v", opts.HeartbeatTimeout())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		var timeout int64 = 10000
		opts.SetHeartbeatTimeout(timeout)
		if opts.HeartbeatTimeout() != timeout {
			t.Fatalf("Expected %v, got %v", timeout, opts.HeartbeatTimeout())
		}
	})
}

func TestPostgresAdapterOptions_ErrorHandler(t *testing.T) {
	opts := DefaultPostgresAdapterOptions()

	t.Run("nil by default", func(t *testing.T) {
		if opts.ErrorHandler() != nil {
			t.Fatal("Expected nil ErrorHandler by default")
		}
	})

	t.Run("set and get", func(t *testing.T) {
		called := false
		handler := func(err error) { called = true }
		opts.SetErrorHandler(handler)
		if opts.ErrorHandler() == nil {
			t.Fatal("Expected non-nil ErrorHandler")
		}
		opts.ErrorHandler()(nil)
		if !called {
			t.Fatal("Expected handler to be called")
		}
	})
}

func TestPostgresAdapterOptions_Assign(t *testing.T) {
	t.Run("assign nil", func(t *testing.T) {
		opts := DefaultPostgresAdapterOptions()
		result := opts.Assign(nil)
		if result != opts {
			t.Fatal("Expected same instance when assigning nil")
		}
	})

	t.Run("assign all fields", func(t *testing.T) {
		source := DefaultPostgresAdapterOptions()
		source.SetKey("src-key")
		source.SetTableName("src_table")
		source.SetPayloadThreshold(4000)
		source.SetCleanupInterval(60000)
		source.SetHeartbeatInterval(3 * time.Second)
		source.SetHeartbeatTimeout(15000)

		target := DefaultPostgresAdapterOptions()
		target.Assign(source)

		if target.Key() != "src-key" {
			t.Fatalf("Expected 'src-key', got %s", target.Key())
		}
		if target.TableName() != "src_table" {
			t.Fatalf("Expected 'src_table', got %s", target.TableName())
		}
		if target.PayloadThreshold() != 4000 {
			t.Fatalf("Expected 4000, got %d", target.PayloadThreshold())
		}
		if target.CleanupInterval() != 60000 {
			t.Fatalf("Expected 60000, got %d", target.CleanupInterval())
		}
		if target.HeartbeatInterval() != 3*time.Second {
			t.Fatalf("Expected 3s, got %v", target.HeartbeatInterval())
		}
		if target.HeartbeatTimeout() != 15000 {
			t.Fatalf("Expected 15000, got %v", target.HeartbeatTimeout())
		}
	})

	t.Run("partial assign preserves existing values", func(t *testing.T) {
		source := DefaultPostgresAdapterOptions()
		source.SetKey("new-key")

		target := DefaultPostgresAdapterOptions()
		target.SetTableName("existing_table")
		target.SetCleanupInterval(60000)
		target.Assign(source)

		if target.Key() != "new-key" {
			t.Fatalf("Expected 'new-key', got %s", target.Key())
		}
		// Original values should be preserved
		if target.TableName() != "existing_table" {
			t.Fatalf("Expected 'existing_table' to be preserved, got %s", target.TableName())
		}
		if target.CleanupInterval() != 60000 {
			t.Fatalf("Expected 60000 to be preserved, got %d", target.CleanupInterval())
		}
	})
}
