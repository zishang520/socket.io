package socket

import (
	"fmt"
	"testing"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

func BenchmarkBroadcastPreparedFrameCache(b *testing.B) {
	for _, recipients := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("Cached/N%d", recipients), func(b *testing.B) {
			payload := types.NewStringBufferString("42/test,[\"event\",\"payload\"]")
			b.ReportAllocs()
			for range b.N {
				frame := newBroadcastFrame(payload.Clone()).(preparedFrameForTest)
				for range recipients {
					_, err := frame.PreparedWebSocketFrame(func(data types.BufferInterface) (any, error) {
						return append([]byte(nil), data.Bytes()...), nil
					})
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		})
		b.Run(fmt.Sprintf("Uncached/N%d", recipients), func(b *testing.B) {
			payload := types.NewStringBufferString("42/test,[\"event\",\"payload\"]")
			b.ReportAllocs()
			for range b.N {
				frame := payload.Clone()
				for range recipients {
					_ = append([]byte(nil), frame.Bytes()...)
				}
			}
		})
	}
}
