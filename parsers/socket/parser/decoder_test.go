package parser

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestNewDecoder(t *testing.T) {
	d := NewDecoder()
	if d == nil {
		t.Fatal("NewDecoder returned nil")
	}
	if _, ok := d.(*decoder); !ok {
		t.Fatal("NewDecoder did not return a *decoder")
	}
}

func TestAdd(t *testing.T) {
	d := NewDecoder().(*decoder)

	tests := []struct {
		name    string
		input   any
		wantErr bool
	}{
		{"Valid string", "4\"hello\"", false},
		{"Invalid string", "4hello", true},
		{"Valid *strings.Reader", strings.NewReader(`4"hello"`), false},
		{"Valid *types.StringBuffer", types.NewStringBufferString("4\"hello\""), false},
		{"Invalid type", 123, true},
		{"Binary data without reconstructor", []byte{1, 2, 3}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := d.Add(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Add(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestDecodeAsString(t *testing.T) {
	d := NewDecoder().(*decoder)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"Invalid EVENT packet (null)", "2[null]", true},
		{"Valid EVENT packet (integer)", "2[0]", false}, // Number event names are valid
		{"Valid EVENT packet (float)", "2[1.1]", false}, // Number event names are valid
		{"Valid EVENT packet", "2[\"hello\",\"world\"]", false},
		{"Invalid packet type", "9invalid", true},
		{"BINARY_EVENT packet", "51-[\"hello\",\"world\",{\"_placeholder\": true, \"num\": 0}]", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := d.decodeAsString(types.NewStringBufferString(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("decodeAsString(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestDecodeString(t *testing.T) {
	d := NewDecoder().(*decoder)

	tests := []struct {
		name    string
		input   string
		want    *Packet
		wantErr bool
	}{
		{
			"Valid EVENT packet",
			"2[\"hello\",\"world\"]",
			&Packet{Type: EVENT, Nsp: "/", Data: []any{"hello", "world"}},
			false,
		},
		{
			"Valid BINARY_EVENT packet",
			"51-[\"hello\",\"world\",{\"_placeholder\": true, \"num\": 0}]",
			&Packet{Type: BINARY_EVENT, Nsp: "/", Attachments: new(uint64), Data: []any{"hello", "world", map[string]any{"_placeholder": true, "num": float64(0)}}},
			false,
		},
		{
			"Valid packet with namespace and id",
			"2/admin,1[\"hello\",\"world\"]",
			&Packet{Type: EVENT, Nsp: "/admin", Id: new(uint64), Data: []any{"hello", "world"}},
			false,
		},
		{"Invalid packet type", "9\"invalid\"", nil, true},
		{"Invalid JSON", "2invalid", nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := d.decodePacket(types.NewStringBufferString(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("decodePacket(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if tt.want != nil && !reflect.DeepEqual(got.Data, tt.want.Data) {
				t.Errorf("decodePacket(%s) = %v, want %v", tt.input, got.Data, tt.want.Data)
			}
		})
	}
}

func TestDestroy(t *testing.T) {
	d := NewDecoder().(*decoder)
	d.reconstructor.Store(newBinaryReconstructor(&Packet{}))
	d.Destroy()
	if d.reconstructor.Load().packet.Load() != nil {
		t.Error("Destroy() did not clear the reconstructor")
	}
}

func TestIsPayloadValid(t *testing.T) {
	tests := []struct {
		name    string
		pType   PacketType
		payload any
		want    bool
	}{
		{"Valid CONNECT", CONNECT, map[string]any{"key": "value"}, true},
		{"Invalid CONNECT", CONNECT, "string", false},
		{"Valid DISCONNECT", DISCONNECT, nil, true},
		{"Invalid DISCONNECT", DISCONNECT, "data", false},
		{"Valid CONNECT_ERROR (map)", CONNECT_ERROR, map[string]any{"error": "message"}, true},
		{"Valid CONNECT_ERROR (string)", CONNECT_ERROR, "error message", true},
		{"Invalid CONNECT_ERROR", CONNECT_ERROR, 123, false},
		{"Valid EVENT", EVENT, []any{"event", "data"}, true},
		{"Valid EVENT (number event name)", EVENT, []any{float64(1), "data"}, true},
		{"Invalid EVENT (reserved)", EVENT, []any{"connect", "data"}, false},
		{"Invalid EVENT (reserved newListener)", EVENT, []any{"newListener", "data"}, false},
		{"Invalid EVENT (reserved removeListener)", EVENT, []any{"removeListener", "data"}, false},
		{"Invalid EVENT (not array)", EVENT, "not array", false},
		{"Invalid EVENT (null event name)", EVENT, []any{nil, "data"}, false},
		{"Invalid EVENT (boolean event name)", EVENT, []any{true, "data"}, false},
		{"Valid ACK", ACK, []any{"data"}, true},
		{"Invalid ACK", ACK, "not array", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPayloadValid(tt.pType, tt.payload); got != tt.want {
				t.Errorf("isPayloadValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Mock for binary data
type mockBinaryData struct {
	io.Reader
	io.Closer
}

func (m *mockBinaryData) Close() error {
	return nil
}

func TestAddBinaryData(t *testing.T) {
	d := NewDecoder().(*decoder)
	packet := &Packet{Type: BINARY_EVENT, Attachments: new(uint64), Data: []any{map[string]any{"_placeholder": true, "num": float64(0)}}}
	*packet.Attachments = 1
	d.reconstructor.Store(newBinaryReconstructor(packet))

	binaryData := &mockBinaryData{Reader: bytes.NewReader([]byte{1, 2, 3})}
	err := d.Add(binaryData)
	if err != nil {
		t.Errorf("Add() error = %v, wantErr false", err)
	}
	if d.reconstructor.Load() != nil {
		t.Error("reconstructor should be nil after processing binary data")
	}
}

func TestDecoderEmitDecoded(t *testing.T) {
	d := NewDecoder().(*decoder)

	var decodedPacket *Packet
	d.On("decoded", func(args ...any) {
		if len(args) > 0 {
			if p, ok := args[0].(*Packet); ok {
				decodedPacket = p
			}
		}
	})

	testPacket := "2[\"test\",\"data\"]"
	err := d.Add(testPacket)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if decodedPacket == nil {
		t.Fatal("decoded event was not emitted")
	}

	expectedPacket := &Packet{Type: EVENT, Nsp: "/", Data: []any{"test", "data"}}
	if !reflect.DeepEqual(decodedPacket, expectedPacket) {
		t.Errorf("decoded packet = %v, want %v", decodedPacket, expectedPacket)
	}
}

func TestDecoderErrorCases(t *testing.T) {
	d := NewDecoder().(*decoder)

	// Test case: Adding string data when reconstructor is active
	d.reconstructor.Store(newBinaryReconstructor(&Packet{}))
	err := d.Add("some string data")
	if err == nil || err.Error() != "got plaintext data when reconstructing a packet" {
		t.Errorf("Expected error for string data during reconstruction, got: %v", err)
	}
	d.reconstructor.Store(nil)

	// Test case: Invalid packet type
	err = d.Add("9invalid packet")
	if err == nil || !strings.Contains(err.Error(), "unknown packet type") {
		t.Errorf("Expected error for invalid packet type, got: %v", err)
	}

	// Test case: Illegal attachments
	err = d.Add("5abc[\"data\"]")
	if err == nil || err.Error() != "illegal attachments" {
		t.Errorf("Expected error for illegal attachments, got: %v", err)
	}

	// Test case: Invalid JSON payload
	err = d.Add("2{invalid json}")
	if err == nil || err.Error() != "invalid payload" {
		t.Errorf("Expected error for invalid JSON payload, got: %v", err)
	}
}

// TestDecodeConnectPacket tests CONNECT packet decoding
func TestDecodeConnectPacket(t *testing.T) {
	d := NewDecoder().(*decoder)

	tests := []struct {
		name    string
		input   string
		wantNsp string
		wantErr bool
	}{
		{"CONNECT without data", "0", "/", false},
		{"CONNECT with auth data", `0{"token":"abc"}`, "/", false},
		{"CONNECT with namespace", "0/admin,", "/admin", false},
		{"CONNECT with namespace and auth", `0/admin,{"token":"abc"}`, "/admin", false},
		{"CONNECT with nested namespace", "0/admin/users,", "/admin/users", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := d.decodePacket(types.NewStringBufferString(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("decodePacket(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if packet.Type != CONNECT {
				t.Errorf("Expected CONNECT type, got %v", packet.Type)
			}
			if packet.Nsp != tt.wantNsp {
				t.Errorf("Expected namespace %s, got %s", tt.wantNsp, packet.Nsp)
			}
		})
	}
}

// TestDecodeDisconnectPacket tests DISCONNECT packet decoding
func TestDecodeDisconnectPacket(t *testing.T) {
	d := NewDecoder().(*decoder)

	tests := []struct {
		name    string
		input   string
		wantNsp string
		wantErr bool
	}{
		{"DISCONNECT default namespace", "1", "/", false},
		{"DISCONNECT with namespace", "1/admin,", "/admin", false},
		{"DISCONNECT with data should fail", "1{}", "/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := d.decodePacket(types.NewStringBufferString(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("decodePacket(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if packet.Type != DISCONNECT {
					t.Errorf("Expected DISCONNECT type, got %v", packet.Type)
				}
				if packet.Nsp != tt.wantNsp {
					t.Errorf("Expected namespace %s, got %s", tt.wantNsp, packet.Nsp)
				}
			}
		})
	}
}

// TestDecodeAckPacket tests ACK packet decoding
func TestDecodeAckPacket(t *testing.T) {
	d := NewDecoder().(*decoder)

	tests := []struct {
		name     string
		input    string
		wantId   uint64
		wantData []any
		wantErr  bool
	}{
		{"ACK with id and data", `31["response"]`, 1, []any{"response"}, false},
		{"ACK with large id", `3123456["data"]`, 123456, []any{"data"}, false},
		{"ACK with namespace", `3/admin,5["ok"]`, 5, []any{"ok"}, false},
		{"ACK without array should fail", `31"response"`, 1, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := d.decodePacket(types.NewStringBufferString(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("decodePacket(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if packet.Type != ACK {
					t.Errorf("Expected ACK type, got %v", packet.Type)
				}
				if packet.Id == nil || *packet.Id != tt.wantId {
					t.Errorf("Expected id %d, got %v", tt.wantId, packet.Id)
				}
			}
		})
	}
}

// TestDecodeConnectErrorPacket tests CONNECT_ERROR packet decoding
func TestDecodeConnectErrorPacket(t *testing.T) {
	d := NewDecoder().(*decoder)

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"CONNECT_ERROR with string message", `4"Not authorized"`, false},
		{"CONNECT_ERROR with object", `4{"message":"error","data":{}}`, false},
		{"CONNECT_ERROR with namespace", `4/admin,"Forbidden"`, false},
		{"CONNECT_ERROR with number should fail", `4123`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet, err := d.decodePacket(types.NewStringBufferString(tt.input))
			if (err != nil) != tt.wantErr {
				t.Errorf("decodePacket(%s) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && packet.Type != CONNECT_ERROR {
				t.Errorf("Expected CONNECT_ERROR type, got %v", packet.Type)
			}
		})
	}
}

// TestDecodeBinaryAckPacket tests BINARY_ACK packet decoding
func TestDecodeBinaryAckPacket(t *testing.T) {
	d := NewDecoder().(*decoder)

	input := `61-5[{"_placeholder":true,"num":0}]`
	packet, err := d.decodePacket(types.NewStringBufferString(input))
	if err != nil {
		t.Fatalf("decodePacket(%s) error = %v", input, err)
	}

	if packet.Type != BINARY_ACK {
		t.Errorf("Expected BINARY_ACK type, got %v", packet.Type)
	}
	if packet.Attachments == nil || *packet.Attachments != 1 {
		t.Errorf("Expected 1 attachment, got %v", packet.Attachments)
	}
	if packet.Id == nil || *packet.Id != 5 {
		t.Errorf("Expected id 5, got %v", packet.Id)
	}
}

// TestBinaryPacketReconstruction tests complete binary packet reconstruction
func TestBinaryPacketReconstruction(t *testing.T) {
	d := NewDecoder().(*decoder)

	var decodedPacket *Packet
	d.On("decoded", func(args ...any) {
		if len(args) > 0 {
			if p, ok := args[0].(*Packet); ok {
				decodedPacket = p
			}
		}
	})

	// Add BINARY_EVENT packet header
	err := d.Add(`51-["upload",{"_placeholder":true,"num":0}]`)
	if err != nil {
		t.Fatalf("Add header error: %v", err)
	}

	// Packet should not be emitted yet
	if decodedPacket != nil {
		t.Error("Packet should not be emitted before binary data")
	}

	// Add binary data
	binaryData := []byte{0x01, 0x02, 0x03, 0x04}
	err = d.Add(binaryData)
	if err != nil {
		t.Fatalf("Add binary error: %v", err)
	}

	// Now packet should be emitted
	if decodedPacket == nil {
		t.Fatal("Packet should be emitted after binary data")
	}

	if decodedPacket.Type != BINARY_EVENT {
		t.Errorf("Expected BINARY_EVENT type, got %v", decodedPacket.Type)
	}

	data, ok := decodedPacket.Data.([]any)
	if !ok || len(data) != 2 {
		t.Fatalf("Expected array with 2 elements, got %v", decodedPacket.Data)
	}

	if data[0] != "upload" {
		t.Errorf("Expected event name 'upload', got %v", data[0])
	}

	// Check that placeholder was replaced with buffer
	if _, ok := data[1].(types.BufferInterface); !ok {
		t.Errorf("Expected BufferInterface, got %T", data[1])
	}
}

// TestMultipleBinaryAttachments tests packets with multiple binary attachments
func TestMultipleBinaryAttachments(t *testing.T) {
	d := NewDecoder().(*decoder)

	var decodedPacket *Packet
	d.On("decoded", func(args ...any) {
		if len(args) > 0 {
			if p, ok := args[0].(*Packet); ok {
				decodedPacket = p
			}
		}
	})

	// Add BINARY_EVENT packet with 2 attachments
	err := d.Add(`52-["files",{"_placeholder":true,"num":0},{"_placeholder":true,"num":1}]`)
	if err != nil {
		t.Fatalf("Add header error: %v", err)
	}

	// Add first binary
	err = d.Add([]byte{0x01})
	if err != nil {
		t.Fatalf("Add first binary error: %v", err)
	}
	if decodedPacket != nil {
		t.Error("Packet should not be emitted after first binary")
	}

	// Add second binary
	err = d.Add([]byte{0x02})
	if err != nil {
		t.Fatalf("Add second binary error: %v", err)
	}
	if decodedPacket == nil {
		t.Fatal("Packet should be emitted after all binaries")
	}
}

// TestZeroAttachments tests BINARY_EVENT with 0 attachments
func TestZeroAttachments(t *testing.T) {
	d := NewDecoder().(*decoder)

	var decodedPacket *Packet
	d.On("decoded", func(args ...any) {
		if len(args) > 0 {
			if p, ok := args[0].(*Packet); ok {
				decodedPacket = p
			}
		}
	})

	// BINARY_EVENT with 0 attachments should emit immediately
	err := d.Add(`50-["event","data"]`)
	if err != nil {
		t.Fatalf("Add error: %v", err)
	}

	if decodedPacket == nil {
		t.Fatal("Packet with 0 attachments should be emitted immediately")
	}
}

// TestReservedEventValidation tests that reserved events are rejected
func TestReservedEventValidation(t *testing.T) {
	d := NewDecoder().(*decoder)

	reservedEvents := []string{"connect", "disconnect", "disconnecting", "connect_error"}

	for _, event := range reservedEvents {
		t.Run(event, func(t *testing.T) {
			input := fmt.Sprintf(`2["%s","data"]`, event)
			err := d.Add(input)
			if err == nil {
				t.Errorf("Expected error for reserved event '%s'", event)
			}
		})
	}
}

// TestDecoderWithIOReader tests adding data via io.Reader
func TestDecoderWithIOReader(t *testing.T) {
	d := NewDecoder().(*decoder)

	packet := &Packet{Type: BINARY_EVENT, Attachments: new(uint64), Data: []any{map[string]any{"_placeholder": true, "num": float64(0)}}}
	*packet.Attachments = 1
	d.reconstructor.Store(newBinaryReconstructor(packet))

	// Test with bytes.Reader (implements io.Reader but not io.Closer)
	reader := bytes.NewReader([]byte{0x01, 0x02, 0x03})
	err := d.Add(reader)
	if err != nil {
		t.Errorf("Add with bytes.Reader error = %v", err)
	}
}
