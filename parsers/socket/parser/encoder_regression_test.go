package parser

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestEncodeAllowsSharedContainers(t *testing.T) {
	shared := map[string]any{"value": []any{"text", []byte{1}}}
	view := []any{"leaf", nil}
	view[1] = view[:1]
	input := &Packet{Type: EVENT, Data: []any{"event", shared, shared, view}}
	buffers, err := NewEncoder().Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(buffers) != 3 {
		t.Fatalf("buffers=%d, want header and two attachments", len(buffers))
	}
	d := NewDecoder()
	decoded := false
	_ = d.On("decoded", func(args ...any) {
		decoded = true
		data := args[0].(*Packet).Data.([]any)
		for _, item := range data[1:3] {
			values := item.(map[string]any)["value"].([]any)
			if got := values[1].(*types.BytesBuffer).Bytes(); !reflect.DeepEqual(got, []byte{1}) {
				t.Fatalf("attachment=%v", got)
			}
		}
		if !reflect.DeepEqual(data[3], []any{"leaf", []any{"leaf"}}) {
			t.Fatalf("slice view changed: %v", data[3])
		}
	})
	for _, buffer := range buffers {
		if err := d.Add(buffer); err != nil {
			t.Fatal(err)
		}
	}
	if !decoded {
		t.Fatal("missing decoded packet")
	}
}

var errAttachmentRead = errors.New("attachment read failed")

type failingAttachment struct{ closed bool }

func (r *failingAttachment) Read(p []byte) (int, error) { p[0] = 1; return 1, errAttachmentRead }
func (r *failingAttachment) Close() error               { r.closed = true; return nil }

func TestEncodeReaderFailure(t *testing.T) {
	for _, deconstruct := range []bool{false, true} {
		r := &failingAttachment{}
		original := []any{"upload", []byte{2}, r}
		packet := &Packet{Type: EVENT, Data: original}
		var buffers []types.BufferInterface
		var err error
		if deconstruct {
			_, buffers, err = DeconstructPacket(packet)
		} else {
			buffers, err = NewEncoder().Encode(packet)
		}
		if !errors.Is(err, errAttachmentRead) || buffers != nil || !r.closed {
			t.Fatalf("buffers=%v err=%v closed=%v", buffers, err, r.closed)
		}
		if packet.Type != EVENT || packet.Attachments != nil || !reflect.DeepEqual(packet.Data, original) {
			t.Fatal("failed encoding mutated packet")
		}
	}
}

func TestEncodePreservesInputAndNilContainers(t *testing.T) {
	data := []any{"upload", map[string]any{"file": []byte{1}}, []any(nil), map[string]any(nil), strings.NewReader("text")}
	packet := &Packet{Type: EVENT, Data: data}
	buffers, err := NewEncoder().Encode(packet)
	if err != nil {
		t.Fatal(err)
	}
	if got := buffers[0].String(); got != `51-["upload",{"file":{"_placeholder":true,"num":0}},null,null,"text"]` {
		t.Fatal(got)
	}
	if packet.Type != EVENT || packet.Attachments != nil {
		t.Fatal("input packet changed")
	}
	if _, ok := data[1].(map[string]any)["file"].([]byte); !ok {
		t.Fatal("input map changed")
	}
}

func TestEventName(t *testing.T) {
	for _, tc := range []struct {
		value any
		want  string
	}{
		{"x", "x"}, {123.0, "123"}, {math.Copysign(0, -1), "0"}, {1e-6, "0.000001"}, {1e-7, "1e-7"}, {1e20, "100000000000000000000"}, {1e21, "1e+21"},
	} {
		if got := EventName(tc.value); got != tc.want {
			t.Errorf("%v: %s != %s", tc.value, got, tc.want)
		}
	}
}
