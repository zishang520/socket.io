package adapter

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"

	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

var adapterLog = log.NewLog("socket.io-adapter")

// EncodeOptions encodes BroadcastOptions into PacketOptions.
func EncodeOptions(opts *socket.BroadcastOptions) *PacketOptions {
	p := &PacketOptions{}
	if opts == nil {
		return p
	}

	if opts.Rooms != nil {
		p.Rooms = opts.Rooms.Keys() // Convert the set to a slice of strings
	}
	if opts.Except != nil {
		p.Except = opts.Except.Keys() // Convert the set to a slice of strings
	}
	if opts.Flags != nil {
		p.Flags = opts.Flags // Pass flags as is
	}
	return p
}

// DecodeOptions decodes PacketOptions back into BroadcastOptions.
func DecodeOptions(opts *PacketOptions) *socket.BroadcastOptions {
	b := &socket.BroadcastOptions{
		Rooms:  types.NewSet[socket.Room](),
		Except: types.NewSet[socket.Room](),
	}
	if opts == nil {
		return b
	}

	b.Rooms.Add(opts.Rooms...)   // Convert slice to set
	b.Except.Add(opts.Except...) // Convert slice to set
	b.Flags = opts.Flags         // Pass flags as is

	return b
}

// RandomId generates a random hexadecimal string of 8 bytes.
func RandomId() string {
	r := make([]byte, 8)
	// Read fills b with cryptographically secure random bytes. It never returns an
	// error, and always fills b entirely.
	_, _ = rand.Read(r)
	return hex.EncodeToString(r)
}

// Uid2 generates a random URL-safe base64 string of the given length.
func Uid2(length int) string {
	r := make([]byte, length)
	// Read fills b with cryptographically secure random bytes. It never returns an
	// error, and always fills b entirely.
	_, _ = rand.Read(r)
	return base64.RawURLEncoding.EncodeToString(r)
}
