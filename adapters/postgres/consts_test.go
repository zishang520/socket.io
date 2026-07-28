package postgres

import (
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
)

func TestClusterProtocolConstants(t *testing.T) {
	constants := []struct {
		name string
		got  adapter.MessageType
		want adapter.MessageType
	}{
		{"INITIAL_HEARTBEAT", INITIAL_HEARTBEAT, adapter.INITIAL_HEARTBEAT},
		{"HEARTBEAT", HEARTBEAT, adapter.HEARTBEAT},
		{"BROADCAST", BROADCAST, adapter.BROADCAST},
		{"SOCKETS_JOIN", SOCKETS_JOIN, adapter.SOCKETS_JOIN},
		{"SOCKETS_LEAVE", SOCKETS_LEAVE, adapter.SOCKETS_LEAVE},
		{"DISCONNECT_SOCKETS", DISCONNECT_SOCKETS, adapter.DISCONNECT_SOCKETS},
		{"FETCH_SOCKETS", FETCH_SOCKETS, adapter.FETCH_SOCKETS},
		{"FETCH_SOCKETS_RESPONSE", FETCH_SOCKETS_RESPONSE, adapter.FETCH_SOCKETS_RESPONSE},
		{"SERVER_SIDE_EMIT", SERVER_SIDE_EMIT, adapter.SERVER_SIDE_EMIT},
		{"SERVER_SIDE_EMIT_RESPONSE", SERVER_SIDE_EMIT_RESPONSE, adapter.SERVER_SIDE_EMIT_RESPONSE},
		{"BROADCAST_CLIENT_COUNT", BROADCAST_CLIENT_COUNT, adapter.BROADCAST_CLIENT_COUNT},
		{"BROADCAST_ACK", BROADCAST_ACK, adapter.BROADCAST_ACK},
		{"ADAPTER_CLOSE", ADAPTER_CLOSE, adapter.ADAPTER_CLOSE},
	}
	for _, constant := range constants {
		if constant.got != constant.want {
			t.Errorf("%s = %d, want %d", constant.name, constant.got, constant.want)
		}
	}
	if EMITTER_UID != adapter.EMITTER_UID {
		t.Errorf("EMITTER_UID = %q, want %q", EMITTER_UID, adapter.EMITTER_UID)
	}
}
