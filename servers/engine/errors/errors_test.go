package errors

import (
	stderrors "errors"
	"testing"
)

func TestNewTransportErrorWrapsSentinels(t *testing.T) {
	description := stderrors.New("connection reset")
	err := NewTransportError("polling", description)

	if !stderrors.Is(err, ErrTransportFailure) {
		t.Fatal("expected transport failure sentinel")
	}
	if !stderrors.Is(err, description) {
		t.Fatal("expected description error to be wrapped")
	}
}
