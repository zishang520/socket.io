// FILEPATH: /e:/go-obj/socket.io/servers/engine/errors/errors_test.go
package errors

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestTransportErrorChainLength(t *testing.T) {
	// Create a deeply nested error chain
	baseErr := errors.New("base error")
	level1Err := fmt.Errorf("level 1: %w", baseErr)
	level2Err := fmt.Errorf("level 2: %w", level1Err)
	deeply_nested_error := fmt.Errorf("level 3: %w", level2Err)

	// Create transport error with the nested error chain
	transportErr := NewTransportError("Test", deeply_nested_error)

	// Test cases
	t.Run("error chain completeness", func(t *testing.T) {
		// Convert error to string to check content
		errString := transportErr.Error()

		// Check if all parts of the error chain are present
		expectedParts := []string{
			"transport error", // From ErrTransportFailure
			"Test",            // reason
			"level 3",         // From deeply_nested_error
			"level 2",         // From level2Err
			"level 1",         // From level1Err
			"base error",      // From baseErr
		}

		for _, part := range expectedParts {
			if !strings.Contains(errString, part) {
				t.Errorf("Error chain missing expected part: %s\nGot error: %s", part, errString)
			}
		}

		// Verify that we can unwrap the error chain
		var transportFailure *Error
		if !errors.As(transportErr, &transportFailure) {
			t.Error("Expected to be able to unwrap to *Error type")
		}

		// Verify the complete error chain is preserved
		currentErr := transportErr
		errorCount := 0
		for currentErr != nil {
			errorCount++
			currentErr = errors.Unwrap(currentErr)
		}

		// Expected number of errors in chain:
		// 1. Transport error wrapper
		// 2. deeply_nested_error (level 3)
		// 3. level2Err
		// 4. level1Err
		// 5. baseErr
		expectedCount := 5
		if errorCount != expectedCount {
			t.Errorf("Expected error chain length of %d, got %d", expectedCount, errorCount)
		}
	})
}

func TestNewTransportErrorFormatPreservation(t *testing.T) {
	// Arrange
	reason := "Test"
	description := errors.New("error")

	// Act
	err := NewTransportError(reason, description)

	// Assert
	if err == nil {
		t.Fatal("Expected error to not be nil")
	}

	expectedFormat := "transport error: Test (error)"
	actualError := err.Error()

	if !strings.Contains(actualError, expectedFormat) {
		t.Errorf("Error format not preserved.\nExpected to contain: %s\nGot: %s",
			expectedFormat, actualError)
	}

	// Verify that the error wraps ErrTransportFailure
	if !errors.Is(err, ErrTransportFailure) {
		t.Error("Error should wrap ErrTransportFailure")
	}

	// Verify that the error contains the description
	if !errors.Is(err, description) {
		t.Error("Error should wrap the description error")
	}
}
