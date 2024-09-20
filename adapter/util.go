package adapter

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"

	"github.com/zishang520/engine.io/v2/log"
	"github.com/zishang520/engine.io/v2/types"
	"github.com/zishang520/socket.io/v2/socket"
)

var adapter_log = log.NewLog("socket.io-adapter")

// Encode BroadcastOptions into PacketOptions
func EncodeOptions(opts *socket.BroadcastOptions) *PacketOptions {
	return &PacketOptions{
		Rooms:  opts.Rooms.Keys(),  // Convert the set to a slice of strings
		Except: opts.Except.Keys(), // Convert the set to a slice of strings
		Flags:  opts.Flags,         // Pass flags as is
	}
}

// Decode PacketOptions back into BroadcastOptions
func DecodeOptions(opts *PacketOptions) *socket.BroadcastOptions {
	return &socket.BroadcastOptions{
		Rooms:  types.NewSet(opts.Rooms...),  // Convert slice to set
		Except: types.NewSet(opts.Except...), // Convert slice to set
		Flags:  opts.Flags,                   // Pass flags as is
	}
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
