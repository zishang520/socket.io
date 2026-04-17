package types

import (
	"testing"
)

func TestExtendedError(t *testing.T) {
	// Test NewExtendedError
	err := NewExtendedError("test error", map[string]string{"key": "value"})

	if err.Message != "test error" {
		t.Errorf("Expected message 'test error', got %q", err.Message)
	}
	if err.Data == nil {
		t.Error("Expected data to be set")
	}
}

func TestExtendedErrorError(t *testing.T) {
	err := NewExtendedError("something went wrong", nil)

	// Test Error() method
	if err.Error() != "something went wrong" {
		t.Errorf("Expected Error() to return 'something went wrong', got %q", err.Error())
	}
}

func TestExtendedErrorErr(t *testing.T) {
	err := NewExtendedError("error with data", "some data")

	// Test Err() method returns error interface
	e := err.Err()
	if e == nil {
		t.Error("Expected Err() to return non-nil error")
	}
	if e.Error() != "error with data" {
		t.Errorf("Expected error message 'error with data', got %q", e.Error())
	}
}

func TestExtendedErrorWithData(t *testing.T) {
	// Test with different data types
	err1 := NewExtendedError("string data", "payload")
	if err1.Data != "payload" {
		t.Errorf("Expected string data 'payload', got %v", err1.Data)
	}

	err2 := NewExtendedError("int data", 42)
	if err2.Data != 42 {
		t.Errorf("Expected int data 42, got %v", err2.Data)
	}

	err3 := NewExtendedError("nil data", nil)
	if err3.Data != nil {
		t.Errorf("Expected nil data, got %v", err3.Data)
	}
}

func TestCodeMessage(t *testing.T) {
	cm := &CodeMessage{
		Code:    404,
		Message: "Not Found",
	}

	if cm.Code != 404 {
		t.Errorf("Expected code 404, got %d", cm.Code)
	}
	if cm.Message != "Not Found" {
		t.Errorf("Expected message 'Not Found', got %q", cm.Message)
	}
}

func TestCodeMessageEmpty(t *testing.T) {
	cm := &CodeMessage{
		Code: 500,
	}

	if cm.Code != 500 {
		t.Errorf("Expected code 500, got %d", cm.Code)
	}
	if cm.Message != "" {
		t.Errorf("Expected empty message, got %q", cm.Message)
	}
}

func TestErrorMessage(t *testing.T) {
	em := &ErrorMessage{
		CodeMessage: &CodeMessage{
			Code:    400,
			Message: "Bad Request",
		},
		Context: map[string]any{
			"field":  "username",
			"reason": "invalid format",
		},
	}

	if em.Code != 400 {
		t.Errorf("Expected code 400, got %d", em.Code)
	}
	if em.Message != "Bad Request" {
		t.Errorf("Expected message 'Bad Request', got %q", em.Message)
	}
	if em.Context["field"] != "username" {
		t.Errorf("Expected context field 'username', got %v", em.Context["field"])
	}
}

func TestErrorMessageNilContext(t *testing.T) {
	em := &ErrorMessage{
		CodeMessage: &CodeMessage{
			Code:    500,
			Message: "Internal Server Error",
		},
	}

	if em.Code != 500 {
		t.Errorf("Expected code 500, got %d", em.Code)
	}
	if em.Context != nil {
		t.Errorf("Expected nil context, got %v", em.Context)
	}
}

func TestExtendedErrorImplementsError(t *testing.T) {
	// Verify ExtendedError implements error interface
	var _ error = (*ExtendedError)(nil)

	err := NewExtendedError("test", nil)
	var e error = err

	if e.Error() != "test" {
		t.Errorf("Expected error message 'test', got %q", e.Error())
	}
}
