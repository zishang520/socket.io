package parser

import (
	"io"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
)

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestV4EncodePayloadClosesPacketReader(t *testing.T) {
	reader := &trackingReadCloser{Reader: strings.NewReader("ABC")}
	encoded, err := Parserv4().EncodePayload([]*packet.Packet{{
		Type: packet.MESSAGE,
		Data: reader,
	}}, false)
	if err != nil {
		t.Fatalf("EncodePayload() error = %v", err)
	}
	if !reader.closed {
		t.Fatal("EncodePayload() did not close packet reader")
	}
	if actual, expected := encoded.String(), "bQUJD"; actual != expected {
		t.Fatalf("EncodePayload() = %q, want %q", actual, expected)
	}
}
