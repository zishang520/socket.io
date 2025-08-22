package types

import (
	"testing"
)

// Test NewSlice and Push method
func TestNewSliceAndPush(t *testing.T) {
	s := NewSlice(1, 2, 3)
	if s.Len() != 3 {
		t.Errorf("expected length 3, got %d", s.Len())
	}

	s.Push(4, 5)
	if s.Len() != 5 {
		t.Errorf("expected length 5, got %d", s.Len())
	}
}

// Test Pop method
func TestPop(t *testing.T) {
	s := NewSlice(1, 2, 3)
	element, err := s.Pop()
	if err != nil || element != 3 {
		t.Errorf("expected 3, got %v", element)
	}
	if s.Len() != 2 {
		t.Errorf("expected length 2, got %d", s.Len())
	}
}

// Test Unshift method
func TestUnshift(t *testing.T) {
	s := NewSlice(1, 2, 3)
	s.Unshift(0)
	if s.elements[0] != 0 {
		t.Errorf("expected 0 at index 0, got %d", s.elements[0])
	}
}

// Test Shift method
func TestShift(t *testing.T) {
	s := NewSlice(1, 2, 3)
	element, err := s.Shift()
	if err != nil || element != 1 {
		t.Errorf("expected 1, got %v", element)
	}
	if s.Len() != 2 {
		t.Errorf("expected length 2, got %d", s.Len())
	}
}

// Test Get method
func TestGet(t *testing.T) {
	s := NewSlice(1, 2, 3)
	element, err := s.Get(1)
	if err != nil || element != 2 {
		t.Errorf("expected 2, got %v", element)
	}
}

// Test Set method
func TestSliceSet(t *testing.T) {
	s := NewSlice(1, 2, 3)
	err := s.Set(1, 5)
	if err != nil || s.elements[1] != 5 {
		t.Errorf("expected 5 at index 1, got %d", s.elements[1])
	}
}

// Test Slice method
func TestSlice(t *testing.T) {
	s := NewSlice(1, 2, 3, 4)
	subSlice, _ := s.Slice(1, 3)
	if len(subSlice) != 2 || subSlice[0] != 2 || subSlice[1] != 3 {
		t.Errorf("expected [2 3], got %v", subSlice)
	}
}

// Test Splice method
func TestSplice(t *testing.T) {
	s := NewSlice(1, 2, 3, 4)
	removed, _ := s.Splice(1, 2, 5, 6)
	if len(removed) != 2 || removed[0] != 2 || removed[1] != 3 {
		t.Errorf("expected [2 3], got %v", removed)
	}
	if len(s.elements) != 4 || s.elements[1] != 5 || s.elements[2] != 6 {
		t.Errorf("expected [1 5 6 4], got %v", s.elements)
	}
}

// Test Remove method
func TestRemove(t *testing.T) {
	s := NewSlice(1, 2, 3, 4)
	s.Remove(func(element int) bool {
		return element%2 == 0
	})
	if len(s.elements) != 3 || s.elements[0] != 1 || s.elements[1] != 3 {
		t.Errorf("expected [1 3 4], got %v", s.elements)
	}
}

// Test Range method
func TestRange(t *testing.T) {
	s := NewSlice(1, 2, 3)
	sum := 0
	s.Range(func(element, index int) bool {
		sum += element
		return true
	})
	if sum != 6 {
		t.Errorf("expected sum 6, got %d", sum)
	}
}

// Test All method
func TestAll(t *testing.T) {
	s := NewSlice(1, 2, 3)
	values := s.All()
	if len(values) != 3 || values[0] != 1 || values[1] != 2 || values[2] != 3 {
		t.Errorf("expected [1 2 3], got %v", values)
	}
}

// Test AllAndClear method
func TestAllAndClear(t *testing.T) {
	s := NewSlice(1, 2, 3)
	flushed := s.AllAndClear()
	if len(flushed) != 3 || s.Len() != 0 {
		t.Errorf("flush failed, got %v", flushed)
	}
}

// Test Clear method
func TestClear(t *testing.T) {
	s := NewSlice(1, 2, 3)
	s.Clear()
	if s.Len() != 0 {
		t.Errorf("clear failed, len %d", s.Len())
	}
}
