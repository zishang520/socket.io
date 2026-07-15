package socket

import (
	"testing"
	"time"

	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/engine/v3"
)

type clientTestConn struct {
	engine.Socket
	protocol int
}

func (c *clientTestConn) Protocol() int {
	return c.protocol
}

type authRecordingNamespace struct {
	Namespace
	name string
	auth map[string]any
}

func (n *authRecordingNamespace) Name() string {
	return n.name
}

func (n *authRecordingNamespace) Add(_ *Client, auth map[string]any, _ func(*Socket)) {
	n.auth = auth
}

func TestClientOnDecodedV3ConnectQueryAuth(t *testing.T) {
	server := NewServer(nil, nil)
	nsp := &authRecordingNamespace{name: "/chat"}
	server._nsps.Store(nsp.name, nsp)
	client := MakeClient()
	client.server = server
	client.conn = &clientTestConn{protocol: 3}

	client.ondecoded(&parser.Packet{
		Type: parser.CONNECT,
		Nsp:  "/chat?token=first&token=second",
	})

	tokens, ok := nsp.auth["token"].([]string)
	if !ok || len(tokens) != 2 || tokens[0] != "first" || tokens[1] != "second" {
		t.Fatalf("Expected query values in auth payload, got %v", nsp.auth)
	}
}

func TestClientOnDecodedV3EventRoutesByNamespacePath(t *testing.T) {
	client := MakeClient()
	client.conn = &clientTestConn{protocol: 3}
	socket := MakeSocket()
	t.Cleanup(socket.taskQueue.Close)
	socket.connected.Store(true)
	client.nsps.Store("/chat", socket)
	received := make(chan []any, 1)
	_ = socket.On("event", func(args ...any) {
		received <- args
	})

	client.ondecoded(&parser.Packet{
		Type: parser.EVENT,
		Nsp:  "/chat?token=ignored",
		Data: []any{"event", "payload"},
	})

	select {
	case args := <-received:
		if len(args) != 1 || args[0] != "payload" {
			t.Fatalf("Expected event payload, got %v", args)
		}
	case <-time.After(time.Second):
		t.Fatal("Timed out waiting for routed event")
	}
}
