package parser

import (
	"bytes"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// These frames follow engine.io/lib/parser-v3's byte-oriented binary payload.
func TestV3NodeBinaryPayload(t *testing.T) {
	for _, text := range []string{"ascii", "é", "中文", "😀"} {
		t.Run(text, func(t *testing.T) {
			frame := append([]byte{0, byte(1 + len(text)), 255, '4'}, []byte(text)...)
			frame = append(frame, 1, 2, 255, 4, 1)
			encoded, err := Parserv3().EncodePayload([]*packet.Packet{
				{Type: packet.MESSAGE, Data: strings.NewReader(text)},
				{Type: packet.MESSAGE, Data: types.NewBytesBuffer([]byte{1})},
			}, true)
			if err != nil || !bytes.Equal(encoded.Bytes(), frame) {
				t.Fatalf("encoded = %v, %v; want %v", encoded, err, frame)
			}
			decoded, err := Parserv3().DecodePayload(types.NewBytesBuffer(frame))
			if err != nil || len(decoded) != 2 {
				t.Fatalf("decoded = %v, %v", decoded, err)
			}
			if decoded[0].Data.(*types.StringBuffer).String() != text || !bytes.Equal(decoded[1].Data.(*types.BytesBuffer).Bytes(), []byte{1}) {
				t.Fatalf("incorrect decoded payload: %v", decoded)
			}
		})
	}
}

func TestV3NodeTextPayloadLengths(t *testing.T) {
	for _, tc := range []struct{ text, wire string }{{"é", "2:4é1:2"}, {"中文", "3:4中文1:2"}, {"😀", "3:4😀1:2"}} {
		encoded, err := Parserv3().EncodePayload([]*packet.Packet{{Type: packet.MESSAGE, Data: strings.NewReader(tc.text)}, {Type: packet.PING}})
		if err != nil || encoded.String() != tc.wire {
			t.Fatalf("encode = %v, %v", encoded, err)
		}
		decoded, err := Parserv3().DecodePayload(types.NewStringBufferString(tc.wire))
		if err != nil || len(decoded) != 2 || decoded[1].Type != packet.PING {
			t.Fatalf("decode = %v, %v", decoded, err)
		}
	}
}

func TestV3RejectIncompleteFrames(t *testing.T) {
	for _, wire := range []types.BufferInterface{types.NewStringBufferString("2:4😀"), types.NewBytesBuffer([]byte{1, 5, 255, 4, 1})} {
		if _, err := Parserv3().DecodePayload(wire); err == nil {
			t.Fatal("accepted incomplete frame")
		}
	}
}

func TestV4PayloadStopsAtInvalidSegment(t *testing.T) {
	for _, tc := range []struct {
		wire   string
		prefix int
	}{{"", 0}, {"\x1e4ok", 0}, {"4ok\x1e", 1}, {"4ok\x1e\x1e4later", 1}, {"4ok\x1ex\x1e4later", 1}} {
		packets, err := Parserv4().DecodePayload(types.NewStringBufferString(tc.wire))
		if err == nil || len(packets) != tc.prefix {
			t.Errorf("%q: packets=%d err=%v", tc.wire, len(packets), err)
		}
	}
}

func TestV3NodeUTF8Validation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		raw     []byte
		want    string
		invalid bool
	}{
		{"overlong", []byte{0xc0, 0xaf}, "", true},
		{"truncated", []byte{0xe2, 0x82}, "", true},
		{"continuation", []byte{0xe2, 0x28, 0xa1}, "", true},
		{"out-of-range", []byte{0xf4, 0x90, 0x80, 0x80}, "", true},
		{"surrogate", []byte{0xed, 0xa0, 0x80}, "�", false},
		{"replacement", []byte{0xef, 0xbf, 0xbd}, "�", false},
		{"emoji", []byte("字符❤️🧡💛🧓🏾💟"), "字符❤️🧡💛🧓🏾💟", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Run("binary-payload", func(t *testing.T) {
				n := len(tc.raw) + 1
				header := []byte{0}
				if n >= 10 {
					header = append(header, byte(n/10))
				}
				header = append(header, byte(n%10), 255, '4')
				packets, err := Parserv3().DecodePayload(types.NewBytesBuffer(append(header, tc.raw...)))
				if tc.invalid {
					if err == nil {
						t.Fatal("accepted malformed UTF-8")
					}
					return
				}
				if err != nil || len(packets) != 1 {
					t.Fatalf("packets=%v err=%v", packets, err)
				}
				if got := packets[0].Data.(*types.StringBuffer).String(); got != tc.want {
					t.Fatalf("got %q want %q", got, tc.want)
				}
			})
			t.Run("explicit-byte-string", func(t *testing.T) {
				encoded := append([]byte{'4'}, utils.Utf8encodeBytes(tc.raw)...)
				pkt, err := Parserv3().DecodePacket(types.NewStringBuffer(encoded), true)
				if tc.invalid {
					if err == nil {
						t.Fatal("accepted malformed UTF-8")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if got := pkt.Data.(*types.StringBuffer).String(); got != tc.want {
					t.Fatalf("got %q want %q", got, tc.want)
				}
			})
		})
	}
}

func TestV3LongByteString(t *testing.T) {
	want := strings.Repeat("aé", 400)
	wire := types.NewStringBuffer(append([]byte{'4'}, utils.Utf8encodeBytes([]byte(want))...))
	pkt, err := Parserv3().DecodePacket(wire, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := pkt.Data.(*types.StringBuffer).String(); got != want {
		t.Fatalf("long text corrupted: got %d bytes want %d", len(got), len(want))
	}
}
