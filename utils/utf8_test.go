package utils

import (
	"bytes"
	"testing"
)

func TestUtf8Encode(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []byte
	}{
		{
			name:     "ASCII characters",
			input:    "Hello",
			expected: []byte("Hello"),
		},
		{
			name:     "Chinese characters",
			input:    "你好",
			expected: []byte{195, 164, 194, 189, 194, 160, 195, 165, 194, 165, 194, 189},
		},
		{
			name:     "Emojis",
			input:    "👋🌍",
			expected: []byte{195, 176, 194, 159, 194, 145, 194, 139, 195, 176, 194, 159, 194, 140, 194, 141},
		},
		{
			name:     "Mixed content",
			input:    "Hello 你好 👋",
			expected: []byte{72, 101, 108, 108, 111, 32, 195, 164, 194, 189, 194, 160, 195, 165, 194, 165, 194, 189, 32, 195, 176, 194, 159, 194, 145, 194, 139},
		},
		{
			name:     "Empty string",
			input:    "",
			expected: []byte{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Utf8encodeString(tt.input)
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Errorf("Utf8Encode() = %v, want %v", []byte(got), tt.expected)
					return
				}
			}
		})
	}
}

func TestUtf8Decode(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
	}{
		{
			name:     "ASCII characters",
			input:    []byte("Hello"),
			expected: []byte{72, 101, 108, 108, 111},
		},
		{
			name:     "Chinese characters",
			input:    []byte{0xE4, 0xBD, 0xA0, 0xE5, 0xA5, 0xBD},
			expected: []byte{195, 164, 194, 189, 194, 160, 195, 165, 194, 165, 194, 189},
		},
		{
			name:     "Emojis",
			input:    []byte{0xF0, 0x9F, 0x91, 0x8B, 0xF0, 0x9F, 0x8C, 0x8D},
			expected: []byte{195, 176, 194, 159, 194, 145, 194, 139, 195, 176, 194, 159, 194, 140, 194, 141},
		},
		{
			name:     "Mixed content",
			input:    []byte{0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x20, 0xE4, 0xBD, 0xA0, 0xE5, 0xA5, 0xBD, 0x20, 0xF0, 0x9F, 0x91, 0x8B},
			expected: []byte{72, 101, 108, 108, 111, 32, 195, 164, 194, 189, 194, 160, 195, 165, 194, 165, 194, 189, 32, 195, 176, 194, 159, 194, 145, 194, 139},
		},
		{
			name:     "Empty bytes",
			input:    []byte{},
			expected: []byte(""),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Utf8encodeBytes(tt.input)
			if !bytes.Equal(got, tt.expected) {
				t.Errorf("Utf8Decode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestUtf8DecodeInvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "Invalid UTF-8 sequence",
			input: []byte{0xFF, 0xFF, 0xFF},
		},
		{
			name:  "Incomplete UTF-8 sequence",
			input: []byte{0xE4, 0xBD}, // Incomplete Chinese character
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Utf8decodeBytes(tt.input)
			if len(got) == 0 {
				t.Error("Utf8Decode() should return replacement character for invalid input")
			}
		})
	}
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"ASCII", "Hello World"},
		{"Chinese", "你好世界"},
		{"Emojis", "👋🌍🎉"},
		{"Mixed", "Hello 你好 👋 World"},
		{"Empty", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := Utf8encodeString(tt.input)
			decoded := Utf8decodeString(encoded)
			if decoded != tt.input {
				t.Errorf("Round trip failed: got %v, want %v", decoded, tt.input)
			}
		})
	}
}
