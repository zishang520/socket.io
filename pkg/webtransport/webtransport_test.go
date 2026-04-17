package webtransport

import (
	"testing"
)

// Test close code constants
func TestCloseCodeConstants(t *testing.T) {
	tests := []struct {
		name string
		code int
	}{
		{"CloseNormalClosure", CloseNormalClosure},
		{"CloseGoingAway", CloseGoingAway},
		{"CloseProtocolError", CloseProtocolError},
		{"CloseUnsupportedData", CloseUnsupportedData},
		{"CloseNoStatusReceived", CloseNoStatusReceived},
		{"CloseAbnormalClosure", CloseAbnormalClosure},
		{"CloseInvalidFramePayloadData", CloseInvalidFramePayloadData},
		{"ClosePolicyViolation", ClosePolicyViolation},
		{"CloseMessageTooBig", CloseMessageTooBig},
		{"CloseMandatoryExtension", CloseMandatoryExtension},
		{"CloseInternalServerErr", CloseInternalServerErr},
		{"CloseServiceRestart", CloseServiceRestart},
		{"CloseTryAgainLater", CloseTryAgainLater},
		{"CloseTLSHandshake", CloseTLSHandshake},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.code < 1000 || tt.code > 1015 {
				t.Errorf("Close code %d out of expected range", tt.code)
			}
		})
	}

	// Test ServiceRestart and TryAgainLater are in valid range
	if CloseServiceRestart != 1012 {
		t.Errorf("Expected CloseServiceRestart to be 1012, got %d", CloseServiceRestart)
	}
	if CloseTryAgainLater != 1013 {
		t.Errorf("Expected CloseTryAgainLater to be 1013, got %d", CloseTryAgainLater)
	}
}

// Test message type constants
func TestMessageConstants(t *testing.T) {
	if TextMessage != 1 {
		t.Errorf("Expected TextMessage to be 1, got %d", TextMessage)
	}
	if BinaryMessage != 2 {
		t.Errorf("Expected BinaryMessage to be 2, got %d", BinaryMessage)
	}
}

// Test CloseError
func TestCloseError(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		text     string
		expected string
	}{
		{
			name:     "normal closure",
			code:     CloseNormalClosure,
			text:     "",
			expected: "webtransport: close 1000 (normal)",
		},
		{
			name:     "going away",
			code:     CloseGoingAway,
			text:     "",
			expected: "webtransport: close 1001 (going away)",
		},
		{
			name:     "protocol error",
			code:     CloseProtocolError,
			text:     "",
			expected: "webtransport: close 1002 (protocol error)",
		},
		{
			name:     "custom code",
			code:     4000,
			text:     "",
			expected: "webtransport: close 4000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &CloseError{
				Code: tt.code,
				Text: tt.text,
			}

			result := err.Error()
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Test CloseError with text
func TestCloseErrorWithText(t *testing.T) {
	err := &CloseError{
		Code: CloseNormalClosure,
		Text: "Goodbye!",
	}

	result := err.Error()
	expected := "webtransport: close 1000 (normal): Goodbye!"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

// Test IsCloseError helper
func TestIsCloseError(t *testing.T) {
	err := &CloseError{
		Code: CloseNormalClosure,
	}

	if !IsCloseError(err, CloseNormalClosure) {
		t.Error("Expected IsCloseError to return true")
	}

	if IsCloseError(err, CloseGoingAway) {
		t.Error("Expected IsCloseError to return false for different code")
	}

	// Test with non-CloseError
	regularErr := &netError{msg: "some error"}
	if IsCloseError(regularErr, CloseNormalClosure) {
		t.Error("Expected IsCloseError to return false for non-CloseError")
	}

	// Test with nil
	if IsCloseError(nil, CloseNormalClosure) {
		t.Error("Expected IsCloseError to return false for nil")
	}
}

// Test netError
func TestNetError(t *testing.T) {
	err := &netError{
		msg:       "test error",
		temporary: true,
		timeout:   false,
	}

	if err.Error() != "test error" {
		t.Errorf("Expected 'test error', got %q", err.Error())
	}
	if !err.Temporary() {
		t.Error("Expected Temporary() to return true")
	}
	if err.Timeout() {
		t.Error("Expected Timeout() to return false")
	}
}

// Test error constants exist
func TestErrorConstants(t *testing.T) {
	if ErrCloseSent == nil {
		t.Error("Expected ErrCloseSent to be defined")
	}
	if ErrReadLimit == nil {
		t.Error("Expected ErrReadLimit to be defined")
	}
}
