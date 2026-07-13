package parser

import (
	"io"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

var benchmarkBufferSink types.BufferInterface

func BenchmarkV4EncodeStringPacket(b *testing.B) {
	parser := Parserv4()
	data := types.NewStringBufferString(strings.Repeat("socket.io", 32))
	pkt := &packet.Packet{Type: packet.MESSAGE, Data: data}

	b.ReportAllocs()
	for b.Loop() {
		rewindBenchmarkBuffers(b, data)
		encoded, err := parser.EncodePacket(pkt, false)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkBufferSink = encoded
	}
}

func BenchmarkV4EncodeBase64Packet(b *testing.B) {
	parser := Parserv4()
	data := types.NewBytesBuffer([]byte(strings.Repeat("binary-payload", 128)))
	pkt := &packet.Packet{Type: packet.MESSAGE, Data: data}

	b.ReportAllocs()
	for b.Loop() {
		rewindBenchmarkBuffers(b, data)
		encoded, err := parser.EncodePacket(pkt, false)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkBufferSink = encoded
	}
}

func BenchmarkV4EncodePayload(b *testing.B) {
	parser := Parserv4()
	packets, buffers := benchmarkPackets()

	b.ReportAllocs()
	for b.Loop() {
		rewindBenchmarkBuffers(b, buffers...)
		encoded, err := parser.EncodePayload(packets, false)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkBufferSink = encoded
	}
}

func BenchmarkV3EncodeTextPayload(b *testing.B) {
	parser := Parserv3()
	packets, buffers := benchmarkPackets()

	b.ReportAllocs()
	for b.Loop() {
		rewindBenchmarkBuffers(b, buffers...)
		encoded, err := parser.EncodePayload(packets, false)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkBufferSink = encoded
	}
}

func BenchmarkV3EncodeBinaryPayload(b *testing.B) {
	parser := Parserv3()
	packets, buffers := benchmarkPackets()

	b.ReportAllocs()
	for b.Loop() {
		rewindBenchmarkBuffers(b, buffers...)
		encoded, err := parser.EncodePayload(packets, true)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkBufferSink = encoded
	}
}

func benchmarkPackets() ([]*packet.Packet, []types.BufferInterface) {
	text1 := types.NewStringBufferString("hello")
	probe := types.NewStringBufferString("probe")
	text2 := types.NewStringBufferString(strings.Repeat("世界", 16))
	binary := types.NewBytesBuffer([]byte(strings.Repeat("binary", 32)))
	return []*packet.Packet{
		{Type: packet.MESSAGE, Data: text1},
		{Type: packet.PING, Data: probe},
		{Type: packet.MESSAGE, Data: binary},
		{Type: packet.MESSAGE, Data: text2},
	}, []types.BufferInterface{text1, probe, binary, text2}
}

func rewindBenchmarkBuffers(b *testing.B, buffers ...types.BufferInterface) {
	b.Helper()
	for _, buffer := range buffers {
		if _, err := buffer.Seek(0, io.SeekStart); err != nil {
			b.Fatal(err)
		}
	}
}
