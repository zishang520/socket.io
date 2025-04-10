package utils

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"sync/atomic"
)

type base64Id struct {
	sequenceNumber atomic.Uint64
}

var bid = &base64Id{}

func Base64Id() *base64Id {
	return bid
}

func (b *base64Id) GenerateId() (string, error) {
	r := make([]byte, 18)
	if _, err := rand.Read(r); err != nil {
		return "", err
	}
	binary.BigEndian.PutUint64(r[10:], b.sequenceNumber.Add(1)-1)
	return base64.RawURLEncoding.EncodeToString(r), nil
}
