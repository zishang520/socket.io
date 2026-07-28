package adapter

import (
	"testing"

	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

func TestHeartbeatServerSideEmitResponseStoresScalarPacket(t *testing.T) {
	cluster := MakeClusterAdapterWithHeartbeat().(*clusterAdapterWithHeartbeat)
	request := &CustomClusterRequest{
		MissingUids: types.NewSet[ServerId]("node", "other"),
		Responses:   types.NewSlice[any](),
	}
	cluster.customRequests.Store("request", request)

	cluster.OnResponse(&ClusterResponse{
		Uid:  "node",
		Type: SERVER_SIDE_EMIT_RESPONSE,
		Data: &ServerSideEmitResponse{
			RequestId: "request",
			Packet:    []any{"response"},
		},
	})

	responses := request.Responses.All()
	if len(responses) != 1 || responses[0] != "response" {
		t.Fatalf("expected scalar response, got %#v", responses)
	}
}

func TestClusterAdapterWithHeartbeatBuilder(t *testing.T) {
	builder := &ClusterAdapterWithHeartbeatBuilder{
		Opts: nil,
	}

	cluster := builder.New(socket.NewNamespace(socket.NewServer(nil, nil), "/test"))
	t.Cleanup(cluster.Close)
}
