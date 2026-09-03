package errors

import (
	"errors"
	"testing"
)

func TestNewTransportErrorWrapsSentinels(t *testing.T) {
	description := errors.New("connection reset")
	err := NewTransportError("polling", description)

	if !errors.Is(err, ErrTransportFailure) {
		t.Fatal("expected transport failure sentinel")
	}
	if !errors.Is(err, description) {
		t.Fatal("expected description error to be wrapped")
	}
}
