package types

import (
	"reflect"
	"testing"
)

func TestSet(t *testing.T) {

	s := NewSet(1, 2, 3)

	if !s.Add(4, 5) || s.Add(4) {
		t.Error("Add should report whether the set changed")
	}
	expectedLen := 5
	if len := s.Len(); len != expectedLen {
		t.Errorf("Add method failed, expected length %d, got %d", expectedLen, len)
	}

	if !s.Delete(3) || s.Delete(3) {
		t.Error("Delete should report whether the set changed")
	}
	expectedLen = 4
	if len := s.Len(); len != expectedLen {
		t.Errorf("Delete method failed, expected length %d, got %d", expectedLen, len)
	}

	if !s.Clear() || s.Clear() {
		t.Error("Clear should report whether the set changed")
	}
	if len := s.Len(); len != 0 {
		t.Errorf("Clear method failed, expected length 0, got %d", len)
	}

	s.Add(1, 2, 3)
	tests := []struct {
		key      int
		expected bool
	}{
		{key: 1, expected: true},
		{key: 4, expected: false},
	}
	for _, test := range tests {
		if has := s.Has(test.key); has != test.expected {
			t.Errorf("Has method failed for key %d, expected %t, got %t", test.key, test.expected, has)
		}
	}

	expectedMap := map[int]Void{1: NULL, 2: NULL, 3: NULL}
	if all := s.All(); !reflect.DeepEqual(all, expectedMap) {
		t.Errorf("All method failed, expected %v, got %v", expectedMap, all)
	}
}

func TestSetZeroValue(t *testing.T) {
	var s Set[int]
	if !s.Add(1) || !s.Has(1) {
		t.Fatal("zero-value Set should support Add")
	}
	if !s.Delete(1) || s.Len() != 0 {
		t.Fatal("zero-value Set should support Delete")
	}
	if s.Clear() {
		t.Fatal("clearing an empty zero-value Set should report no change")
	}
}
