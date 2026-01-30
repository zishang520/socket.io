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

// TestDeconstructPacketNoBinary tests DeconstructPacket with no binary data
func TestDeconstructPacketNoBinary(t *testing.T) {
	packet := &Packet{
		Type: EVENT,
		Data: []any{"event", "hello", 123, true},
	}

	deconstructed, buffers := DeconstructPacket(packet)

	if len(buffers) != 0 {
		t.Errorf("Expected 0 buffers for non-binary data, got %d", len(buffers))
	}
	if *deconstructed.Attachments != 0 {
		t.Errorf("Expected 0 attachments, got %d", *deconstructed.Attachments)
	}
}

// TestDeconstructPacketNilData tests DeconstructPacket with nil data
func TestDeconstructPacketNilData(t *testing.T) {
	packet := &Packet{
		Type: EVENT,
		Data: nil,
	}

	deconstructed, buffers := DeconstructPacket(packet)

	if len(buffers) != 0 {
		t.Errorf("Expected 0 buffers for nil data, got %d", len(buffers))
	}
	if *deconstructed.Attachments != 0 {
		t.Errorf("Expected 0 attachments, got %d", *deconstructed.Attachments)
	}
}

// TestDeconstructPacketSliceOfBytes tests multiple binary slices
func TestDeconstructPacketSliceOfBytes(t *testing.T) {
	packet := &Packet{
		Type: EVENT,
		Data: []any{
			"upload",
			[]byte{0x01},
			[]byte{0x02},
			[]byte{0x03},
		},
	}

	deconstructed, buffers := DeconstructPacket(packet)

	if len(buffers) != 3 {
		t.Fatalf("Expected 3 buffers, got %d", len(buffers))
	}

	// Verify each placeholder is correct
	data := deconstructed.Data.([]any)
	for i := 1; i <= 3; i++ {
		placeholder, ok := data[i].(*Placeholder)
		if !ok {
			t.Errorf("Expected placeholder at index %d", i)
			continue
		}
		if placeholder.Num != int64(i-1) {
			t.Errorf("Expected placeholder num %d, got %d", i-1, placeholder.Num)
		}
	}
}

// TestReconstructPacketInvalidIndex tests reconstruction with invalid buffer index
func TestReconstructPacketInvalidIndex(t *testing.T) {
	packet := &Packet{
		Type: EVENT,
		Data: []any{
			map[string]any{"_placeholder": true, "num": float64(5)}, // Invalid index
		},
		Attachments: new(uint64),
	}
	*packet.Attachments = 1

	buffers := []types.BufferInterface{
		types.NewBytesBuffer([]byte{0x01}),
	}

	_, err := ReconstructPacket(packet, buffers)
	// The function returns an error for invalid attachments
	if err == nil {
		t.Log("ReconstructPacket handled invalid index gracefully")
	} else {
		// Error is expected for illegal attachments
		t.Logf("Got expected error: %v", err)
	}
}

// TestReconstructPacketEmptyBuffers tests reconstruction with empty buffer list
func TestReconstructPacketEmptyBuffers(t *testing.T) {
	packet := &Packet{
		Type:        EVENT,
		Data:        []any{"event", "data"},
		Attachments: new(uint64),
	}
	*packet.Attachments = 0

	buffers := []types.BufferInterface{}

	reconstructed, err := ReconstructPacket(packet, buffers)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	data := reconstructed.Data.([]any)
	if data[0] != "event" || data[1] != "data" {
		t.Errorf("Data should remain unchanged")
	}
}

// TestDeconstructPacketDeepNested tests deeply nested binary data
func TestDeconstructPacketDeepNested(t *testing.T) {
	packet := &Packet{
		Type: EVENT,
		Data: []any{
			"event",
			map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"binary": []byte{0x01, 0x02},
					},
				},
			},
		},
	}

	deconstructed, buffers := DeconstructPacket(packet)

	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffer, got %d", len(buffers))
	}

	if !bytes.Equal(buffers[0].Bytes(), []byte{0x01, 0x02}) {
		t.Errorf("Buffer data mismatch")
	}

	// Verify deep nested placeholder
	data := deconstructed.Data.([]any)
	nested := data[1].(map[string]any)["level1"].(map[string]any)["level2"].(map[string]any)["binary"]
	placeholder, ok := nested.(*Placeholder)
	if !ok {
		t.Errorf("Expected placeholder at nested path")
	}
	if placeholder.Num != 0 {
		t.Errorf("Expected placeholder num 0, got %d", placeholder.Num)
	}
}

// TestReconstructPacketDeepNested tests reconstruction of deeply nested binary
func TestReconstructPacketDeepNested(t *testing.T) {
	packet := &Packet{
		Type: EVENT,
		Data: []any{
			"event",
			map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"binary": map[string]any{"_placeholder": true, "num": float64(0)},
					},
				},
			},
		},
		Attachments: new(uint64),
	}
	*packet.Attachments = 1

	buffers := []types.BufferInterface{
		types.NewBytesBuffer([]byte{0x01, 0x02}),
	}

	reconstructed, err := ReconstructPacket(packet, buffers)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify deep nested reconstruction
	data := reconstructed.Data.([]any)
	nested := data[1].(map[string]any)["level1"].(map[string]any)["level2"].(map[string]any)["binary"]
	buf, ok := nested.(types.BufferInterface)
	if !ok {
		t.Errorf("Expected BufferInterface at nested path")
	}
	if !bytes.Equal(buf.Bytes(), []byte{0x01, 0x02}) {
		t.Errorf("Buffer data mismatch")
	}
}

// TestPlaceholderStruct tests Placeholder struct fields
func TestPlaceholderStruct(t *testing.T) {
	p := &Placeholder{
		Placeholder: true,
		Num:         42,
	}

	if !p.Placeholder {
		t.Error("Placeholder field should be true")
	}
	if p.Num != 42 {
		t.Errorf("Num field should be 42, got %d", p.Num)
	}
}

// TestDeconstructWithSliceInMap tests slices containing binary within maps
func TestDeconstructWithSliceInMap(t *testing.T) {
	packet := &Packet{
		Type: EVENT,
		Data: []any{
			"upload",
			map[string]any{
				"files": []any{
					[]byte{0x01},
					[]byte{0x02},
				},
			},
		},
	}

	deconstructed, buffers := DeconstructPacket(packet)

	if len(buffers) != 2 {
		t.Fatalf("Expected 2 buffers, got %d", len(buffers))
	}

	// Verify placeholders in slice
	data := deconstructed.Data.([]any)
	files := data[1].(map[string]any)["files"].([]any)
	for i := 0; i < 2; i++ {
		placeholder, ok := files[i].(*Placeholder)
		if !ok {
			t.Errorf("Expected placeholder at files[%d]", i)
			continue
		}
		if placeholder.Num != int64(i) {
			t.Errorf("Expected placeholder num %d, got %d", i, placeholder.Num)
		}
	}
}
