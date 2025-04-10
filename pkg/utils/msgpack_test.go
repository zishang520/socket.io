package utils

import (
	"bytes"
	"testing"
)

func TestMsgPack(t *testing.T) {
	pack := MsgPack()

	t.Run("Encode/Decode", func(t *testing.T) {
		data, err := pack.Encode([]any{[]byte{1, 2, 3, 4}, 0, 4, nil})
		if err != nil {
			t.Fatal("Encode error must be nil")
		}
		check := []byte{148, 196, 4, 1, 2, 3, 4, 0, 4, 192}
		if !bytes.Equal(data, check) {
			t.Fatalf(`Encode value not as expected: %v, want match for %v`, data, check)
		}
		var value any
		err = pack.Decode(data, &value)
		if err != nil {
			t.Fatal("Decode error must be nil")
		}
		if d, ok := value.([]any); !ok {
			t.Fatal("Decode value must be []any")
		} else {
			if n := len(d); n != 4 {
				t.Fatalf(`Decode len(value) not as expected: %v, want match for %v`, 4, n)
			}
			if d[3] != nil {
				t.Fatalf(`Decode value[3] not as expected: %v, want match for %v`, nil, d[3])
			}
		}
	})
}
