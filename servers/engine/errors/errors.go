package errors

import (
	"errors"
	"fmt"
)

var (
	ErrTransportFailure = errors.New("transport failure")

	ErrUnsupportedTransport    = errors.New("unsupported transport name")
	ErrTransportNotImplemented = errors.New("transport creation not implemented")
	ErrInvalidHeartbeat        = errors.New("invalid heartbeat direction")
)

func NewTransportError(reason string, description error) error {
	return fmt.Errorf("%w: %s (%w)", ErrTransportFailure, reason, description)
}
