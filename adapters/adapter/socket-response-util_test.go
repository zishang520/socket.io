package adapter

import (
	"encoding/json"
	"testing"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

type socketDetailsWithoutRooms struct{}

func (socketDetailsWithoutRooms) Id() socket.SocketId          { return "socket" }
func (socketDetailsWithoutRooms) Handshake() *socket.Handshake { return nil }
func (socketDetailsWithoutRooms) Rooms() *types.Set[socket.Room] {
	return nil
}
func (socketDetailsWithoutRooms) Data() any { return nil }

func TestSocketDetailsToResponses(t *testing.T) {
	details := []socket.SocketDetails{
		NewRemoteSocket(&SocketResponse{Id: "socket1", Rooms: []socket.Room{"room1"}}),
		NewRemoteSocket(&SocketResponse{Id: "socket2", Rooms: []socket.Room{"room2"}}),
		NewRemoteSocket(&SocketResponse{Id: "socket3"}),
	}

	responses := SocketDetailsToResponses(details)
	if len(responses) != len(details) {
		t.Fatalf("responses length = %d, want %d", len(responses), len(details))
	}
	if responses[0].Id != "socket1" || responses[1].Id != "socket2" {
		t.Fatal("responses do not preserve socket IDs")
	}
	if responses[2].Rooms == nil || len(responses[2].Rooms) != 0 {
		t.Fatalf("empty rooms = %#v, want non-nil empty slice", responses[2].Rooms)
	}
	data, err := json.Marshal(responses[2])
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"id":"socket3","handshake":null,"rooms":[],"data":null}`; got != want {
		t.Fatalf("JSON socket details = %s, want %s", got, want)
	}
}

func TestSocketDetailsToResponsesAcceptsNilRooms(t *testing.T) {
	responses := SocketDetailsToResponses([]socket.SocketDetails{socketDetailsWithoutRooms{}})
	if len(responses) != 1 || responses[0].Rooms == nil || len(responses[0].Rooms) != 0 {
		t.Fatalf("rooms = %#v, want non-nil empty slice", responses[0].Rooms)
	}
}

func TestSocketDetailConversions(t *testing.T) {
	local := NewRemoteSocket(&SocketResponse{Id: "local"})
	details := SocketDetailsToAny([]socket.SocketDetails{local})
	details = append(details, SocketResponsesToDetailsAny([]SocketResponse{{Id: "remote"}})...)

	sockets := AnySliceToSocketDetails(details)
	if len(sockets) != 2 || sockets[0] != local || sockets[1].Id() != "remote" {
		t.Fatalf("socket details = %#v", sockets)
	}
}

func TestFetchSocketsResponseKeepsEmptySockets(t *testing.T) {
	response := FetchSocketsResponse{
		RequestId: "request",
		Sockets:   SocketDetailsToResponses(nil),
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
