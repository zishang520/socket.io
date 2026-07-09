package engine

import (
	"strings"
	"testing"

	"github.com/zishang520/socket.io/parsers/engine/v3/packet"
)

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
