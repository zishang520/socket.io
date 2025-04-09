package errors

import (
	"testing"
)

func TestError(t *testing.T) {
	err := New("Test Error")
	t.Run("type", func(t *testing.T) {
		if err, ok := err.Err().(*Error); !ok {
			t.Fatalf(`err.(type) = %T, want match for *Error`, err)
		}
	})
	t.Run("message", func(t *testing.T) {
		if msg := err.Error(); msg != "Test Error" {
			t.Fatalf(`err = %q, want match for %#q`, err, "Test Error")
		}
	})
}
