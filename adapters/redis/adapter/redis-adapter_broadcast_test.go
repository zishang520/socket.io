package adapter

import (
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// captureAdapter records broadcasts while every other call uses the real adapter.
type captureAdapter struct {
	socket.Adapter
	packets []*parser.Packet
}

func (c *captureAdapter) Broadcast(packet *parser.Packet, _ *socket.BroadcastOptions) {
	c.packets = append(c.packets, packet)
}

func newCaptureAdapter(t *testing.T) (*redisAdapter, *captureAdapter) {
	t.Helper()
	srv := socket.NewServer(nil, nil)
	t.Cleanup(func() { srv.Close(nil) })
	base := srv.Of("/", nil).Adapter()
	captured := &captureAdapter{Adapter: base}
	return &redisAdapter{
		Adapter: captured,
		channel: "socket.io#/#",
		uid:     "local-node",
		parser:  utils.MsgPack(),
	}, captured
}

func remoteBroadcast(t *testing.T, r *redisAdapter, uid string) []byte {
	t.Helper()
	msg, err := r.parser.Encode(&Packet{
		Uid: adapter.ServerId(uid),
		Packet: &parser.Packet{
			Type: parser.EVENT,
			Nsp:  "/",
			Data: []any{"hello"},
		},
	})
	if err != nil {
		t.Fatalf("encode broadcast: %v", err)
	}
	return msg
}

func TestOnMessageDeliversNamespaceChannel(t *testing.T) {
	r, captured := newCaptureAdapter(t)
	r.Adapter.AddAll(socket.SocketId("s1"), types.NewSet(socket.Room("room-1")))

	r.onMessage("", r.channel, remoteBroadcast(t, r, "remote-node"))
	if len(captured.packets) != 1 {
		t.Fatalf("namespace channel broadcasts = %d, want 1", len(captured.packets))
	}

	r.onMessage("", r.channel+"room-1#", remoteBroadcast(t, r, "remote-node"))
	if len(captured.packets) != 2 {
		t.Fatalf("known room broadcasts = %d, want 2", len(captured.packets))
	}

	before := len(captured.packets)
	r.onMessage("", r.channel+"missing#", remoteBroadcast(t, r, "remote-node"))
	r.onMessage("", "other#/#", remoteBroadcast(t, r, "remote-node"))
	r.onMessage("", r.channel, remoteBroadcast(t, r, string(r.uid)))
	if len(captured.packets) != before {
		t.Fatalf("ignored channels still broadcast, got %d extra", len(captured.packets)-before)
	}
}
