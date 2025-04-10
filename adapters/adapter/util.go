package adapter

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"

	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

var adapter_log = log.NewLog("socket.io-adapter")

// Encode BroadcastOptions into PacketOptions
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

// Decode PacketOptions back into BroadcastOptions
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

func RandomId() (string, error) {
	r := make([]byte, 8)
	if _, err := rand.Read(r); err != nil {
		return "", err
	}
	return hex.EncodeToString(r), nil
}

func Uid2(length int) (string, error) {
	r := make([]byte, length)
	if _, err := rand.Read(r); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(r), nil
}

func SliceMap[I any, O any](i []I, converter func(I) O) (o []O) {
	for _, _i := range i {
		o = append(o, converter(_i))
	}
	return o
}

// Tap calls the given function with the given value, then returns the value.
func Tap[T any](value T, callback func(T)) T {
	if callback != nil {
		callback(value)
	}
	return value
}
