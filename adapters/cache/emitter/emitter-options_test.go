package emitter

import (
	"testing"

	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

func TestEmitterOptions_Defaults(t *testing.T) {
	opts := DefaultEmitterOptions()

	if opts.GetRawKey() != nil {
		t.Error("expected nil raw key on fresh options")
	}
	if opts.Key() != "" {
		t.Errorf("Key(): got %q, want empty string", opts.Key())
	}
	if opts.GetRawParser() != nil {
		t.Error("expected nil raw parser on fresh options")
	}
	if opts.Parser() != nil {
		t.Error("Parser() should return nil on fresh options")
	}
	if opts.GetRawSharded() != nil {
		t.Error("expected nil raw sharded on fresh options")
	}
	if opts.Sharded() {
		t.Error("Sharded() should return false on fresh options")
	}
	if opts.GetRawSubscriptionMode() != nil {
		t.Error("expected nil raw subscription mode on fresh options")
	}
	// Default when unset is DynamicSubscriptionMode.
	if opts.SubscriptionMode() != cache.DynamicSubscriptionMode {
		t.Errorf("SubscriptionMode(): got %q, want %q", opts.SubscriptionMode(), cache.DynamicSubscriptionMode)
	}
}

func TestEmitterOptions_Assign(t *testing.T) {
	src := DefaultEmitterOptions()
	src.SetKey("testkey")
	src.SetSharded(true)
	src.SetSubscriptionMode(cache.StaticSubscriptionMode)
	src.SetParser(utils.MsgPack())

	dst := DefaultEmitterOptions()
	dst.Assign(src)

	if dst.Key() != "testkey" {
		t.Errorf("Key(): got %q, want %q", dst.Key(), "testkey")
	}
	if !dst.Sharded() {
		t.Error("Sharded() should be true after Assign")
	}
	if dst.SubscriptionMode() != cache.StaticSubscriptionMode {
		t.Errorf("SubscriptionMode(): got %q, want %q", dst.SubscriptionMode(), cache.StaticSubscriptionMode)
	}
	if dst.Parser() == nil {
		t.Error("Parser() should not be nil after Assign")
	}
}

func TestEmitterOptions_AssignNil(t *testing.T) {
	opts := DefaultEmitterOptions()
	result := opts.Assign(nil)
	if result == nil {
		t.Error("Assign(nil) should return the options, not nil")
	}
}

func TestEmitterOptions_AssignDoesNotOverwriteWithUnset(t *testing.T) {
	dst := DefaultEmitterOptions()
	dst.SetKey("original")

	src := DefaultEmitterOptions() // key not set
	dst.Assign(src)

	if dst.Key() != "original" {
		t.Errorf("Assign with unset key should not overwrite; got %q", dst.Key())
	}
}
