package types

type (
	CodeMessage struct {
		Code    int    `json:"code" msgpack:"code"`
		Message string `json:"message,omitempty" msgpack:"message,omitempty"`
	}

	ErrorMessage struct {
		*CodeMessage

		Req     *HttpContext   `json:"req,omitempty" msgpack:"req,omitempty"`
		Context map[string]any `json:"context,omitempty" msgpack:"context,omitempty"`
	}

	// ExtendedError represents an error with an associated message and additional data.
	// This type is used across both client and server Socket.IO implementations to provide
	// structured error information, particularly for connection/middleware errors.
	ExtendedError struct {
		Message string `json:"message" msgpack:"message"` // Error message
		Data    any    `json:"data" msgpack:"data"`       // Additional error data
	}
)

// NewExtendedError creates a new ExtendedError with the given message and data.
func NewExtendedError(message string, data any) *ExtendedError {
	return &ExtendedError{Message: message, Data: data}
}

// Err returns the error interface implementation, allowing ExtendedError to be used as an error.
func (e *ExtendedError) Err() error {
	return e
}

// Error implements the error interface, returning the error message.
func (e *ExtendedError) Error() string {
	return e.Message
}
