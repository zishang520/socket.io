package parser

import (
	"bytes"
	"testing"

	"github.com/zishang520/socket.io/servers/engine/v3/types"
)

func TestEncode(t *testing.T) {
	e := NewEncoder()

	// Test case for non-binary packet
	packet := &Packet{
		Type: EVENT,
		Data: map[string]any{"key": "value"},
	}
	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Errorf("Expected 1 buffer, got %d", len(buffers))
	}

	expected := []byte(`2{"key":"value"}`)
	if !bytes.Equal(buffers[0].(*types.StringBuffer).Bytes(), expected) {
		t.Errorf("Unexpected encoding result. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expected)
	}

	// Test case for binary packet
	packet = &Packet{
		Type:        EVENT,
		Attachments: new(uint64),
		Data:        []byte{1, 2, 3},
	}
	*packet.Attachments = 1
	buffers = e.Encode(packet)
	if len(buffers) != 2 {
		t.Errorf("Expected 2 buffers, got %d", len(buffers))
	}

	encodedPacket := buffers[0].(*types.StringBuffer).Bytes()
	expectedPrefix := []byte("5") // Adjust based on actual encoding type
	if !bytes.HasPrefix(encodedPacket, expectedPrefix) {
		t.Errorf("Unexpected encoding result prefix. Got %v, expected %v", encodedPacket, expectedPrefix)
	}

	if !bytes.Equal(buffers[1].(*types.BytesBuffer).Bytes(), []byte{1, 2, 3}) {
		t.Errorf("Unexpected buffer content. Got %v, expected %v", buffers[1].(*types.BytesBuffer).Bytes(), []byte{1, 2, 3})
	}
}

func TestEncodeAck(t *testing.T) {
	e := NewEncoder()

	// Test ACK packet encoding
	packet := &Packet{
		Type: ACK,
		Data: map[string]any{"key": "value"},
	}
	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffer, got %d", len(buffers))
	}

	expected := []byte(`3{"key":"value"}`)
	if !bytes.Equal(buffers[0].(*types.StringBuffer).Bytes(), expected) {
		t.Errorf("Unexpected encoding result. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expected)
	}
}

