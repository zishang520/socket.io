package parser

import (
	"testing"

	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

func TestNewBinaryReconstructor(t *testing.T) {
	packet := &Packet{Attachments: new(uint64)}
	*packet.Attachments = 1

	br := newBinaryReconstructor(packet)
	if br == nil {
		t.Fatal("Expected non-nil binaryReconstructor")
	}
}

func TestTakeBinaryData(t *testing.T) {
	// Setup
	packet := &Packet{
		Type: EVENT,
		Data: []any{
			"string data",
			map[string]any{"_placeholder": true, "num": float64(0)},
			map[string]any{
				"key":  "value",
				"file": map[string]any{"_placeholder": true, "num": float64(1)},
			},
			map[string]any{"_placeholder": true, "num": float64(2)},
			types.NewStringBuffer([]byte{0x07, 0x08, 0x09}),
			map[string]any{"_placeholder": true, "num": float64(3)},
		},
		Attachments: new(uint64),
	}
	*packet.Attachments = 3
	br := newBinaryReconstructor(packet)

	// Test valid data
	buf1 := types.NewBytesBufferString("data1")
	buf2 := types.NewBytesBufferString("data2")

	// Take first binary data
	pkt, err := br.takeBinaryData(buf1)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if pkt != nil {
		t.Errorf("Expected nil packet, got: %v", pkt)
	}

	// Take second binary data
	pkt, err = br.takeBinaryData(buf2)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if pkt != nil {
		t.Errorf("Expected nil packet, got: %v", pkt)
	}

	// Test with an unexpected third piece of data
	buf3 := types.NewStringBufferString("extra data")
	pkt, err = br.takeBinaryData(buf3)
	if err == nil {
		t.Error("Expected error due to extra data")
	}
	if pkt != nil {
		t.Error("Expected nil packet after extra data")
	}
}

func TestTakeBinaryDataErrorHandling(t *testing.T) {
	// Setup
	packet := &Packet{
		Type: EVENT,
		Data: []any{
			"string data",
			map[string]any{"_placeholder": true, "num": float64(0)},
			map[string]any{
				"key":  "value",
				"file": map[string]any{"_placeholder": true, "num": float64(1)},
			},
			map[string]any{"_placeholder": true, "num": float64(2)},
			types.NewStringBuffer([]byte{0x07, 0x08, 0x09}),
		},
		Attachments: new(uint64),
	}
	*packet.Attachments = 2
	br := newBinaryReconstructor(packet)

	buf1 := types.NewBytesBufferString("data1")
	buf2 := types.NewBytesBufferString("data2")

	// Take first binary data
	_, err := br.takeBinaryData(buf1)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Take second binary data and expect an error
	_, err = br.takeBinaryData(buf2)
	if err == nil {
		t.Fatal("Expected error during packet reconstruction")
	}
}

func TestCleanUp(t *testing.T) {
	packet := &Packet{
		Type: EVENT,
		Data: []any{
			"string data",
			map[string]any{"_placeholder": true, "num": float64(0)},
			map[string]any{
				"key":  "value",
				"file": map[string]any{"_placeholder": true, "num": float64(1)},
			},
			map[string]any{"_placeholder": true, "num": float64(2)},
			types.NewStringBuffer([]byte{0x07, 0x08, 0x09}),
		},
		Attachments: new(uint64),
	}
	*packet.Attachments = 2
	br := newBinaryReconstructor(packet)

	buf := types.NewBytesBufferString("data")
	br.takeBinaryData(buf)

	br.cleanUp()

	if br.buffers != nil {
		t.Errorf("Expected buffers to be nil after cleanup, got: %v", br.buffers)
	}
	if br.reconPack.Load() != nil {
		t.Errorf("Expected reconPack to be nil after cleanup, got: %v", br.reconPack.Load())
	}
}
