package types

import (
	"testing"
)

func TestNewSome(t *testing.T) {
	// Test with string
	s := NewSome("hello")
	if !s.IsPresent() {
		t.Error("Expected Some to be present")
	}
	if s.IsEmpty() {
		t.Error("Expected Some to not be empty")
	}
	if v := s.Get(); v != "hello" {
		t.Errorf("Expected 'hello', got %q", v)
	}
}

func TestSomeInt(t *testing.T) {
	// Test with int
	s := NewSome(42)
	if !s.IsPresent() {
		t.Error("Expected Some to be present")
	}
	if v := s.Get(); v != 42 {
		t.Errorf("Expected 42, got %d", v)
	}
}

func TestSomeStruct(t *testing.T) {
	// Test with struct
	type Person struct {
		Name string
		Age  int
	}

	p := Person{Name: "Alice", Age: 30}
	s := NewSome(p)

	if !s.IsPresent() {
		t.Error("Expected Some to be present")
	}
	if v := s.Get(); v != p {
		t.Errorf("Expected %+v, got %+v", p, v)
	}
}

func TestSomeNilGet(t *testing.T) {
	// Test calling Get on nil Some
	var s *Some[string]

	if s.IsPresent() {
		t.Error("Expected nil Some to not be present")
	}
	if !s.IsEmpty() {
		t.Error("Expected nil Some to be empty")
	}

	// Get should return zero value
	if v := s.Get(); v != "" {
		t.Errorf("Expected empty string for nil Some, got %q", v)
	}
}

func TestSomeNilGetInt(t *testing.T) {
	// Test calling Get on nil Some with int
	var s *Some[int]

	if v := s.Get(); v != 0 {
		t.Errorf("Expected 0 for nil Some[int], got %d", v)
	}
}

func TestSomeNilGetStruct(t *testing.T) {
	// Test calling Get on nil Some with struct
	type Point struct {
		X, Y int
	}

	var s *Some[Point]
	if v := s.Get(); v != (Point{}) {
		t.Errorf("Expected zero Point for nil Some[Point], got %+v", v)
	}
}

func TestOptionalInterface(t *testing.T) {
	// Test that Some implements Optional interface
	var opt = NewSome("test")

	if !opt.IsPresent() {
		t.Error("Expected Optional to be present")
	}
	if opt.IsEmpty() {
		t.Error("Expected Optional to not be empty")
	}
	if v := opt.Get(); v != "test" {
		t.Errorf("Expected 'test', got %q", v)
	}
}

func TestOptionalIntInterface(t *testing.T) {
	var opt = NewSome(123)

	if v := opt.Get(); v != 123 {
		t.Errorf("Expected 123, got %d", v)
	}
}
