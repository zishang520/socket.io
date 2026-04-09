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

func TestGenerateId_NonEmpty(t *testing.T) {
	id := Base64Id().GenerateId()
	if id == "" {
		t.Fatal("GenerateId() should return a non-empty string")
	}
}

func TestGenerateId_CorrectLength(t *testing.T) {
	// 18 bytes encoded with base64 RawURLEncoding => 18*8/6 = 24 characters
	id := Base64Id().GenerateId()
	if len(id) != 24 {
		t.Fatalf("GenerateId() length = %d, want 24", len(id))
	}
}

func TestGenerateId_ValidBase64RawURL(t *testing.T) {
	id := Base64Id().GenerateId()
	decoded, err := base64.RawURLEncoding.DecodeString(id)
	if err != nil {
		t.Fatalf("GenerateId() produced invalid base64 RawURLEncoding: %v", err)
	}
	if len(decoded) != 18 {
		t.Fatalf("decoded length = %d, want 18", len(decoded))
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

func TestGenerateId_SequenceMonotonicallyIncreasing(t *testing.T) {
	// Create a fresh instance to test sequence behavior
	b := &base64Id{}
	var prevSeq uint64

	for i := range 100 {
		id := b.GenerateId()
		decoded, err := base64.RawURLEncoding.DecodeString(id)
		if err != nil {
			t.Fatalf("invalid base64: %v", err)
		}
		// Last 8 bytes contain the sequence number in big-endian
		seq := binaryBigEndianUint64(decoded[10:])
		if i == 0 {
			if seq != 0 {
				t.Fatalf("first sequence = %d, want 0", seq)
			}
		} else {
			if seq != prevSeq+1 {
				t.Fatalf("sequence not monotonic: got %d, want %d", seq, prevSeq+1)
			}
		}
		prevSeq = seq
	}
}

func TestGenerateId_RandomPrefixDiffers(t *testing.T) {
	// Two IDs generated close together should have different random prefixes
	b := &base64Id{}
	id1 := b.GenerateId()
	id2 := b.GenerateId()

	d1, _ := base64.RawURLEncoding.DecodeString(id1)
	d2, _ := base64.RawURLEncoding.DecodeString(id2)

	// The first 10 bytes are random and should differ
	prefixSame := true
	for i := range 10 {
		if d1[i] != d2[i] {
			prefixSame = false
			break
		}
	}
	if prefixSame {
		t.Fatal("random prefixes of two IDs should almost certainly differ")
	}
}

// binaryBigEndianUint64 decodes a uint64 from big-endian bytes.
func binaryBigEndianUint64(b []byte) uint64 {
	_ = b[7] // bounds check
	return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 |
		uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7])
}
