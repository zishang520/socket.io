package transports

import (
	"testing"

	enginepacket "github.com/zishang520/socket.io/parsers/engine/v3/packet"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type cachedPreparedFrameForTransportTest struct {
	types.BufferInterface
	wsBuilds int
	wsValue  any
	wtBuilds int
	wtValue  any
}

func (f *cachedPreparedFrameForTransportTest) PreparedWebSocketFrame(build func(types.BufferInterface) (any, error)) (any, error) {
	if f.wsValue != nil {
		return f.wsValue, nil
	}
	f.wsBuilds++
	value, err := build(f.BufferInterface)
	if err != nil {
		return nil, err
	}
	f.wsValue = value
	return value, nil
}

func (f *cachedPreparedFrameForTransportTest) PreparedWebTransportFrame(build func(types.BufferInterface) (any, error)) (any, error) {
	if f.wtValue != nil {
		return f.wtValue, nil
	}
	f.wtBuilds++
	value, err := build(f.BufferInterface)
	if err != nil {
		return nil, err
	}
	f.wtValue = value
	return value, nil
}

func TestWebSocketPreparedMessageUsesBroadcastCache(t *testing.T) {
	frame := &cachedPreparedFrameForTransportTest{BufferInterface: types.NewStringBufferString("42/test,[\"event\"]")}
	packet := &enginepacket.Packet{
		Type: enginepacket.MESSAGE,
		Data: types.NewStringBufferString("2/test,[\"event\"]"),
		Options: &enginepacket.Options{
			WsPreEncodedFrame: frame,
		},
	}

	first, err := websocketPreparedMessage(packet)
	if err != nil {
		t.Fatalf("unexpected first prepared message error: %v", err)
	}
	second, err := websocketPreparedMessage(packet)
	if err != nil {
		t.Fatalf("unexpected second prepared message error: %v", err)
	}
	if first != second {
		t.Fatal("expected websocket prepared message to be reused from frame cache")
	}
	if frame.wsBuilds != 1 {
		t.Fatalf("expected one websocket prepared message build, got %d", frame.wsBuilds)
	}
}

func TestWebTransportPreparedMessageUsesBroadcastCache(t *testing.T) {
	frame := &cachedPreparedFrameForTransportTest{BufferInterface: types.NewStringBufferString("42/test,[\"event\"]")}
	packet := &enginepacket.Packet{
		Type: enginepacket.MESSAGE,
		Data: types.NewStringBufferString("2/test,[\"event\"]"),
		Options: &enginepacket.Options{
			WsPreEncodedFrame: frame,
		},
	}

	first, err := webTransportPreparedMessage(packet)
	if err != nil {
		t.Fatalf("unexpected first prepared message error: %v", err)
	}
	second, err := webTransportPreparedMessage(packet)
	if err != nil {
		t.Fatalf("unexpected second prepared message error: %v", err)
	}
	if first != second {
		t.Fatal("expected webtransport prepared message to be reused from frame cache")
	}
	if frame.wtBuilds != 1 {
		t.Fatalf("expected one webtransport prepared message build, got %d", frame.wtBuilds)
	}
}
