package parser

import (
	"testing"
)

func TestPacketType_Valid(t *testing.T) {
	tests := []struct {
		name string
		typ  PacketType
		want bool
	}{
		{"Valid CONNECT", CONNECT, true},
		{"Valid DISCONNECT", DISCONNECT, true},
		{"Valid EVENT", EVENT, true},
		{"Valid ACK", ACK, true},
		{"Valid CONNECT_ERROR", CONNECT_ERROR, true},
		{"Valid BINARY_EVENT", BINARY_EVENT, true},
		{"Valid BINARY_ACK", BINARY_ACK, true},
		{"Invalid PacketType", PacketType('7'), false},
		{"Invalid PacketType", PacketType('z'), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.Valid(); got != tt.want {
				t.Errorf("PacketType.Valid() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPacketType_String(t *testing.T) {
	tests := []struct {
		name string
		typ  PacketType
		want string
	}{
		{"CONNECT", CONNECT, "CONNECT"},
		{"DISCONNECT", DISCONNECT, "DISCONNECT"},
		{"EVENT", EVENT, "EVENT"},
		{"ACK", ACK, "ACK"},
		{"CONNECT_ERROR", CONNECT_ERROR, "CONNECT_ERROR"},
		{"BINARY_EVENT", BINARY_EVENT, "BINARY_EVENT"},
		{"BINARY_ACK", BINARY_ACK, "BINARY_ACK"},
		{"UNKNOWN PacketType", PacketType('7'), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.typ.String(); got != tt.want {
				t.Errorf("PacketType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}
