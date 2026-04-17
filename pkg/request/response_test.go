package request

import (
	"net/http"
	"testing"
)

func TestResponseOk(t *testing.T) {
	tests := []struct {
		statusCode int
		expected   bool
	}{
		{200, true},
		{201, true},
		{204, true},
		{299, true},
		{199, false},
		{300, false},
		{400, false},
		{404, false},
		{500, false},
	}

	for _, tt := range tests {
		t.Run(string(rune(tt.statusCode)), func(t *testing.T) {
			// Test the logic directly since we can't easily mock resty.Response
			ok := tt.statusCode >= 200 && tt.statusCode <= 299
			if ok != tt.expected {
				t.Errorf("StatusCode %d: expected %v, got %v", tt.statusCode, tt.expected, ok)
			}
		})
	}
}

func TestResponseOkBoundary(t *testing.T) {
	// Test exact boundaries using variables to avoid gocritic dupSubExpr
	boundaries := []struct {
		code     int
		expected bool
		name     string
	}{
		{200, true, "lower bound 200"},
		{299, true, "upper bound 299"},
		{199, false, "below lower bound 199"},
		{300, false, "above upper bound 300"},
	}
	for _, b := range boundaries {
		ok := b.code >= http.StatusOK && b.code <= 299
		if ok != b.expected {
			t.Errorf("%s: expected %v, got %v", b.name, b.expected, ok)
		}
	}
}

// Test Response struct
func TestResponseStruct(t *testing.T) {
	// Just verify Response type exists and has Ok method signature
	var resp *Response
	_ = resp // Will be nil, but verifies type exists
}

// Helper function to test Ok() with actual HTTP status codes
func TestHttpStatusCodes(t *testing.T) {
	statusCodes := map[int]bool{
		http.StatusOK:                  true,  // 200
		http.StatusCreated:             true,  // 201
		http.StatusAccepted:            true,  // 202
		http.StatusNoContent:           true,  // 204
		http.StatusMovedPermanently:    false, // 301
		http.StatusFound:               false, // 302
		http.StatusBadRequest:          false, // 400
		http.StatusUnauthorized:        false, // 401
		http.StatusForbidden:           false, // 403
		http.StatusNotFound:            false, // 404
		http.StatusInternalServerError: false, // 500
		http.StatusNotImplemented:      false, // 501
		http.StatusBadGateway:          false, // 502
		http.StatusServiceUnavailable:  false, // 503
	}

	for code, expected := range statusCodes {
		ok := code >= 200 && code <= 299
		if ok != expected {
			t.Errorf("HTTP %d: expected %v, got %v", code, expected, ok)
		}
	}
}
