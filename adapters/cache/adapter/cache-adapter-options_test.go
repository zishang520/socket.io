package adapter

import (
	"testing"
	"time"
)

func TestCacheAdapterOptions_Defaults(t *testing.T) {
	opts := DefaultCacheAdapterOptions()
	if opts.GetRawRequestsTimeout() != nil {
		t.Error("expected nil raw requests timeout on fresh options")
	}
	if opts.RequestsTimeout() != 0 {
		t.Errorf("expected zero timeout, got %v", opts.RequestsTimeout())
	}
	if opts.PublishOnSpecificResponseChannel() != false {
		t.Error("expected false PublishOnSpecificResponseChannel on fresh options")
	}
}

func TestCacheAdapterOptions_Assign(t *testing.T) {
	src := DefaultCacheAdapterOptions()
	src.SetRequestsTimeout(3 * time.Second)
	src.SetPublishOnSpecificResponseChannel(true)
	src.SetKey("custom-key")

	dst := DefaultCacheAdapterOptions()
	dst.Assign(src)

	if dst.RequestsTimeout() != 3*time.Second {
		t.Errorf("RequestsTimeout: got %v, want 3s", dst.RequestsTimeout())
	}
	if !dst.PublishOnSpecificResponseChannel() {
		t.Error("PublishOnSpecificResponseChannel should be true after Assign")
	}
	if dst.Key() != "custom-key" {
		t.Errorf("Key: got %q, want %q", dst.Key(), "custom-key")
	}
}

func TestCacheAdapterOptions_AssignNil(t *testing.T) {
	opts := DefaultCacheAdapterOptions()
	result := opts.Assign(nil)
	if result == nil {
		t.Error("Assign(nil) should return the options, not nil")
	}
}
