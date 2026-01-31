package adapter

import (
	"testing"
)

func TestSubscriptionModeConstants(t *testing.T) {
	t.Run("StaticSubscriptionMode", func(t *testing.T) {
		if StaticSubscriptionMode != "static" {
			t.Errorf("Expected 'static', got %q", StaticSubscriptionMode)
		}
	})

	t.Run("DynamicSubscriptionMode", func(t *testing.T) {
		if DynamicSubscriptionMode != "dynamic" {
			t.Errorf("Expected 'dynamic', got %q", DynamicSubscriptionMode)
		}
	})

	t.Run("DynamicPrivateSubscriptionMode", func(t *testing.T) {
		if DynamicPrivateSubscriptionMode != "dynamic-private" {
			t.Errorf("Expected 'dynamic-private', got %q", DynamicPrivateSubscriptionMode)
		}
	})

	t.Run("DefaultShardedSubscriptionMode", func(t *testing.T) {
		if DefaultShardedSubscriptionMode != DynamicSubscriptionMode {
			t.Errorf("Expected DynamicSubscriptionMode, got %q", DefaultShardedSubscriptionMode)
		}
	})
}

func TestDefaultShardedRedisAdapterOptions(t *testing.T) {
	opts := DefaultShardedRedisAdapterOptions()

	if opts == nil {
		t.Fatal("Expected non-nil options")
	}

	t.Run("default values", func(t *testing.T) {
		if opts.GetRawSubscriptionMode() != nil {
			t.Fatal("Expected nil RawSubscriptionMode by default")
		}
		if opts.GetRawChannelPrefix() != nil {
			t.Fatal("Expected nil RawChannelPrefix by default")
		}
	})
}

func TestShardedRedisAdapterOptions_SubscriptionMode(t *testing.T) {
	opts := DefaultShardedRedisAdapterOptions()

	t.Run("empty by default", func(t *testing.T) {
		// Default is empty string before set
		if opts.SubscriptionMode() != "" {
			t.Fatalf("Expected empty, got %v", opts.SubscriptionMode())
		}
	})

	t.Run("set StaticSubscriptionMode", func(t *testing.T) {
		opts.SetSubscriptionMode(StaticSubscriptionMode)
		if opts.SubscriptionMode() != StaticSubscriptionMode {
			t.Fatalf("Expected StaticSubscriptionMode, got %v", opts.SubscriptionMode())
		}
		if opts.GetRawSubscriptionMode() == nil {
			t.Fatal("Expected non-nil RawSubscriptionMode after set")
		}
	})

	t.Run("set DynamicSubscriptionMode", func(t *testing.T) {
		opts.SetSubscriptionMode(DynamicSubscriptionMode)
		if opts.SubscriptionMode() != DynamicSubscriptionMode {
			t.Fatalf("Expected DynamicSubscriptionMode, got %v", opts.SubscriptionMode())
		}
	})
}

func TestShardedRedisAdapterOptions_ChannelPrefix(t *testing.T) {
	opts := DefaultShardedRedisAdapterOptions()

	t.Run("empty by default", func(t *testing.T) {
		if opts.ChannelPrefix() != "" {
			t.Fatalf("Expected empty, got %s", opts.ChannelPrefix())
		}
	})

	t.Run("set and get", func(t *testing.T) {
		opts.SetChannelPrefix("my-prefix")
		if opts.ChannelPrefix() != "my-prefix" {
			t.Fatalf("Expected 'my-prefix', got %s", opts.ChannelPrefix())
		}
	})
}

func TestShardedRedisAdapterOptions_Assign(t *testing.T) {
	t.Run("assign nil", func(t *testing.T) {
		opts := DefaultShardedRedisAdapterOptions()
		result := opts.Assign(nil)
		if result != opts {
			t.Fatal("Expected same instance when assigning nil")
		}
	})

	t.Run("assign from ShardedRedisAdapterOptions", func(t *testing.T) {
		source := DefaultShardedRedisAdapterOptions()
		source.SetSubscriptionMode(StaticSubscriptionMode)
		source.SetChannelPrefix("src-prefix")

		target := DefaultShardedRedisAdapterOptions()
		target.Assign(source)

		if target.SubscriptionMode() != StaticSubscriptionMode {
			t.Fatalf("Expected StaticSubscriptionMode, got %v", target.SubscriptionMode())
		}
		if target.ChannelPrefix() != "src-prefix" {
			t.Fatalf("Expected 'src-prefix', got %s", target.ChannelPrefix())
		}
	})

	t.Run("partial assign preserves existing values", func(t *testing.T) {
		source := DefaultShardedRedisAdapterOptions()
		source.SetChannelPrefix("new-prefix")

		target := DefaultShardedRedisAdapterOptions()
		target.SetSubscriptionMode(StaticSubscriptionMode)
		target.Assign(source)

		if target.ChannelPrefix() != "new-prefix" {
			t.Fatalf("Expected 'new-prefix', got %s", target.ChannelPrefix())
		}
		// Original subscription mode should be preserved
		if target.SubscriptionMode() != StaticSubscriptionMode {
			t.Fatalf("Expected StaticSubscriptionMode to be preserved, got %v", target.SubscriptionMode())
		}
	})
}

func TestDefaultShardedChannelPrefix(t *testing.T) {
	if DefaultShardedChannelPrefix != "socket.io" {
		t.Errorf("Expected 'socket.io', got %q", DefaultShardedChannelPrefix)
	}
}
