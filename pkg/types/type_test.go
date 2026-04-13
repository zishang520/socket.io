package types

import (
	"testing"
)

func TestIncomingHttpHeadersNil(t *testing.T) {
	var headers IncomingHttpHeaders

	result := headers.Header()
	if result != nil {
		t.Errorf("Expected nil for nil headers, got %v", result)
	}
}

func TestIncomingHttpHeadersEmpty(t *testing.T) {
	headers := IncomingHttpHeaders{}

	result := headers.Header()
	if len(result) != 0 {
		t.Errorf("Expected empty header map, got %v", result)
	}
}

func TestIncomingHttpHeadersString(t *testing.T) {
	headers := IncomingHttpHeaders{
		"content-type": "application/json",
		"accept":       "text/html",
	}

	result := headers.Header()

	if len(result) != 2 {
		t.Errorf("Expected 2 headers, got %d", len(result))
	}
	if result.Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %q", result.Get("Content-Type"))
	}
	if result.Get("Accept") != "text/html" {
		t.Errorf("Expected Accept 'text/html', got %q", result.Get("Accept"))
	}
}

func TestIncomingHttpHeadersStringSlice(t *testing.T) {
	headers := IncomingHttpHeaders{
		"accept": []string{"text/html", "application/xhtml+xml"},
	}

	result := headers.Header()

	if len(result["Accept"]) != 2 {
		t.Errorf("Expected 2 Accept values, got %d", len(result["Accept"]))
	}
	// Check both values are present
	if result["Accept"][0] != "text/html" || result["Accept"][1] != "application/xhtml+xml" {
		t.Errorf("Expected Accept values [text/html, application/xhtml+xml], got %v", result["Accept"])
	}
}

func TestIncomingHttpHeadersAnySlice(t *testing.T) {
	headers := IncomingHttpHeaders{
		"accept": []any{"text/html", "application/json"},
	}

	result := headers.Header()

	if len(result["Accept"]) != 2 {
		t.Errorf("Expected 2 Accept values, got %d", len(result["Accept"]))
	}
}

func TestIncomingHttpHeadersCanonicalCase(t *testing.T) {
	headers := IncomingHttpHeaders{
		"content-type":    "application/json",
		"CONTENT-TYPE":    "text/plain", // should overwrite due to canonical key
		"x-custom-header": "value",
	}

	result := headers.Header()

	// Due to map iteration order, we just check that canonical keys exist
	if result.Get("Content-Type") == "" {
		t.Error("Expected Content-Type to be set")
	}
}

func TestParsedUrlQueryNil(t *testing.T) {
	var query ParsedUrlQuery

	result := query.Query()
	if result != nil {
		t.Errorf("Expected nil for nil query, got %v", result)
	}
}

func TestParsedUrlQueryEmpty(t *testing.T) {
	query := ParsedUrlQuery{}

	result := query.Query()
	if len(result) != 0 {
		t.Errorf("Expected empty query values, got %v", result)
	}
}

func TestParsedUrlQueryString(t *testing.T) {
	query := ParsedUrlQuery{
		"name": "John",
		"age":  "30",
		"city": "New York",
	}

	result := query.Query()

	if len(result) != 3 {
		t.Errorf("Expected 3 query params, got %d", len(result))
	}
	if result.Get("name") != "John" {
		t.Errorf("Expected name 'John', got %q", result.Get("name"))
	}
	if result.Get("age") != "30" {
		t.Errorf("Expected age '30', got %q", result.Get("age"))
	}
}

func TestParsedUrlQueryStringSlice(t *testing.T) {
	query := ParsedUrlQuery{
		"tags": []string{"go", "testing", "unit"},
	}

	result := query.Query()

	if len(result["tags"]) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(result["tags"]))
	}
	// Check all values are present
	expected := []string{"go", "testing", "unit"}
	for i, v := range expected {
		if result["tags"][i] != v {
			t.Errorf("Expected tag[%d] to be %q, got %q", i, v, result["tags"][i])
		}
	}
}

func TestParsedUrlQueryAnySlice(t *testing.T) {
	query := ParsedUrlQuery{
		"ids": []any{"1", "2", "3"},
	}

	result := query.Query()

	if len(result["ids"]) != 3 {
		t.Errorf("Expected 3 ids, got %d", len(result["ids"]))
	}
}

