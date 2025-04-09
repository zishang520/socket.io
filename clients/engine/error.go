package engine

import "context"

// Error represents a custom error type for Engine.IO transport errors.
// It provides detailed information about transport-related errors, including
// the error message, underlying cause, error type, and additional context.
//
// The Error type implements both the standard error interface and the
// errors.Unwrap interface, allowing it to work seamlessly with Go's error
// handling mechanisms.
type Error struct {
	// Message is a human-readable description of the error.
	// It provides a clear explanation of what went wrong.
	Message string

	// Description contains the underlying error that caused this transport error.
	// This can be nil if there is no underlying error.
	Description error

	// Type identifies the category of the error (e.g., "TransportError").
	// This helps in error classification and handling.
	Type string

	// Context contains additional context information about the error.
	// This can include request/response data, timing information, etc.
	Context context.Context

	// errs contains a slice of underlying errors that contributed to this error.
	// This supports error wrapping and error chain inspection.
	errs []error
}

// Err returns the error interface implementation.
// This method allows the Error type to be used as a standard error.
//
// Returns:
//   - error: The error interface implementation
func (e *Error) Err() error {
	return e
}

// Error implements the error interface.
// It returns a human-readable error message describing what went wrong.
//
// Returns:
//   - string: The error message
func (e *Error) Error() string {
	return e.Message
}

// Unwrap returns the slice of underlying errors.
// This implements the errors.Unwrap interface for error chain inspection.
// It allows the Error type to work with Go's error wrapping mechanisms
// like errors.Is and errors.As.
//
// Returns:
//   - []error: The slice of underlying errors
func (e *Error) Unwrap() []error {
	return e.errs
}

// NewTransportError creates a new transport error with the specified details.
// This is a convenience function for creating transport-specific errors with
// appropriate context and type information.
//
// Parameters:
//   - reason: A human-readable description of the error
//   - description: The underlying error that caused this transport error (can be nil)
//   - context: Additional context information about the error (can be nil)
//
// Returns:
//   - *Error: A new Error instance configured as a transport error
//
// Example:
//
//	err := NewTransportError(
//	    "connection timeout",
//	    errors.New("read deadline exceeded"),
//	    ctx,
//	)
func NewTransportError(reason string, description error, context context.Context) *Error {
	return &Error{
		Message:     reason,
		Description: description,
		Type:        "TransportError",
		Context:     context,
		errs:        []error{description},
	}
}
