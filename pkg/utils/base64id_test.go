package utils

import (
	"encoding/base64"
	"testing"
)

func TestBase64Id_Singleton(t *testing.T) {
	a := Base64Id()
	b := Base64Id()
	if a != b {
		t.Fatal("Base64Id() should return the same singleton instance")
	}
}

func TestGenerateId_CorrectLength(t *testing.T) {
	// 15 bytes encoded with base64 RawURLEncoding produce 20 characters.
	id := Base64Id().GenerateId()
	if len(id) != 20 {
		t.Fatalf("GenerateId() length = %d, want 20", len(id))
	}
}

func TestGenerateId_ValidBase64RawURL(t *testing.T) {
	id := Base64Id().GenerateId()
	decoded, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		t.Fatalf("GenerateId() produced invalid base64 RawURLEncoding: %v", err)
	}
	if len(decoded) != 15 {
		t.Fatalf("decoded length = %d, want 15", len(decoded))
	}
}

func TestGenerateId_Uniqueness(t *testing.T) {
	seen := make(map[string]struct{})
	b := Base64Id()
	for range 1000 {
		id := b.GenerateId()
		if _, exists := seen[id]; exists {
			t.Fatalf("GenerateId() produced duplicate id: %s", id)
		}
		seen[id] = struct{}{}
	}
}

func TestGenerateId_NodeSequenceSuffix(t *testing.T) {
	for _, test := range []struct {
		sequence uint32
		want     uint32
	}{
		{0, 0},
		{1, 1},
		{1<<24 - 1, 1<<24 - 1},
		{1 << 24, 0},
	} {
		b := &base64Id{}
		b.sequenceNumber.Store(test.sequence)
		id := b.GenerateId()
		decoded, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil {
			t.Fatalf("invalid base64: %v", err)
		}
		seq := uint32(decoded[12])<<16 | uint32(decoded[13])<<8 | uint32(decoded[14])
		if seq != test.want {
			t.Fatalf("sequence suffix = %d, want %d", seq, test.want)
		}
	}
}

func TestGenerateId_RandomPrefixDiffers(t *testing.T) {
	// Two IDs generated close together should have different random prefixes
	b := &base64Id{}
	id1 := b.GenerateId()
	id2 := b.GenerateId()

	d1, _ := base64.RawURLEncoding.DecodeString(id1)
	d2, _ := base64.RawURLEncoding.DecodeString(id2)

	// The first 12 bytes are random and should differ.
	prefixSame := true
	for i := range 12 {
		if d1[i] != d2[i] {
			prefixSame = false
			break
		}
	}
	if prefixSame {
		t.Fatal("random prefixes of two IDs should almost certainly differ")
	}
}

func TestIsValidSid(t *testing.T) {
	tests := map[string]bool{
		"yH8rZp1uWq3xA7cN9mK2vB4d":              true,
		"namespace#socket:id.test":              true,
		"会话编号42":                                true,
		"":                                      false,
		"contains space":                        false,
		"contains/slash":                        false,
		string([]byte{0xff}):                    false,
		"abcdefghijklmnopqrstuvwxyzABCDEFGHIJK": false,
	}

	for sid, expected := range tests {
		if actual := IsValidSid(sid); actual != expected {
			t.Errorf("IsValidSid(%q) = %t, want %t", sid, actual, expected)
		}
	}
}
