package utils

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"sync/atomic"
	"unicode"
	"unicode/utf8"
)

type base64Id struct {
	sequenceNumber atomic.Uint32
}

var bid = &base64Id{}

func Base64Id() *base64Id {
	return bid
}

func (b *base64Id) GenerateId() string {
	r := make([]byte, 15)
	sequence := b.sequenceNumber.Add(1) - 1
	binary.BigEndian.PutUint32(r[11:], sequence)
	_, _ = rand.Read(r[:12])
	return base64.RawURLEncoding.EncodeToString(r)
}

// IsValidSid checks whether the given session ID has a safe format.
// Valid characters: alphanumeric, '-', '_', '.', '#', ':' (for protocol v3 namespace#id format).
// Maximum length: 36 characters.
func IsValidSid(sid string) bool {
	if len(sid) == 0 || len(sid) > 36 {
		return false
	}
	for i := 0; i < len(sid); i++ {
		c := sid[i]
		if c >= utf8.RuneSelf {
			return isValidUnicodeSid(sid[i:])
		}
		if !isValidASCIISidByte(c) {
			return false
		}
	}
	return true
}

func isValidASCIISidByte(c byte) bool {
	return 'a' <= c && c <= 'z' ||
		'A' <= c && c <= 'Z' ||
		'0' <= c && c <= '9' ||
		c == '-' || c == '_' || c == '.' || c == '#' || c == ':'
}

func isValidUnicodeSid(sid string) bool {
	for _, c := range sid {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '_' && c != '.' && c != '#' && c != ':' {
			return false
		}
	}
	return true
}
