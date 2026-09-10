package parser

import (
	"bytes"
	"testing"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestDecoderDestroyAndReuse(t *testing.T) {
	d := NewDecoder()
	if err := d.Add(`51-["x",{"_placeholder":true,"num":0}]`); err != nil {
		t.Fatal(err)
	}
	d.Destroy()
	if err := d.Add(`2["next"]`); err != nil {
		t.Fatal(err)
	}
}

func TestRejectMalformedPackets(t *testing.T) {
	for _, wire := range []string{`0null`, `1null`, `2["x"]junk`, `2["x"][]`, `50-["x"]`} {
		if err := NewDecoder().Add(wire); err == nil {
			t.Errorf("accepted %s", wire)
		}
	}
	for _, index := range []string{``, `,"num":"0"`, `,"num":-1`, `,"num":0.5`, `,"num":1`, `,"num":1e100`} {
		d := NewDecoder()
		if err := d.Add(`51-["x",{"_placeholder":true` + index + `}]`); err != nil {
			t.Fatal(err)
		}
		if err := d.Add([]byte{1}); err == nil {
			t.Errorf("accepted index %s", index)
		}
		d.Destroy()
		if err := d.Add(`2["next"]`); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBinaryPacketNormalization(t *testing.T) {
	for _, wire := range []string{`51-["x",{"_placeholder":true,"num":0}]`, `61-[{"_placeholder":true,"num":0}]`} {
		d := NewDecoder()
		var got *Packet
		_ = d.On("decoded", func(args ...any) { got = args[0].(*Packet) })
		if err := d.Add(wire); err != nil {
			t.Fatal(err)
		}
		if err := d.Add([]byte{1}); err != nil {
			t.Fatal(err)
		}
		want := EVENT
		if wire[0] == '6' {
			want = ACK
		}
		if got == nil || got.Type != want {
			t.Fatalf("decoded=%v", got)
		}
		encoded, err := NewEncoder().Encode(got)
		if err != nil {
			t.Fatal(err)
		}
		if len(encoded) != 2 || encoded[0].String() != wire || !bytes.Equal(encoded[1].Bytes(), []byte{1}) {
			t.Fatalf("encoded=%v", encoded)
		}
		if err := d.Add(`2["next"]`); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDirectBinaryRoundTripAndNilContainers(t *testing.T) {
	p, b, err := DeconstructPacket(&Packet{Type: EVENT, Data: []any{"x", []byte{1}, []any(nil), map[string]any(nil)}})
	if err != nil {
		t.Fatal(err)
	}
	p, err = ReconstructPacket(p, b)
	if err != nil {
		t.Fatal(err)
	}
	data := p.Data.([]any)
	if buf, ok := data[1].(types.BufferInterface); !ok || !bytes.Equal(buf.Bytes(), []byte{1}) {
		t.Fatalf("attachment=%T", data[1])
	}
	if data[2].([]any) != nil || data[3].(map[string]any) != nil {
		t.Fatal("nil containers changed")
	}
}

func TestDecoderIDLengthBoundary(t *testing.T) {
	o := DefaultDecoderOptions()
	o.SetMaxPacketIDLength(3)
	if err := NewDecoder(o).Add(`2123["x"]`); err != nil {
		t.Fatal(err)
	}
	if err := NewDecoder(o).Add(`21234["x"]`); err == nil {
		t.Fatal("accepted oversized ID")
	}
	for _, n := range []int{0, -1} {
		o.SetMaxPacketIDLength(n)
		o.SetMaxNamespaceLength(n)
		if err := NewDecoder(o).Add(`2/chat,123["x"]`); err != nil {
			t.Fatal(err)
		}
	}
	if err := NewDecoder().Add(`218446744073709551615["x"]`); err != nil {
		t.Fatal(err)
	}
	if err := NewDecoder().Add(`218446744073709551616["x"]`); err == nil {
		t.Fatal("accepted uint64 overflow")
	}
}
