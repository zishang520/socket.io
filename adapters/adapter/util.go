package adapter

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"

	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var adapterLog = log.NewLog("socket.io-adapter")

// EncodeOptions encodes BroadcastOptions into PacketOptions.
func EncodeOptions(opts *socket.BroadcastOptions) *PacketOptions {
	p := &PacketOptions{
		Rooms:  []socket.Room{},
		Except: []socket.Room{},
	}
	if opts == nil {
		return p
	}

	if opts.Rooms != nil {
		p.Rooms = utils.NonNilSlice(opts.Rooms.Keys())
	}
	if opts.Except != nil {
		p.Except = utils.NonNilSlice(opts.Except.Keys())
	}
	p.Flags = opts.Flags
	return p
}

// NormalizeOptions returns a shallow copy with non-nil room slices.
func NormalizeOptions(opts *PacketOptions) *PacketOptions {
	options := new(PacketOptions)
	if opts != nil {
		*options = *opts
	}
	options.Rooms = utils.NonNilSlice(options.Rooms)
	options.Except = utils.NonNilSlice(options.Except)
	return options
}

// DecodeOptions decodes PacketOptions back into BroadcastOptions.
func DecodeOptions(opts *PacketOptions) *socket.BroadcastOptions {
	if opts == nil {
		return &socket.BroadcastOptions{
			Rooms:  types.NewSet[socket.Room](),
			Except: types.NewSet[socket.Room](),
		}
	}

	return &socket.BroadcastOptions{
		Rooms:  types.NewSet(opts.Rooms...),
		Except: types.NewSet(opts.Except...),
		Flags:  opts.Flags,
	}
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
