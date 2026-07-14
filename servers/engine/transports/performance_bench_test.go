package transports

import (
	"bytes"
	"testing"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

func BenchmarkTransportSetReadyState(b *testing.B) {
	transport := MakeTransport()

	b.ReportAllocs()
	for b.Loop() {
		transport.SetReadyState("open")
	}
}

func BenchmarkJSONPResponseAssembly(b *testing.B) {
	head := "___eio[12]("
	payload := bytes.Repeat([]byte(`"engine.io payload"`), 32)
	foot := ");"

	b.ReportAllocs()
	for b.Loop() {
		response := types.NewStringBuffer(make([]byte, 0, len(head)+len(payload)+len(foot)))
		_, _ = response.WriteString(head)
		_, _ = response.Write(payload)
		_, _ = response.WriteString(foot)
		if response.Len() == 0 {
			b.Fatal("empty JSONP response")
		}
	}
}
