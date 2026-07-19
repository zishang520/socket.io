package adapter

import (
	"encoding/json"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

func TestSocketDetailsToResponses(t *testing.T) {
	details := []socket.SocketDetails{
		NewRemoteSocket(&SocketResponse{Id: "socket1", Rooms: []socket.Room{"room1"}}),
		NewRemoteSocket(&SocketResponse{Id: "socket2", Rooms: []socket.Room{"room2"}}),
	}

	responses := socketDetailsToResponses(details)
	if len(responses) != len(details) {
		t.Fatalf("responses length = %d, want %d", len(responses), len(details))
	}
	if responses[0].Id != "socket1" || responses[1].Id != "socket2" {
		t.Fatal("responses do not preserve socket IDs")
	}
}

func TestFetchSocketsResponseKeepsEmptySockets(t *testing.T) {
	response := FetchSocketsResponse{
		RequestId: "request",
		Sockets:   socketDetailsToResponses(nil),
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(jsonData), `{"requestId":"request","sockets":[]}`; got != want {
		t.Fatalf("json response = %s, want %s", got, want)
	}

	msgpackData, err := msgpack.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := msgpack.Unmarshal(msgpackData, &decoded); err != nil {
		t.Fatal(err)
	}
	if sockets, ok := decoded["sockets"].([]any); !ok || len(sockets) != 0 {
		t.Fatalf("msgpack sockets = %#v, want an empty array", decoded["sockets"])
	}
}
