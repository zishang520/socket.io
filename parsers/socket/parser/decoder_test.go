package parser

import (
	"bytes"
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
		{"Invalid EVENT packet", "2[null]", true},
		{"Invalid EVENT packet", "2[0]", true},
		{"Invalid EVENT packet", "2[1.1]", true},
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
		{"Invalid EVENT (reserved)", EVENT, []any{"connect", "data"}, false},
		{"Invalid EVENT (not array)", EVENT, "not array", false},
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
