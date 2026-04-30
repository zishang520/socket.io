package utils

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"sync/atomic"
	"unicode"
)

type base64Id struct {
	sequenceNumber atomic.Uint64
}

var bid = &base64Id{}

func Base64Id() *base64Id {
	return bid
}

func (b *base64Id) GenerateId() string {
	r := make([]byte, 18)
	// Read fills b with cryptographically secure random bytes. It never returns an
	// error, and always fills b entirely.
	_, _ = rand.Read(r)
	binary.BigEndian.PutUint64(r[10:], b.sequenceNumber.Add(1)-1)
	return base64.RawURLEncoding.EncodeToString(r)
}

// IsValidSid checks whether the given session ID has a safe format.
// Valid characters: alphanumeric, '-', '_', '.', '#', ':' (for protocol v3 namespace#id format).
// Maximum length: 36 characters.
func IsValidSid(sid string) bool {
	if len(sid) == 0 || len(sid) > 36 {
		return false
	}
	for _, c := range sid {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '-' && c != '_' && c != '.' && c != '#' && c != ':' {
			return false
		}
	}
	return true
}
