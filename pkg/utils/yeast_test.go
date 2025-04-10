package utils

import (
	"testing"
	"time"
)

// TestEncode tests the Encode method of the Yeast struct.
func TestEncode(t *testing.T) {
	y := NewYeast()

	tests := []struct {
		number   int64
		expected string
	}{
		{0, "0"},
		{1, "1"},
		{62, "-"},
		{63, "_"},
		{64, "10"},
		{123456, "U90"},
	}

	for _, test := range tests {
		result := y.Encode(test.number)
		if result != test.expected {
			t.Errorf("Encode(%d) = %s; expected %s", test.number, result, test.expected)
		}
	}
}

// TestDecode tests the Decode method of the Yeast struct.
func TestDecode(t *testing.T) {
	y := NewYeast()

	tests := []struct {
		str      string
		expected int64
	}{
		{"0", 0},
		{"1", 1},
		{"-", 62},
		{"_", 63},
		{"10", 64},
		{"W7E", 131534},
	}

	for _, test := range tests {
		result := y.Decode(test.str)
		if result != test.expected {
			t.Errorf("Decode(%s) = %d; expected %d", test.str, result, test.expected)
		}
	}
}

// TestYeast tests the Yeast method of the Yeast struct.
func TestYeast(t *testing.T) {
	y := NewYeast()

	// Generate multiple YEAST IDs to ensure uniqueness and correctness
	id1 := y.Yeast()
	id2 := y.Yeast()

	if id1 == id2 {
		t.Errorf("Yeast() generated two identical IDs: %s and %s", id1, id2)
	}

	// Add a short delay to ensure different millisecond timestamp
	time.Sleep(1 * time.Millisecond)

	id3 := y.Yeast()
	if id1 == id3 || id2 == id3 {
		t.Errorf("Yeast() generated a duplicate ID: %s", id3)
	}
}
