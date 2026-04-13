package utils

import (
	"testing"
)

func TestIs(t *testing.T) {
	// Test with string
	if !Is[string]("test") {
		t.Error("Expected 'test' to be string")
	}
	if Is[string](123) {
		t.Error("Expected 123 to not be string")
	}

	// Test with int
	if !Is[int](42) {
		t.Error("Expected 42 to be int")
	}
	if Is[int]("test") {
		t.Error("Expected 'test' to not be int")
	}

	// Test with nil
	if Is[string](nil) {
		t.Error("Expected nil to not be string")
	}
}

func TestTryCast(t *testing.T) {
	// Test successful cast
	result := TryCast[string]("test")
	if result != "test" {
		t.Errorf("Expected 'test', got %q", result)
	}

	// Test failed cast (should return zero value)
	result = TryCast[string](123)
	if result != "" {
		t.Errorf("Expected empty string for failed cast, got %q", result)
	}

	// Test with int
	intResult := TryCast[int](42)
	if intResult != 42 {
		t.Errorf("Expected 42, got %d", intResult)
	}

	intResult = TryCast[int]("test")
	if intResult != 0 {
		t.Errorf("Expected 0 for failed cast, got %d", intResult)
	}
}

func TestPtr(t *testing.T) {
	// Test with int
	val := 42
	ptr := Ptr(val)
	if ptr == nil {
		t.Fatal("Expected non-nil pointer")
	}
	if *ptr != 42 {
		t.Errorf("Expected 42, got %d", *ptr)
	}

	// Test with string
	str := "hello"
	strPtr := Ptr(str)
	if strPtr == nil {
		t.Fatal("Expected non-nil pointer")
	}
	if *strPtr != "hello" {
		t.Errorf("Expected 'hello', got %q", *strPtr)
	}
}

func TestTap(t *testing.T) {
	// Test with callback
	val := 42
	called := false

	result := Tap(val, func(v int) {
		called = true
		if v != 42 {
			t.Errorf("Expected callback to receive 42, got %d", v)
		}
	})

	if !called {
		t.Error("Expected callback to be called")
	}
	if result != 42 {
		t.Errorf("Expected Tap to return 42, got %d", result)
	}

	// Test with nil callback
	result = Tap(val, nil)
	if result != 42 {
		t.Errorf("Expected Tap with nil callback to return 42, got %d", result)
	}
}

func TestValue(t *testing.T) {
	// Test with empty string
	result := Value("", "default")
	if result != "default" {
		t.Errorf("Expected 'default', got %q", result)
	}

	// Test with non-empty string
	result = Value("actual", "default")
	if result != "actual" {
		t.Errorf("Expected 'actual', got %q", result)
	}
}

func TestContains(t *testing.T) {
	// Test with match
	haystack := "The quick brown fox jumps over the lazy dog"
	needles := []string{"fox", "cat", "dog"}

	result := Contains(haystack, needles)
	if result != "fox" {
		t.Errorf("Expected 'fox', got %q", result)
	}

	// Test with no match
	needles = []string{"cat", "bird"}
	result = Contains(haystack, needles)
	if result != "" {
		t.Errorf("Expected empty string for no match, got %q", result)
	}

	// Test with empty needles
	result = Contains(haystack, []string{})
	if result != "" {
		t.Errorf("Expected empty string for empty needles, got %q", result)
	}

	// Test with empty needle strings
	result = Contains(haystack, []string{"", ""})
	if result != "" {
		t.Errorf("Expected empty string for empty needle strings, got %q", result)
	}
}

