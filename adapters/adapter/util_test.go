package adapter

import (
	"encoding/json"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
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

func TestNormalizeOptions(t *testing.T) {
	flags := &socket.BroadcastFlags{Local: true}
	opts := &PacketOptions{Flags: flags}
	normalized := NormalizeOptions(opts)

	if normalized == opts {
		t.Fatal("NormalizeOptions() returned the input pointer")
	}
	if opts.Rooms != nil || opts.Except != nil {
		t.Fatal("NormalizeOptions() modified the input")
	}
	if normalized.Rooms == nil || normalized.Except == nil {
		t.Fatal("NormalizeOptions() returned nil room slices")
	}
	if normalized.Flags != flags {
		t.Fatal("NormalizeOptions() did not preserve flags")
	}

	normalized = NormalizeOptions(nil)
	if normalized.Rooms == nil || normalized.Except == nil {
		t.Fatal("NormalizeOptions(nil) returned nil room slices")
	}
}

func TestPacketOptionsWireCompatibility(t *testing.T) {
	timeout := int64(750)
	flags := &socket.BroadcastFlags{
		Local:   true,
		Timeout: &timeout,
	}
	flags.Compress = new(false)
	opts := EncodeOptions(&socket.BroadcastOptions{
		Flags: flags,
	})

	formats := []struct {
		name      string
		marshal   func(any) ([]byte, error)
		unmarshal func([]byte, any) error
	}{
		{name: "JSON", marshal: json.Marshal, unmarshal: json.Unmarshal},
		{name: "MessagePack", marshal: msgpack.Marshal, unmarshal: msgpack.Unmarshal},
	}
	for _, format := range formats {
		t.Run(format.name, func(t *testing.T) {
			data, err := format.marshal(opts)
			if err != nil {
				t.Fatal(err)
			}
			if format.name == "JSON" {
				if got, want := string(data), `{"rooms":[],"except":[],"flags":{"compress":false,"local":true,"timeout":750}}`; got != want {
					t.Fatalf("JSON options = %s, want Node.js shape %s", got, want)
				}
			}

			var decoded PacketOptions
			if err := format.unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Rooms == nil || decoded.Except == nil {
				t.Fatalf("wire options = %#v, want empty room arrays", decoded)
			}
			if decoded.Flags == nil || decoded.Flags.Timeout == nil || *decoded.Flags.Timeout != 750 {
				t.Fatalf("wire timeout = %#v, want 750 milliseconds", decoded.Flags)
			}
			if decoded.Flags.Compress == nil || *decoded.Flags.Compress {
				t.Fatalf("decoded compress = %#v, want false", decoded.Flags.Compress)
			}

			runtime := DecodeOptions(&decoded)
			if runtime.Flags == nil || runtime.Flags.Timeout == nil || *runtime.Flags.Timeout != timeout {
				t.Fatalf("decoded timeout = %#v, want %d", runtime.Flags, timeout)
			}
		})
	}
}
