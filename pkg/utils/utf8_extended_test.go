package utils

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestUtf16Len(t *testing.T) {
	tests := []struct {
		name     string
		r        rune
		expected int
	}{
		{"ASCII 'A'", 'A', 1},
		{"ASCII '0'", '0', 1},
		{"CJK character", '你', 1},
		{"surrogate boundary low", rune(0xD7FF), 1},
		{"BMP after surrogates", rune(0xE000), 1},
		{"last BMP codepoint", rune(0xFFFF), 1},
		{"first supplementary (emoji)", '😀', 2},
		{"musical symbol", '𝄞', 2},
		{"max rune", rune(0x10FFFF), 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Utf16Len(tt.r)
			if got != tt.expected {
				t.Errorf("Utf16Len(%U) = %d, want %d", tt.r, got, tt.expected)
			}
		})
	}
}

func TestUtf16Count(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty", "", 0},
		{"ASCII", "Hello", 5},
		{"CJK", "你好", 2},
		{"emoji (surrogate pair)", "😀", 2},
		{"mixed", "Hi😀你", 5}, // H=1, i=1, 😀=2, 你=1
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Utf16Count([]byte(tt.input))
			if got != tt.expected {
				t.Errorf("Utf16Count(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestUtf16CountString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"empty", "", 0},
		{"ASCII", "Hello", 5},
		{"emoji", "👋🌍", 4},  // each emoji is 2 UTF-16 code units
		{"mixed", "A😀B", 4}, // A=1, 😀=2, B=1
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Utf16CountString(tt.input)
			if got != tt.expected {
				t.Errorf("Utf16CountString(%q) = %d, want %d", tt.input, got, tt.expected)
			}
		})
	}
}

func TestUtf8decodeString(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"ASCII", "Hello World"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Utf8decodeString(tt.input)
			if got != tt.input {
				t.Errorf("Utf8decodeString(%q) = %q, want %q", tt.input, got, tt.input)
			}
		})
	}
}

func TestNewUtf8Encoder(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"ASCII", []byte("Hello")},
		{"high bytes", []byte{0xC0, 0xFF, 0x80}},
		{"empty", []byte{}},
		{"single byte", []byte{0x41}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			enc := NewUtf8Encoder(&buf)
			n, err := enc.Write(tt.input)
			if err != nil {
				t.Fatalf("Write error: %v", err)
			}
			if n != len(tt.input) {
				t.Errorf("Write returned n=%d, want %d", n, len(tt.input))
			}
			// Verify output matches Utf8encodeBytes
			expected := Utf8encodeBytes(tt.input)
			if !bytes.Equal(buf.Bytes(), expected) {
				t.Errorf("Encoder output %v, want %v", buf.Bytes(), expected)
			}
		})
	}
}

func TestNewUtf8EncoderLargeInput(t *testing.T) {
	// Test with input larger than bufferSize/2 to exercise chunking
	input := bytes.Repeat([]byte{0xC0, 0x41}, 600)
	var buf bytes.Buffer
	enc := NewUtf8Encoder(&buf)
	n, err := enc.Write(input)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write returned n=%d, want %d", n, len(input))
	}
	expected := Utf8encodeBytes(input)
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Error("Large encoder output mismatch")
	}
}

func TestNewUtf8Decoder(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"ASCII", "Hello World"},
		{"empty", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded := Utf8encodeString(tt.input)
			dec := NewUtf8Decoder(strings.NewReader(encoded))
			result, err := io.ReadAll(dec)
			if err != nil {
				t.Fatalf("ReadAll error: %v", err)
			}
			if string(result) != tt.input {
				t.Errorf("Decoder output %q, want %q", string(result), tt.input)
			}
		})
	}
}

func TestNewUtf8DecoderRoundTrip(t *testing.T) {
	originals := []string{
		"Hello World",
		"你好世界",
		"Mixed 混合 Content",
	}
	for _, orig := range originals {
		t.Run(orig, func(t *testing.T) {
			// Encode via encoder
			var encoded bytes.Buffer
			enc := NewUtf8Encoder(&encoded)
			if _, err := enc.Write([]byte(orig)); err != nil {
				t.Fatalf("Encode error: %v", err)
			}

			// Decode via decoder
			dec := NewUtf8Decoder(&encoded)
			result, err := io.ReadAll(dec)
			if err != nil {
				t.Fatalf("Decode error: %v", err)
			}
			if string(result) != orig {
				t.Errorf("Round trip: got %q, want %q", string(result), orig)
			}
		})
	}
}

func TestNewUtf8DecoderEmptyRead(t *testing.T) {
	dec := NewUtf8Decoder(strings.NewReader(""))
	p := make([]byte, 0)
	n, err := dec.Read(p)
	if n != 0 || err != nil {
		t.Errorf("Read(empty buf) = %d, %v; want 0, nil", n, err)
	}
}
