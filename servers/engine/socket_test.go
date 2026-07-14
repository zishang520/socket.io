package engine

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/servers/engine/v3/transports"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestOpenPacketJSON(t *testing.T) {
	data, err := json.Marshal(openPacket{
		MaxPayload:   1_000_000,
		PingInterval: 25_000,
		PingTimeout:  20_000,
		Sid:          "lv_VI97HAXpY6yYWAAAC",
		Upgrades:     []string{"websocket"},
	})
	if err != nil {
		t.Fatal(err)
	}
	const expected = `{"maxPayload":1000000,"pingInterval":25000,"pingTimeout":20000,"sid":"lv_VI97HAXpY6yYWAAAC","upgrades":["websocket"]}`
	if string(data) != expected {
		t.Fatalf("open packet = %s, want %s", data, expected)
	}
}

func TestSendPacketCopiesOptions(t *testing.T) {
	socket := MakeSocket().(*socket)
	socket.readyState.Store("open")
	transport := transports.MakeTransport()
	socket.transport.Store(new(transport))
	frame := types.NewStringBufferString("prepared frame")
	compress := false
	options := &packet.Options{Compress: &compress, WsPreEncodedFrame: frame}

	socket.sendPacket(packet.MESSAGE, strings.NewReader("payload"), options, nil)
	packets := socket.writeBuffer.AllAndClear()
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	queued := packets[0].Options
	if queued == options || queued.Compress == options.Compress {
		t.Fatal("packet options were not copied")
	}
	if *queued.Compress || queued.WsPreEncodedFrame != frame {
		t.Fatal("copied packet options do not preserve their values")
	}

	compress = true
	if *queued.Compress {
		t.Fatal("queued compression option changed with the caller's value")
	}
}

func TestIsProbePingPacket(t *testing.T) {
	tests := []struct {
		name string
		pkt  *packet.Packet
		want bool
	}{
		{
			name: "probe ping",
			pkt:  &packet.Packet{Type: packet.PING, Data: strings.NewReader("probe")},
			want: true,
		},
		{
			name: "non ping probe",
			pkt:  &packet.Packet{Type: packet.MESSAGE, Data: strings.NewReader("probe")},
			want: false,
		},
		{
			name: "ping with different data",
			pkt:  &packet.Packet{Type: packet.PING, Data: strings.NewReader("other")},
			want: false,
		},
		{
			name: "ping with extra bytes",
			pkt:  &packet.Packet{Type: packet.PING, Data: strings.NewReader("probe-extra")},
			want: false,
		},
		{
			name: "ping with nil data",
			pkt:  &packet.Packet{Type: packet.PING},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProbePingPacket(tt.pkt); got != tt.want {
				t.Fatalf("isProbePingPacket() = %v, want %v", got, tt.want)
			}
		})
	}
}
