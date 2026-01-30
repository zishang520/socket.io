package parser

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/v3/pkg/types"
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

// TestEncodeConnectPacket tests CONNECT packet encoding
func TestEncodeConnectPacket(t *testing.T) {
	e := NewEncoder()

	tests := []struct {
		name     string
		packet   *Packet
		expected string
	}{
		{
			"CONNECT without data",
			&Packet{Type: CONNECT},
			"0",
		},
		{
			"CONNECT with auth data",
			&Packet{Type: CONNECT, Data: map[string]any{"token": "abc"}},
			`0{"token":"abc"}`,
		},
		{
			"CONNECT with namespace",
			&Packet{Type: CONNECT, Nsp: "/admin"},
			"0/admin,",
		},
		{
			"CONNECT with namespace and auth",
			&Packet{Type: CONNECT, Nsp: "/admin", Data: map[string]any{"token": "xyz"}},
			`0/admin,{"token":"xyz"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffers := e.Encode(tt.packet)
			if len(buffers) != 1 {
				t.Fatalf("Expected 1 buffer, got %d", len(buffers))
			}
			result := buffers[0].(*types.StringBuffer).String()
			if result != tt.expected {
				t.Errorf("Got %s, expected %s", result, tt.expected)
			}
		})
	}
}

// TestEncodeDisconnectPacket tests DISCONNECT packet encoding
func TestEncodeDisconnectPacket(t *testing.T) {
	e := NewEncoder()

	tests := []struct {
		name     string
		packet   *Packet
		expected string
	}{
		{
			"DISCONNECT default namespace",
			&Packet{Type: DISCONNECT},
			"1",
		},
		{
			"DISCONNECT with namespace",
			&Packet{Type: DISCONNECT, Nsp: "/admin"},
			"1/admin,",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffers := e.Encode(tt.packet)
			if len(buffers) != 1 {
				t.Fatalf("Expected 1 buffer, got %d", len(buffers))
			}
			result := buffers[0].(*types.StringBuffer).String()
			if result != tt.expected {
				t.Errorf("Got %s, expected %s", result, tt.expected)
			}
		})
	}
}

// TestEncodeConnectErrorPacket tests CONNECT_ERROR packet encoding
func TestEncodeConnectErrorPacket(t *testing.T) {
	e := NewEncoder()

	tests := []struct {
		name     string
		packet   *Packet
		expected string
	}{
		{
			"CONNECT_ERROR with string",
			&Packet{Type: CONNECT_ERROR, Data: "Not authorized"},
			`4"Not authorized"`,
		},
		{
			"CONNECT_ERROR with object",
			&Packet{Type: CONNECT_ERROR, Data: map[string]any{"message": "error"}},
			`4{"message":"error"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buffers := e.Encode(tt.packet)
			if len(buffers) != 1 {
				t.Fatalf("Expected 1 buffer, got %d", len(buffers))
			}
			result := buffers[0].(*types.StringBuffer).String()
			if result != tt.expected {
				t.Errorf("Got %s, expected %s", result, tt.expected)
			}
		})
	}
}

// TestEncodeBinaryAckPacket tests BINARY_ACK packet encoding
func TestEncodeBinaryAckPacket(t *testing.T) {
	e := NewEncoder()

	id := uint64(5)
	packet := &Packet{
		Type: ACK,
		Id:   &id,
		Data: []any{[]byte{0x01, 0x02, 0x03}},
	}

	buffers := e.Encode(packet)
	if len(buffers) != 2 {
		t.Fatalf("Expected 2 buffers, got %d", len(buffers))
	}

	// Should be upgraded to BINARY_ACK
	header := buffers[0].(*types.StringBuffer).String()
	if !bytes.HasPrefix([]byte(header), []byte("61-5")) {
		t.Errorf("Expected BINARY_ACK header starting with '61-5', got %s", header)
	}
}

// TestEncodeNestedBinaryData tests encoding with nested binary data
func TestEncodeNestedBinaryData(t *testing.T) {
	e := NewEncoder()

	packet := &Packet{
		Type: EVENT,
		Data: []any{
			"upload",
			map[string]any{
				"file": []byte{0x01, 0x02},
				"meta": map[string]any{
					"thumbnail": []byte{0x03, 0x04},
				},
			},
		},
	}

	buffers := e.Encode(packet)
	if len(buffers) != 3 {
		t.Fatalf("Expected 3 buffers (header + 2 binaries), got %d", len(buffers))
	}

	// Verify binary buffers
	if !bytes.Equal(buffers[1].Bytes(), []byte{0x01, 0x02}) {
		t.Errorf("First binary buffer mismatch")
	}
	if !bytes.Equal(buffers[2].Bytes(), []byte{0x03, 0x04}) {
		t.Errorf("Second binary buffer mismatch")
	}
}

// TestEncodeWithStringsReader tests encoding with *strings.Reader data
func TestEncodeWithStringsReader(t *testing.T) {
	e := NewEncoder()

	packet := &Packet{
		Type: EVENT,
		Data: []any{"event", strings.NewReader("string data")},
	}

	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffer (strings.Reader is not binary), got %d", len(buffers))
	}
}

// TestEncodeWithIOReader tests encoding with io.Reader (binary)
func TestEncodeWithIOReader(t *testing.T) {
	e := NewEncoder()

	packet := &Packet{
		Type: EVENT,
		Data: []any{"upload", bytes.NewReader([]byte{0x01, 0x02, 0x03})},
	}

	buffers := e.Encode(packet)
	if len(buffers) != 2 {
		t.Fatalf("Expected 2 buffers (io.Reader is binary), got %d", len(buffers))
	}
}

// TestEncodeBinaryEventExplicit tests explicitly set BINARY_EVENT
func TestEncodeBinaryEventExplicit(t *testing.T) {
	e := NewEncoder()

	attachments := uint64(1)
	packet := &Packet{
		Type:        BINARY_EVENT,
		Attachments: &attachments,
		Nsp:         "/chat",
	}

	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffer, got %d", len(buffers))
	}

	result := buffers[0].(*types.StringBuffer).String()
	if !bytes.HasPrefix([]byte(result), []byte("51-/chat,")) {
		t.Errorf("Unexpected result: %s", result)
	}
}

// TestEncodePacketWithAllFields tests encoding a packet with all fields set
func TestEncodePacketWithAllFields(t *testing.T) {
	e := NewEncoder()

	id := uint64(42)
	packet := &Packet{
		Type: EVENT,
		Nsp:  "/admin",
		Id:   &id,
		Data: []any{"message", "hello"},
	}

	buffers := e.Encode(packet)
	if len(buffers) != 1 {
		t.Fatalf("Expected 1 buffer, got %d", len(buffers))
	}

	result := buffers[0].(*types.StringBuffer).String()
	expected := `2/admin,42["message","hello"]`
	if result != expected {
		t.Errorf("Got %s, expected %s", result, expected)
	}
}
