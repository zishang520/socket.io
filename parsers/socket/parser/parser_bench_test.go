package parser

import (
	"io"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

var (
	benchmarkBuffersSink []types.BufferInterface
	benchmarkPacketSink  *Packet
	benchmarkDataSink    any
	benchmarkErrorSink   error
)

func BenchmarkEncodeEvent(b *testing.B) {
	encoder := NewEncoder()
	packet := &Packet{
		Type: EVENT,
		Nsp:  "/chat",
		Id:   new(uint64(42)),
		Data: []any{"message", map[string]any{
			"room": "lobby",
			"text": strings.Repeat("socket.io", 16),
		}},
	}

	b.ReportAllocs()
	for b.Loop() {
		benchmarkBuffersSink, benchmarkErrorSink = encoder.Encode(packet)
		if benchmarkErrorSink != nil {
			b.Fatal(benchmarkErrorSink)
		}
	}
}

func BenchmarkEncodeBinaryEvent(b *testing.B) {
	encoder := NewEncoder()
	packet := &Packet{
		Type: EVENT,
		Data: []any{"upload", map[string]any{
			"name": "payload.bin",
			"data": []byte(strings.Repeat("binary", 128)),
		}},
	}

	b.ReportAllocs()
	for b.Loop() {
		benchmarkBuffersSink, benchmarkErrorSink = encoder.Encode(packet)
		if benchmarkErrorSink != nil {
			b.Fatal(benchmarkErrorSink)
		}
	}
}

func BenchmarkDecodeEventPacket(b *testing.B) {
	decoder := NewDecoder().(*decoder)
	buffer := types.NewStringBufferString(`2/chat,42["message",{"room":"lobby","text":"hello"}]`)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := buffer.Seek(0, io.SeekStart); err != nil {
			b.Fatal(err)
		}
		packet, err := decoder.decodePacket(buffer)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkPacketSink = packet
	}
}

func BenchmarkDecodeBinaryPacketHeader(b *testing.B) {
	decoder := NewDecoder().(*decoder)
	buffer := types.NewStringBufferString(`51-/chat,42["upload",{"_placeholder":true,"num":0}]`)

	b.ReportAllocs()
	for b.Loop() {
		if _, err := buffer.Seek(0, io.SeekStart); err != nil {
			b.Fatal(err)
		}
		packet, err := decoder.decodePacket(buffer)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkPacketSink = packet
	}
}

func BenchmarkReconstructPlainMap(b *testing.B) {
	data := map[string]any{
		"event": "message",
		"meta": map[string]any{
			"room":    "lobby",
			"attempt": float64(1),
		},
		"items": []any{
			map[string]any{"id": float64(1), "active": true},
			map[string]any{"id": float64(2), "active": false},
		},
	}

	b.ReportAllocs()
	for b.Loop() {
		result, err := reconstructData(data, nil)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkDataSink = result
	}
}

func BenchmarkHandleBinaryData(b *testing.B) {
	decoder := NewDecoder().(*decoder)
	attachments := uint64(1)
	placeholderData := map[string]any{
		"_placeholder": true,
		"num":          float64(0),
	}
	binaryData := []byte(strings.Repeat("binary", 128))

	b.ReportAllocs()
	for b.Loop() {
		packet := &Packet{
			Type:        BINARY_EVENT,
			Data:        []any{"upload", placeholderData},
			Attachments: &attachments,
		}
		decoder.reconstructor.Store(newBinaryReconstructor(packet))
		benchmarkErrorSink = decoder.handleBinaryData(binaryData)
		if benchmarkErrorSink != nil {
			b.Fatal(benchmarkErrorSink)
		}
	}
}
