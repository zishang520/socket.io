package adapter

import (
	"testing"

	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestDecodeOptions(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		opts := DecodeOptions(nil)
		if opts.Rooms == nil || opts.Except == nil {
			t.Fatal("DecodeOptions(nil) returned nil sets")
		}
		if opts.Rooms.Len() != 0 || opts.Except.Len() != 0 || opts.Flags != nil {
			t.Fatal("DecodeOptions(nil) returned non-empty options")
		}
	})

	t.Run("values", func(t *testing.T) {
		flags := &socket.BroadcastFlags{Local: true}
		opts := DecodeOptions(&PacketOptions{
			Rooms:  []socket.Room{"room1", "room1", "room2"},
			Except: []socket.Room{"room3", "room3"},
			Flags:  flags,
		})

		if opts.Rooms.Len() != 2 || !opts.Rooms.Has("room1") || !opts.Rooms.Has("room2") {
			t.Fatal("DecodeOptions() did not decode rooms as a set")
		}
		if opts.Except.Len() != 1 || !opts.Except.Has("room3") {
			t.Fatal("DecodeOptions() did not decode excluded rooms as a set")
		}
		if opts.Flags != flags {
			t.Fatal("DecodeOptions() did not preserve flags")
		}
	})
}
