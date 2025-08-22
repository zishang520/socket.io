package parser

import (
	"bytes"
	"testing"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

// TestDeconstructPacket tests the DeconstructPacket function
func TestDeconstructPacket(t *testing.T) {
	Data := []any{
		"string data",
		[]byte{0x01, 0x02, 0x03},
		map[string]any{
			"key":  "value",
			"file": []byte{0x04, 0x05, 0x06},
		},
		types.NewBytesBuffer([]byte{0x07, 0x08, 0x09}),
		types.NewStringBuffer([]byte{0x07, 0x08, 0x09}),
	}
	// Prepare the packet with mixed data
	packet := &Packet{
		Type: EVENT,
		Data: Data,
	}

	// Call the DeconstructPacket function
	deconstructedPacket, buffers := DeconstructPacket(packet)

	// Check if the attachments count matches the buffers length
	if *deconstructedPacket.Attachments != uint64(len(buffers)) {
		t.Errorf("expected %d attachments, got %d", len(buffers), *deconstructedPacket.Attachments)
	}

	// Check if buffers are correctly created
	if !bytes.Equal(buffers[0].Bytes(), Data[1].([]byte)) {
		t.Errorf("buffer %d data mismatch", 0)
	}

	// Check if buffers are correctly created
	if !bytes.Equal(buffers[1].Bytes(), Data[2].(map[string]any)["file"].([]byte)) {
		t.Errorf("buffer %d data mismatch", 1)
	}

	// Check if buffers are correctly created
	if !bytes.Equal(buffers[2].Bytes(), []byte{0x07, 0x08, 0x09}) {
		t.Errorf("buffer %d data mismatch", 2)
	}

	// Check if placeholders are correctly placed
	if placeholder, ok := deconstructedPacket.Data.([]any)[3].(*Placeholder); !ok || !placeholder.Placeholder || placeholder.Num != 2 {
		t.Errorf("expected placeholder in data[3], got %v", deconstructedPacket.Data.([]any)[3])
	}
}

// TestReconstructPacket tests the ReconstructPacket function
func TestReconstructPacket(t *testing.T) {
	// Prepare the deconstructed packet with placeholders
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

	// Prepare buffers for reconstruction
	buffers := []types.BufferInterface{
		types.NewBytesBuffer([]byte{0x01, 0x02, 0x03}),
		types.NewBytesBuffer([]byte{0x04, 0x05, 0x06}),
		types.NewBytesBuffer([]byte{0x07, 0x08, 0x09}),
	}

	// Call the ReconstructPacket function
	reconstructedPacket, err := ReconstructPacket(packet, buffers)
	if err != nil {
		t.Errorf("error reconstructing packet: %v", err)
	}

	if len(buffers) != 3 {
		t.Errorf("expected `3` in buffers, got %d", len(buffers))
	}

	if reconstructedPacket.Attachments != nil {
		t.Errorf("expected `nil` in attachments, got %d", *reconstructedPacket.Attachments)
	}

	// Check if data is correctly reconstructed
	data := reconstructedPacket.Data.([]any)
	if stringData, ok := data[0].(string); !ok || stringData != "string data" {
		t.Errorf("expected 'string data' in data[0], got %v", data[0])
	}

	if bytesData, ok := data[1].(types.BufferInterface); !ok || !bytes.Equal(bytesData.Bytes(), buffers[0].Bytes()) {
		t.Errorf("expected buffer data in data[1], got %v", data[1])
	}

	if fileData, ok := data[2].(map[string]any)["file"].(types.BufferInterface); !ok || !bytes.Equal(fileData.Bytes(), buffers[1].Bytes()) {
		t.Errorf("expected buffer data in data[2]['file'], got %v", data[2].(map[string]any)["file"])
	}

	if bytesData, ok := data[3].(types.BufferInterface); !ok || !bytes.Equal(bytesData.Bytes(), buffers[2].Bytes()) {
		t.Errorf("expected buffer data in data[2], got %v", data[2])
	}
}
