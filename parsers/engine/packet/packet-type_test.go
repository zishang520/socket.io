package packet

import (
	"testing"
)

func TestPacketType(t *testing.T) {
	tests := []struct {
		name     string
		pType    Type
		expected string
	}{
		{"OPEN packet", OPEN, "open"},
		{"CLOSE packet", CLOSE, "close"},
		{"PING packet", PING, "ping"},
		{"PONG packet", PONG, "pong"},
		{"MESSAGE packet", MESSAGE, "message"},
		{"UPGRADE packet", UPGRADE, "upgrade"},
		{"NOOP packet", NOOP, "noop"},
		{"ERROR packet", ERROR, "error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pType; string(got) != tt.expected {
				t.Errorf("Type = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPacketTypeFromString(t *testing.T) {
	tests := []struct {
		name     string
		str      string
		expected Type
	}{
		{"open string", "open", OPEN},
		{"close string", "close", CLOSE},
		{"ping string", "ping", PING},
		{"pong string", "pong", PONG},
		{"message string", "message", MESSAGE},
		{"upgrade string", "upgrade", UPGRADE},
		{"noop string", "noop", NOOP},
		{"error string", "error", ERROR},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Type(tt.str); got != tt.expected {
				t.Errorf("From = %v, want %v", got, tt.expected)
			}
		})
	}
}
