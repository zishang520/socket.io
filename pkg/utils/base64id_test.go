package utils

import (
	"fmt"
	"testing"
)

func ExampleBase64Id() {
	_, err := Base64Id().GenerateId()
	fmt.Println(err)

	// Output:
	// <nil>
}

func TestBase64Id(t *testing.T) {
	t.Run("GenerateId", func(t *testing.T) {
		if _, err := Base64Id().GenerateId(); err != nil {
			t.Fatalf(`Base64Id().GenerateId() = %t, want match for <nil>`, err)
		}
	})
}