func TestMapValues(t *testing.T) {
	// Test with normal map
	input := map[string]int{"a": 1, "b": 2, "c": 3}

	result := MapValues(input, func(v int) string {
		return string(rune(v + 64)) // 1->A, 2->B, 3->C
	})

	if len(result) != 3 {
		t.Errorf("Expected 3 items, got %d", len(result))
	}
	if result["a"] != "A" {
		t.Errorf("Expected result['a'] to be 'A', got %q", result["a"])
	}
	if result["b"] != "B" {
		t.Errorf("Expected result['b'] to be 'B', got %q", result["b"])
	}
	if result["c"] != "C" {
		t.Errorf("Expected result['c'] to be 'C', got %q", result["c"])
	}

	// Test with nil map
	result = MapValues[string, int, string](nil, func(v int) string {
		return string(rune(v))
	})
	if result != nil {
		t.Errorf("Expected nil result for nil input, got %v", result)
	}
}

func TestStripHostPort(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com"},
		{"example.com:8080", "example.com"},
		{"localhost:3000", "localhost"},
		{"127.0.0.1:80", "127.0.0.1"},
		{"[::1]:8080", "::1"},
	}

	for _, tt := range tests {
		result := StripHostPort(tt.input)
		if result != tt.expected {
			t.Errorf("StripHostPort(%q) expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "/"},
		{"/", "/"},
		{"/foo/bar", "/foo/bar"},
		{"/foo/../bar", "/bar"},
		{"/foo/./bar", "/foo/bar"},
		{"/foo//bar", "/foo/bar"},
		{"foo/bar", "/foo/bar"},
		{"/foo/bar/", "/foo/bar/"},
	}

	for _, tt := range tests {
		result := CleanPath(tt.input)
		if result != tt.expected {
			t.Errorf("CleanPath(%q) expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestCheckInvalidHeaderChar(t *testing.T) {
	// Test valid headers
	if CheckInvalidHeaderChar("valid-header") {
		t.Error("Expected 'valid-header' to be valid")
	}
	if CheckInvalidHeaderChar("value with spaces") {
		t.Error("Expected 'value with spaces' to be valid")
	}
	if CheckInvalidHeaderChar("value\twith\ttabs") {
		t.Error("Expected 'value\twith\ttabs' to be valid")
	}

	// Test invalid headers (control characters)
	if !CheckInvalidHeaderChar("invalid\x00header") {
		t.Error("Expected 'invalid\\x00header' to be invalid")
	}
	if !CheckInvalidHeaderChar("invalid\nheader") {
		t.Error("Expected 'invalid\\nheader' to be invalid")
	}
	if !CheckInvalidHeaderChar("invalid\rheader") {
		t.Error("Expected 'invalid\\rheader' to be invalid")
	}
	if !CheckInvalidHeaderChar("invalid\x7fheader") {
		t.Error("Expected 'invalid\\x7fheader' to be invalid")
	}
}

func TestIsWithStructs(t *testing.T) {
	type MyStruct struct {
		Name string
	}

	s := MyStruct{Name: "test"}

	if !Is[MyStruct](s) {
		t.Error("Expected s to be MyStruct")
	}
	if Is[MyStruct]("test") {
		t.Error("Expected 'test' to not be MyStruct")
	}
}

func TestTryCastWithStructs(t *testing.T) {
	type MyStruct struct {
		Name string
	}

	s := MyStruct{Name: "test"}

	result := TryCast[MyStruct](s)
	if result.Name != "test" {
		t.Errorf("Expected Name to be 'test', got %q", result.Name)
	}

	// Failed cast
	result = TryCast[MyStruct]("test")
	if result.Name != "" {
		t.Errorf("Expected empty Name for failed cast, got %q", result.Name)
	}
}

func TestTapWithStructs(t *testing.T) {
	type Person struct {
		Name string
		Age  int
	}

	p := Person{Name: "Alice", Age: 30}

	result := Tap(p, func(person Person) {
		if person.Name != "Alice" {
			t.Errorf("Expected Name to be 'Alice', got %q", person.Name)
		}
	})

	if result.Name != "Alice" || result.Age != 30 {
		t.Errorf("Expected Person{Name: 'Alice', Age: 30}, got %+v", result)
	}
}
