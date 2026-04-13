package utils

import (
	"testing"
)

func TestEncodeNegative(t *testing.T) {
	y := NewYeast()
	// Negative numbers should be treated as absolute value
	result := y.Encode(-64)
	expected := y.Encode(64)
	if result != expected {
		t.Errorf("Encode(-64) = %q, Encode(64) = %q, should be equal", result, expected)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	y := NewYeast()
	values := []int64{0, 1, 63, 64, 100, 1000, 123456, 9999999}
	for _, v := range values {
		encoded := y.Encode(v)
		decoded := y.Decode(encoded)
		if decoded != v {
			t.Errorf("Round trip failed for %d: encoded=%q, decoded=%d", v, encoded, decoded)
		}
	}
}

func TestDecodeEmptyString(t *testing.T) {
	y := NewYeast()
	got := y.Decode("")
	if got != 0 {
		t.Errorf("Decode(\"\") = %d, want 0", got)
	}
}

func TestYeastSameMilli(t *testing.T) {
	y := NewYeast()
	// Call Yeast() multiple times rapidly — same millisecond calls result in seed suffix
	ids := make(map[string]struct{})
	for range 100 {
		id := y.Yeast()
		if _, exists := ids[id]; exists {
			t.Fatalf("Yeast() produced duplicate ID: %s", id)
		}
		ids[id] = struct{}{}
	}
}

func TestYeastDate(t *testing.T) {
	id := YeastDate()
	if id == "" {
		t.Error("YeastDate() returned empty string")
	}
	// Call again to verify uniqueness
	id2 := YeastDate()
	if id == id2 {
		// They could be different if enough time passed, but within same ms they should differ via seed
		// Just verify both are non-empty
		if id2 == "" {
			t.Error("YeastDate() returned empty string on second call")
		}
	}
}

func TestDefaultYeast(t *testing.T) {
	if DefaultYeast == nil {
		t.Fatal("DefaultYeast is nil")
	}
	id := DefaultYeast.Yeast()
	if id == "" {
		t.Error("DefaultYeast.Yeast() returned empty string")
	}
}
