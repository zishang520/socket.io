package adapter

import (
	"testing"

	baseadapter "github.com/zishang520/socket.io/adapters/adapter/v3"
)

type recordingParser struct {
	decodeCalled bool
	packet       *Packet
}

func (p *recordingParser) Encode(any) ([]byte, error) { return nil, nil }

func (p *recordingParser) Decode(_ []byte, value any) error {
	p.decodeCalled = true
	if packet, ok := value.(**Packet); ok {
		*packet = p.packet
	}
	return nil
}

func TestRedisAdapterOnMessageAcceptsNamespaceChannel(t *testing.T) {
	parser := &recordingParser{packet: &Packet{Uid: baseadapter.ServerId("sender")}}
	adapter := MakeRedisAdapter().(*redisAdapter)
	adapter.channel = "socket.io#/#"
	adapter.uid = "sender"
	adapter.parser = parser

	adapter.onMessage(adapter.channel+"*", adapter.channel, []byte("payload"))

	if !parser.decodeCalled {
		t.Fatal("expected namespace channel message to be decoded")
	}
}