func TestEncodeEventBinary(t *testing.T) {
	e := NewEncoder()

	// Test EVENT packet encoding
	data := []any{[]byte{1, 2, 3}, []byte{4, 5, 6}}
	packet := &Packet{
		Type:        EVENT,
		Attachments: new(uint64),
		Data:        data,
		Nsp:         "data",
	}
	*packet.Attachments = 2
	buffers := e.Encode(packet)
	if len(buffers) != 3 {
		t.Fatalf("Expected 3 buffers, got %d", len(buffers))
	}

	expectedPrefix := []byte("52-data,") // Adjust based on actual encoding type
	if !bytes.HasPrefix(buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix) {
		t.Errorf("Unexpected encoding result prefix. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix)
	}

	if !bytes.Equal(buffers[1].(*types.BytesBuffer).Bytes(), data[0].([]byte)) {
		t.Errorf("Unexpected buffer content. Got %v, expected %v", buffers[1].(*types.BytesBuffer).Bytes(), data)
	}
}

func TestEncodeEventString(t *testing.T) {
	e := NewEncoder()

	// Test EVENT packet encoding
	data := "string"
	packet := &Packet{
		Type: EVENT,
		Data: data,
	}
	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffers, got %d", len(buffers))
	}

	expectedPrefix := []byte("2") // Adjust based on actual encoding type
	if !bytes.HasPrefix(buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix) {
		t.Errorf("Unexpected encoding result prefix. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix)
	}

	if !bytes.Equal(buffers[0].(*types.StringBuffer).Bytes(), []byte(`2"string"`)) {
		t.Errorf("Unexpected buffer content. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), data)
	}
}

func TestEncodeEventId(t *testing.T) {
	e := NewEncoder()

	// Test EVENT packet encoding
	data := "string"
	packet := &Packet{
		Type: EVENT,
		Data: data,
		Id:   new(uint64),
	}
	*packet.Id = 6
	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffers, got %d", len(buffers))
	}

	expectedPrefix := []byte("26") // Adjust based on actual encoding type
	if !bytes.HasPrefix(buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix) {
		t.Errorf("Unexpected encoding result prefix. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix)
	}

	if !bytes.Equal(buffers[0].(*types.StringBuffer).Bytes(), []byte(`26"string"`)) {
		t.Errorf("Unexpected buffer content. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), data)
	}
}

func TestEncodeEventNamespace(t *testing.T) {
	e := NewEncoder()

	// Test EVENT packet encoding
	data := "string"
	packet := &Packet{
		Type: EVENT,
		Data: data,
		Nsp:  "/",
	}
	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffers, got %d", len(buffers))
	}

	expectedPrefix := []byte("2\"") // Adjust based on actual encoding type
	if !bytes.HasPrefix(buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix) {
		t.Errorf("Unexpected encoding result prefix. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix)
	}

	if !bytes.Equal(buffers[0].(*types.StringBuffer).Bytes(), []byte(`2"string"`)) {
		t.Errorf("Unexpected buffer content. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), data)
	}

	packet = &Packet{
		Type: EVENT,
		Data: data,
		Nsp:  "/test",
	}
	buffers = e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffers, got %d", len(buffers))
	}

	expectedPrefix = []byte("2/") // Adjust based on actual encoding type
	if !bytes.HasPrefix(buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix) {
		t.Errorf("Unexpected encoding result prefix. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix)
	}

	if !bytes.Equal(buffers[0].(*types.StringBuffer).Bytes(), []byte(`2/test,"string"`)) {
		t.Errorf("Unexpected buffer content. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), data)
	}
}

func TestEncodeAckBinary(t *testing.T) {
	e := NewEncoder()

	// Test ACK packet encoding
	data := []byte{4, 5, 6}
	packet := &Packet{
		Type:        ACK,
		Attachments: new(uint64),
		Data:        data,
	}
	*packet.Attachments = 1
	buffers := e.Encode(packet)
	if len(buffers) != 2 {
		t.Fatalf("Expected 2 buffers, got %d", len(buffers))
	}

	expectedPrefix := []byte("61-{") // Adjust based on actual encoding type
	if !bytes.HasPrefix(buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix) {
		t.Errorf("Unexpected encoding result prefix. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expectedPrefix)
	}

	if !bytes.Equal(buffers[1].(*types.BytesBuffer).Bytes(), data) {
		t.Errorf("Unexpected buffer content. Got %v, expected %v", buffers[1].(*types.BytesBuffer).Bytes(), data)
	}
}

func TestEncodeEmptyPacket(t *testing.T) {
	e := NewEncoder()

	// Test empty packet encoding for EVENT type
	packet := &Packet{
		Type: EVENT,
	}
	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffer, got %d", len(buffers))
	}

	expected := []byte("2")
	if !bytes.Equal(buffers[0].(*types.StringBuffer).Bytes(), expected) {
		t.Errorf("Unexpected encoding result. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expected)
	}
}

func TestEncodeEmptyBinaryPacket(t *testing.T) {
	e := NewEncoder()

	// Test empty binary packet encoding
	packet := &Packet{
		Type: BINARY_EVENT,
	}
	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffer, got %d", len(buffers))
	}

	expected := []byte("5")
	if !bytes.Equal(buffers[0].(*types.StringBuffer).Bytes(), expected) {
		t.Errorf("Unexpected encoding result. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expected)
	}
}

func TestEncodeInvalidData(t *testing.T) {
	e := NewEncoder()

	// Test encoding with invalid data
	packet := &Packet{
		Type: EVENT,
		Data: map[string]any{"key": func() {}}, // Invalid data
	}
	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffer, got %d", len(buffers))
	}

	expected := []byte(`2`) // Expected to still produce a valid result despite invalid data
	if !bytes.Equal(buffers[0].(*types.StringBuffer).Bytes(), expected) {
		t.Errorf("Unexpected encoding result. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expected)
	}
}

func TestEncodeErrorHandling(t *testing.T) {
	e := NewEncoder()

	// Test error handling with an invalid type in data
	packet := &Packet{
		Type: EVENT,
		Data: map[string]any{"key": make(chan int)}, // Invalid data type
	}
	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffer, got %d", len(buffers))
	}

	expected := []byte("2") // Expected to still produce a valid result despite the invalid type
	if !bytes.Equal(buffers[0].(*types.StringBuffer).Bytes(), expected) {
		t.Errorf("Unexpected encoding result. Got %v, expected %v", buffers[0].(*types.StringBuffer).Bytes(), expected)
	}
}
