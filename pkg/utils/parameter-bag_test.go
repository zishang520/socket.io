package utils

import (
	"testing"
)

func TestParameterBag(t *testing.T) {
	bag := NewParameterBag(nil)

	bag.Add("key1", "value1")
	bag.Add("key1", "value2")
	bag.Add("key2", "value3")

	value, ok := bag.Get("key1")
	if !ok || value != "value2" {
		t.Errorf("Expected value2, got %s", value)
	}

	firstValue, ok := bag.GetFirst("key1")
	if !ok || firstValue != "value1" {
		t.Errorf("Expected value1, got %s", firstValue)
	}

	lastValue, ok := bag.GetLast("key1")
	if !ok || lastValue != "value2" {
		t.Errorf("Expected value2, got %s", lastValue)
	}

	values, ok := bag.Gets("key1")
	if !ok || len(values) != 2 || values[0] != "value1" || values[1] != "value2" {
		t.Errorf("Expected [value1 value2], got %v", values)
	}

	bag.Set("key1", "value4")
	value, ok = bag.Get("key1")
	if !ok || value != "value4" {
		t.Errorf("Expected value4, got %s", value)
	}

	bag.Replace(map[string][]string{
		"key3": {"value5"},
	})
	value, ok = bag.Get("key3")
	if !ok || value != "value5" {
		t.Errorf("Expected value5, got %s", value)
	}

	bag.With(map[string][]string{
		"key3": {"value6"},
	})
	value, ok = bag.Get("key3")
	if !ok || value != "value6" {
		t.Errorf("Expected value6, got %s", value)
	}

	bag.Remove("key3")
	_, ok = bag.Get("key3")
	if ok {
		t.Errorf("Expected key3 to be removed")
	}

	count := bag.Count()
	if count != 0 {
		t.Errorf("Expected 0, got %d", count)
	}
}
