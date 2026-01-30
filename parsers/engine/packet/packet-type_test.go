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

func TestPacketTypeString(t *testing.T) {
	tests := []struct {
		name     string
		pType    Type
		expected string
	}{
		{"OPEN String()", OPEN, "open"},
		{"CLOSE String()", CLOSE, "close"},
		{"MESSAGE String()", MESSAGE, "message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pType.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestPacketTypeIsValid(t *testing.T) {
	tests := []struct {
		name     string
		pType    Type
		expected bool
	}{
		{"OPEN is valid", OPEN, true},
		{"CLOSE is valid", CLOSE, true},
		{"PING is valid", PING, true},
		{"PONG is valid", PONG, true},
		{"MESSAGE is valid", MESSAGE, true},
		{"UPGRADE is valid", UPGRADE, true},
		{"NOOP is valid", NOOP, true},
		{"ERROR is not valid (not in wire format)", ERROR, false},
		{"invalid type", Type("invalid"), false},
		{"empty type", Type(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.pType.IsValid(); got != tt.expected {
				t.Errorf("IsValid() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewPacket(t *testing.T) {
	t.Run("New creates packet with type and nil data", func(t *testing.T) {
		pkt := New(MESSAGE, nil)
		if pkt.Type != MESSAGE {
			t.Errorf("Type = %v, want %v", pkt.Type, MESSAGE)
		}
		if pkt.Data != nil {
			t.Errorf("Data = %v, want nil", pkt.Data)
		}
		if pkt.Options != nil {
			t.Errorf("Options = %v, want nil", pkt.Options)
		}
	})
}

func TestNewOptions(t *testing.T) {
	t.Run("NewOptions creates options with compress flag", func(t *testing.T) {
		opts := NewOptions(true)
		if opts.Compress == nil {
			t.Error("Compress should not be nil")
		}
		if *opts.Compress != true {
			t.Errorf("Compress = %v, want true", *opts.Compress)
		}
	})

	t.Run("NewOptions with false compress", func(t *testing.T) {
		opts := NewOptions(false)
		if opts.Compress == nil {
			t.Error("Compress should not be nil")
		}
		if *opts.Compress != false {
			t.Errorf("Compress = %v, want false", *opts.Compress)
		}
	})
}

func TestNewWithOptions(t *testing.T) {
	t.Run("NewWithOptions creates packet with all fields", func(t *testing.T) {
		opts := NewOptions(true)
		pkt := NewWithOptions(PING, nil, opts)
		if pkt.Type != PING {
			t.Errorf("Type = %v, want %v", pkt.Type, PING)
		}
		if pkt.Options != opts {
			t.Errorf("Options = %v, want %v", pkt.Options, opts)
		}
	})
}
