package engine

import (
	"encoding/json"
	"testing"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func BenchmarkApplyMiddlewares(b *testing.B) {
	server := MakeBaseServer()
	for range 3 {
		server.Use(func(_ *types.HttpContext, next func(error)) {
			next(nil)
		})
	}
	callback := func(error) {}

	b.ReportAllocs()
	for b.Loop() {
		server.ApplyMiddlewares(nil, callback)
	}
}

func BenchmarkSocketSetReadyState(b *testing.B) {
	socket := MakeSocket()

	b.ReportAllocs()
	for b.Loop() {
		socket.SetReadyState("open")
	}
}

func BenchmarkOpenPacketJSON(b *testing.B) {
	upgrades := []string{"websocket", "webtransport"}

	b.ReportAllocs()
	for b.Loop() {
		if _, err := json.Marshal(openPacket{
			Sid:          "lv_VI97HAXpY6yYWAAAC",
			Upgrades:     upgrades,
			PingInterval: 25_000,
			PingTimeout:  20_000,
			MaxPayload:   1_000_000,
		}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkSendPacket(b *testing.B) {
	socket := MakeSocket().(*socket)
	socket.readyState.Store("open")
	transport := transports.MakeTransport()
	socket.transport.Store(new(transport))
	data := types.NewStringBufferString("event payload")
	compress := false
	options := &packet.Options{Compress: &compress}

	b.ReportAllocs()
	for b.Loop() {
		socket.sendPacket(packet.MESSAGE, data, options, nil)
		socket.writeBuffer.Clear()
	}
}
