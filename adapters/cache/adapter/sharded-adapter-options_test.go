package adapter

import (
	"testing"

	cache "github.com/zishang520/socket.io/adapters/cache/v3"
)

func TestShardedCacheAdapterOptions_Defaults(t *testing.T) {
	opts := DefaultShardedCacheAdapterOptions()
	if opts.GetRawChannelPrefix() != nil {
		t.Error("expected nil channel prefix on fresh options")
	}
	if opts.ChannelPrefix() != "" {
		t.Errorf("expected empty channel prefix, got %q", opts.ChannelPrefix())
	}
	if opts.GetRawSubscriptionMode() != nil {
		t.Error("expected nil subscription mode on fresh options")
	}
	if opts.SubscriptionMode() != "" {
		t.Errorf("expected empty subscription mode, got %q", opts.SubscriptionMode())
	}
}

func TestShardedCacheAdapterOptions_Assign(t *testing.T) {
	src := DefaultShardedCacheAdapterOptions()
	src.SetChannelPrefix("myprefix")
	src.SetSubscriptionMode(cache.DynamicPrivateSubscriptionMode)

	dst := DefaultShardedCacheAdapterOptions()
	dst.Assign(src)

	if dst.ChannelPrefix() != "myprefix" {
		t.Errorf("ChannelPrefix: got %q, want %q", dst.ChannelPrefix(), "myprefix")
	}
	if dst.SubscriptionMode() != cache.DynamicPrivateSubscriptionMode {
		t.Errorf("SubscriptionMode: got %q, want %q", dst.SubscriptionMode(), cache.DynamicPrivateSubscriptionMode)
	}
}

func TestShardedCacheAdapterOptions_AssignNil(t *testing.T) {
	opts := DefaultShardedCacheAdapterOptions()
	result := opts.Assign(nil)
	if result == nil {
		t.Error("Assign(nil) should return the options, not nil")
	}
}