func TestParsedUrlQueryMixedTypes(t *testing.T) {
	query := ParsedUrlQuery{
		"name": "John",
		"age":  30,
		"tags": []any{"admin", "user"},
	}

	result := query.Query()

	if result.Get("name") != "John" {
		t.Errorf("Expected name 'John', got %q", result.Get("name"))
	}
	if result.Get("age") != "30" {
		t.Errorf("Expected age '30', got %q", result.Get("age"))
	}
	if len(result["tags"]) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(result["tags"]))
	}
}

func TestConvertToStringSliceString(t *testing.T) {
	result := convertToStringSlice("test")
	if len(result) != 1 || result[0] != "test" {
		t.Errorf("Expected ['test'], got %v", result)
	}
}

func TestConvertToStringSliceStringSlice(t *testing.T) {
	result := convertToStringSlice([]string{"a", "b", "c"})
	if len(result) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(result))
	}
}

func TestConvertToStringSliceAnySlice(t *testing.T) {
	result := convertToStringSlice([]any{"a", 123, true})
	if len(result) != 3 {
		t.Errorf("Expected 3 elements, got %d", len(result))
	}
	if result[0] != "a" || result[1] != "123" || result[2] != "true" {
		t.Errorf("Expected [a, 123, true], got %v", result)
	}
}

func TestConvertAnyToStringInt(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{int(42), "42"},
		{int8(8), "8"},
		{int16(16), "16"},
		{int32(32), "32"},
		{int64(64), "64"},
	}

	for _, tt := range tests {
		result := convertAnyToString(tt.input)
		if result != tt.expected {
			t.Errorf("Expected %q for %T, got %q", tt.expected, tt.input, result)
		}
	}
}

func TestConvertAnyToStringUint(t *testing.T) {
	tests := []struct {
		input    any
		expected string
	}{
		{uint(42), "42"},
		{uint8(8), "8"},
		{uint16(16), "16"},
		{uint32(32), "32"},
		{uint64(64), "64"},
	}

	for _, tt := range tests {
		result := convertAnyToString(tt.input)
		if result != tt.expected {
			t.Errorf("Expected %q for %T, got %q", tt.expected, tt.input, result)
		}
	}
}

func TestConvertAnyToStringFloat(t *testing.T) {
	result := convertAnyToString(float64(3.14))
	if result != "3.14" {
		t.Errorf("Expected '3.14', got %q", result)
	}
}

func TestConvertAnyToStringBool(t *testing.T) {
	if result := convertAnyToString(true); result != "true" {
		t.Errorf("Expected 'true', got %q", result)
	}
	if result := convertAnyToString(false); result != "false" {
		t.Errorf("Expected 'false', got %q", result)
	}
}

func TestConvertAnyToStringNil(t *testing.T) {
	result := convertAnyToString(nil)
	if result != "" {
		t.Errorf("Expected empty string for nil, got %q", result)
	}
}

func TestVoidType(t *testing.T) {
	// Test that NULL is a zero-sized struct
	if NULL != (Void{}) {
		t.Error("Expected NULL to equal empty Void struct")
	}
}

func TestIncomingHttpHeadersWithIntValues(t *testing.T) {
	// Edge case: headers with non-string values that should be converted
	headers := IncomingHttpHeaders{
		"x-request-id": 12345,
	}

	result := headers.Header()
	if result.Get("X-Request-Id") != "12345" {
		t.Errorf("Expected X-Request-Id '12345', got %q", result.Get("X-Request-Id"))
	}
}

func TestParsedUrlQueryWithNumericStrings(t *testing.T) {
	query := ParsedUrlQuery{
		"price":  99.99,
		"count":  10,
		"active": true,
	}

	result := query.Query()

	if result.Get("price") != "99.99" {
		t.Errorf("Expected price '99.99', got %q", result.Get("price"))
	}
	if result.Get("count") != "10" {
		t.Errorf("Expected count '10', got %q", result.Get("count"))
	}
	if result.Get("active") != "true" {
		t.Errorf("Expected active 'true', got %q", result.Get("active"))
	}
}
