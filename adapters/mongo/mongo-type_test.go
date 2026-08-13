package mongo

import (
	"bytes"
	"slices"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMongoEventTypesMatchNodeProtocol(t *testing.T) {
	tests := []struct {
		name string
		got  adapter.MessageType
	}{
		{"INITIAL_HEARTBEAT", INITIAL_HEARTBEAT},
		{"HEARTBEAT", HEARTBEAT},
		{"BROADCAST", BROADCAST},
		{"SOCKETS_JOIN", SOCKETS_JOIN},
		{"SOCKETS_LEAVE", SOCKETS_LEAVE},
		{"DISCONNECT_SOCKETS", DISCONNECT_SOCKETS},
		{"FETCH_SOCKETS", FETCH_SOCKETS},
		{"FETCH_SOCKETS_RESPONSE", FETCH_SOCKETS_RESPONSE},
		{"SERVER_SIDE_EMIT", SERVER_SIDE_EMIT},
		{"SERVER_SIDE_EMIT_RESPONSE", SERVER_SIDE_EMIT_RESPONSE},
		{"BROADCAST_CLIENT_COUNT", BROADCAST_CLIENT_COUNT},
		{"BROADCAST_ACK", BROADCAST_ACK},
		{"SESSION", SESSION},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if want := adapter.MessageType(i + 1); tt.got != want {
				t.Fatalf("unexpected event type: got %d, want %d", tt.got, want)
			}

			document, err := bson.Marshal(&AdapterEvent{Type: tt.got})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := bson.Raw(document).Lookup("type").AsInt64(), int64(i+1); got != want {
				t.Fatalf("unexpected BSON event type: got %d, want %d", got, want)
			}
		})
	}
}

func TestMarshalAdapterDataUsesNodeFieldNames(t *testing.T) {
	t.Run("requestId", func(t *testing.T) {
		raw := mustMarshalAdapterData(t, &adapter.FetchSocketsMessage{
			RequestId: "request-1",
		})

		assertMongoKeys(t, raw, "opts", "requestId")
		if got := raw.Lookup("requestId").StringValue(); got != "request-1" {
			t.Fatalf("unexpected requestId: got %q", got)
		}
		assertMongoMissing(t, raw, "requestid")
	})

	t.Run("clientCount", func(t *testing.T) {
		raw := mustMarshalAdapterData(t, &adapter.BroadcastClientCount{
			RequestId:   "request-2",
			ClientCount: 42,
		})

		assertMongoKeys(t, raw, "requestId", "clientCount")
		if got := raw.Lookup("clientCount").AsInt64(); got != 42 {
			t.Fatalf("unexpected clientCount: got %d", got)
		}
		assertMongoMissing(t, raw, "requestid")
		assertMongoMissing(t, raw, "clientcount")
	})
}

func TestMarshalAdapterDataNormalizesSocketRoomsWithoutMutation(t *testing.T) {
	response := &adapter.FetchSocketsResponse{
		RequestId: "request-1",
		Sockets:   []adapter.SocketResponse{{Id: "socket-1"}},
	}
	raw := mustMarshalAdapterData(t, response)

	if response.Sockets[0].Rooms != nil {
		t.Fatal("input socket rooms were modified")
	}
	sockets, err := raw.Lookup("sockets").Array().Values()
	if err != nil {
		t.Fatal(err)
	}
	if len(sockets) != 1 {
		t.Fatalf("unexpected sockets: %v", sockets)
	}
	assertMongoEmptyArray(t, sockets[0].Document().Lookup("rooms"))
}

func TestMarshalAdapterDataOmitsOptionalRequestAndPacketID(t *testing.T) {
	raw := mustMarshalAdapterData(t, &adapter.BroadcastMessage{
		Packet: &parser.Packet{
			Type: parser.EVENT,
			Nsp:  "/",
			Data: []any{"event", "value"},
		},
	})

	assertMongoKeys(t, raw, "packet", "opts")
	assertMongoMissing(t, raw, "requestId")

	packet := raw.Lookup("packet").Document()
	assertMongoKeys(t, packet, "type", "nsp", "data")
	assertMongoMissing(t, packet, "id")
	assertMongoMissing(t, packet, "attachments")

	serverSideEmit := mustMarshalAdapterData(t, &adapter.ServerSideEmitMessage{
		Packet: []any{"event", "value"},
	})
	assertMongoKeys(t, serverSideEmit, "packet")
	assertMongoMissing(t, serverSideEmit, "requestId")

	withoutNamespace := mustMarshalAdapterData(t, &adapter.BroadcastMessage{
		Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event"}},
	})
	assertMongoMissing(t, withoutNamespace.Lookup("packet").Document(), "nsp")
}

func TestMarshalAdapterDataUsesNodeOptionsShape(t *testing.T) {
	compress := false
	timeout := int64(1_750)
	flags := &socket.BroadcastFlags{
		Local:                true,
		Broadcast:            true,
		Binary:               true,
		Timeout:              &timeout,
		ExpectSingleResponse: true,
	}
	flags.Compress = &compress
	flags.Volatile = true

	raw := mustMarshalAdapterData(t, &adapter.BroadcastMessage{
		Packet: &parser.Packet{Type: parser.EVENT, Nsp: "/", Data: []any{"event"}},
		Opts:   &adapter.PacketOptions{Flags: flags},
	})
	opts := raw.Lookup("opts").Document()
	assertMongoKeys(t, opts, "rooms", "except", "flags")
	assertMongoEmptyArray(t, opts.Lookup("rooms"))
	assertMongoEmptyArray(t, opts.Lookup("except"))

	wireFlags := opts.Lookup("flags").Document()
	assertMongoKeys(t, wireFlags,
		"compress", "volatile", "local", "broadcast", "binary", "timeout", "expectSingleResponse",
	)
	if wireFlags.Lookup("compress").Boolean() {
		t.Fatal("compress=false was not preserved")
	}
	for _, key := range []string{"volatile", "local", "broadcast", "binary", "expectSingleResponse"} {
		if !wireFlags.Lookup(key).Boolean() {
			t.Fatalf("%s=true was not preserved", key)
		}
	}
	if got := wireFlags.Lookup("timeout").AsInt64(); got != 1_750 {
		t.Fatalf("timeout was not converted to milliseconds: got %d", got)
	}
	assertMongoMissing(t, wireFlags, "writeOptions")
	assertMongoMissing(t, wireFlags, "preEncoded")
	assertMongoMissing(t, wireFlags, "wsPreEncodedFrame")
}

func TestUnmarshalAdapterDataAcceptsNodeOptions(t *testing.T) {
	raw := mustMarshalNodeDocument(t, bson.D{
		{Key: "packet", Value: bson.D{
			{Key: "type", Value: int32(parser.EVENT)},
			{Key: "data", Value: bson.A{"event"}},
			{Key: "nsp", Value: "/"},
		}},
		{Key: "opts", Value: bson.D{
			{Key: "rooms", Value: bson.A{}},
			{Key: "except", Value: bson.A{}},
			{Key: "flags", Value: bson.D{
				{Key: "volatile", Value: true},
				{Key: "compress", Value: false},
				{Key: "timeout", Value: int32(2_750)},
				{Key: "expectSingleResponse", Value: true},
			}},
		}},
	})

	decoded, err := UnmarshalAdapterData(BROADCAST, bson.RawValue{
		Type:  bson.TypeEmbeddedDocument,
		Value: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	message, ok := decoded.(*adapter.BroadcastMessage)
	if !ok {
		t.Fatalf("unexpected decoded type: %T", decoded)
	}
	if message.Opts == nil || message.Opts.Flags == nil {
		t.Fatal("missing decoded options or flags")
	}
	if message.Opts.Rooms == nil || len(message.Opts.Rooms) != 0 {
		t.Fatalf("unexpected rooms: %#v", message.Opts.Rooms)
	}
	if message.Opts.Except == nil || len(message.Opts.Except) != 0 {
		t.Fatalf("unexpected except rooms: %#v", message.Opts.Except)
	}
	if !message.Opts.Flags.Volatile {
		t.Fatal("volatile=true was not decoded")
	}
	if message.Opts.Flags.Compress == nil || *message.Opts.Flags.Compress {
		t.Fatal("compress=false was not decoded")
	}
	if message.Opts.Flags.Timeout == nil || *message.Opts.Flags.Timeout != 2_750 {
		t.Fatalf("unexpected timeout: %v", message.Opts.Flags.Timeout)
	}
	if !message.Opts.Flags.ExpectSingleResponse {
		t.Fatal("expectSingleResponse=true was not decoded")
	}
}

func TestUnmarshalAdapterDataAcceptsNodeSocketDetails(t *testing.T) {
	raw := mustMarshalNodeDocument(t, bson.D{
		{Key: "requestId", Value: "request-1"},
		{Key: "sockets", Value: bson.A{bson.D{
			{Key: "id", Value: "socket-1"},
			{Key: "handshake", Value: bson.D{
				{Key: "headers", Value: bson.D{{Key: "origin", Value: "https://example.com"}}},
				{Key: "time", Value: "now"},
				{Key: "address", Value: "127.0.0.1"},
				{Key: "xdomain", Value: true},
				{Key: "secure", Value: true},
				{Key: "issued", Value: int64(42)},
				{Key: "url", Value: "/socket.io"},
				{Key: "query", Value: bson.D{{Key: "transport", Value: "websocket"}}},
				{Key: "auth", Value: bson.D{{Key: "token", Value: "secret"}}},
			}},
			{Key: "rooms", Value: bson.A{"socket-1", "room-1"}},
			{Key: "data", Value: bson.D{{Key: "name", Value: "alice"}}},
		}}},
	})

	decoded, err := UnmarshalAdapterData(FETCH_SOCKETS_RESPONSE, bson.RawValue{
		Type:  bson.TypeEmbeddedDocument,
		Value: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, ok := decoded.(*adapter.FetchSocketsResponse)
	if !ok || len(response.Sockets) != 1 {
		t.Fatalf("unexpected response: %#v", decoded)
	}
	details := response.Sockets[0]
	if details.Id != "socket-1" || details.Handshake == nil {
		t.Fatalf("unexpected socket details: %#v", details)
	}
	if details.Handshake.Url != "/socket.io" || details.Handshake.Headers["origin"] != "https://example.com" ||
		details.Handshake.Query["transport"] != "websocket" || details.Handshake.Auth["token"] != "secret" {
		t.Fatalf("unexpected handshake: %#v", details.Handshake)
	}
	data, ok := details.Data.(map[string]any)
	if !ok || data["name"] != "alice" {
		t.Fatalf("unexpected socket data: %#v", details.Data)
	}
}

func TestMarshalAdapterDataUsesScalarResponses(t *testing.T) {
	tests := []struct {
		name       string
		data       any
		packetType bson.Type
	}{
		{
			name:       "server-side emit scalar",
			data:       &adapter.ServerSideEmitResponse{RequestId: "request-1", Packet: "answer"},
			packetType: bson.TypeString,
		},
		{
			name:       "server-side emit array",
			data:       &adapter.ServerSideEmitResponse{RequestId: "request-1", Packet: []any{"answer"}},
			packetType: bson.TypeArray,
		},
		{
			name:       "broadcast acknowledgement without argument",
			data:       &adapter.BroadcastAck{RequestId: "request-2"},
			packetType: bson.TypeNull,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := mustMarshalAdapterData(t, tt.data)
			assertMongoKeys(t, raw, "requestId", "packet")
			assertMongoMissing(t, raw, "requestid")
			if got := raw.Lookup("packet").Type; got != tt.packetType {
				t.Fatalf("unexpected packet type: got %s, want %s", got, tt.packetType)
			}
		})
	}
}

func TestUnmarshalAdapterDataAcceptsNodeScalarResponses(t *testing.T) {
	binary := []byte{0x00, 0x7f, 0x80, 0xff}

	t.Run("broadcast acknowledgement converts Binary", func(t *testing.T) {
		decoded := mustUnmarshalNodeResponse(t, BROADCAST_ACK, bson.Binary{Subtype: 0x80, Data: binary})
		packet := responsePacket(t, decoded)
		got, ok := packet.([]byte)
		if !ok || !bytes.Equal(got, binary) {
			t.Fatalf("unexpected broadcast acknowledgement: %#v", packet)
		}
	})

	t.Run("server-side emit response preserves Binary", func(t *testing.T) {
		decoded := mustUnmarshalNodeResponse(t, SERVER_SIDE_EMIT_RESPONSE, bson.Binary{Subtype: 0, Data: binary})
		packet := responsePacket(t, decoded)
		got, ok := packet.(bson.Binary)
		if !ok || got.Subtype != 0 || !bytes.Equal(got.Data, binary) {
			t.Fatalf("unexpected server-side emit response: %#v", packet)
		}
	})

	for _, tt := range []struct {
		name        string
		messageType adapter.MessageType
	}{
		{"server-side emit response", SERVER_SIDE_EMIT_RESPONSE},
		{"broadcast acknowledgement", BROADCAST_ACK},
	} {
		t.Run(tt.name+"/null", func(t *testing.T) {
			decoded := mustUnmarshalNodeResponse(t, tt.messageType, nil)
			packet := responsePacket(t, decoded)
			if packet != nil {
				t.Fatalf("unexpected null packet: %#v", packet)
			}
		})
	}
}

func TestSessionDocumentsMatchNodeShape(t *testing.T) {
	t.Run("session", func(t *testing.T) {
		raw := mustMarshalAdapterData(t, &SessionDocument{
			Sid:   "socket-1",
			Pid:   "private-1",
			Rooms: []socket.Room{"room-1", "room-2"},
			Data: bson.D{
				{Key: "name", Value: "alice"},
				{Key: "admin", Value: true},
			},
		})

		assertMongoKeys(t, raw, "sid", "pid", "rooms", "data")
		rooms, err := raw.Lookup("rooms").Array().Values()
		if err != nil {
			t.Fatal(err)
		}
		if len(rooms) != 2 || rooms[0].StringValue() != "room-1" || rooms[1].StringValue() != "room-2" {
			t.Fatalf("unexpected session rooms: %v", rooms)
		}
		data := raw.Lookup("data").Document()
		assertMongoKeys(t, data, "name", "admin")
		if data.Lookup("name").StringValue() != "alice" || !data.Lookup("admin").Boolean() {
			t.Fatalf("unexpected session data: %s", data)
		}
	})

	t.Run("tombstone", func(t *testing.T) {
		raw := mustMarshalAdapterData(t, &SessionTombstone{
			Pid:       "private-1",
			Tombstone: true,
		})

		assertMongoKeys(t, raw, "pid", "tombstone")
		if raw.Lookup("pid").StringValue() != "private-1" || !raw.Lookup("tombstone").Boolean() {
			t.Fatalf("unexpected tombstone: %s", raw)
		}
		assertMongoMissing(t, raw, "sid")
		assertMongoMissing(t, raw, "rooms")
		assertMongoMissing(t, raw, "data")
	})
}

func TestUnmarshalAdapterDataAcceptsNodeSession(t *testing.T) {
	binary := []byte{0x01, 0x02, 0xff}
	raw := mustMarshalNodeDocument(t, bson.D{
		{Key: "sid", Value: "socket-1"},
		{Key: "pid", Value: "private-1"},
		{Key: "rooms", Value: bson.A{"room-1", "room-2"}},
		{Key: "data", Value: bson.D{
			{Key: "name", Value: "alice"},
			{Key: "payload", Value: bson.Binary{Subtype: 0, Data: binary}},
		}},
	})

	decoded, err := UnmarshalAdapterData(SESSION, bson.RawValue{
		Type:  bson.TypeEmbeddedDocument,
		Value: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	session, ok := decoded.(*SessionDocument)
	if !ok {
		t.Fatalf("unexpected decoded type: %T", decoded)
	}
	if session.Sid != "socket-1" || session.Pid != "private-1" {
		t.Fatalf("unexpected session IDs: sid=%q pid=%q", session.Sid, session.Pid)
	}
	if !slices.Equal(session.Rooms, []socket.Room{"room-1", "room-2"}) {
		t.Fatalf("unexpected session rooms: %v", session.Rooms)
	}
	data, ok := session.Data.(map[string]any)
	if !ok {
		t.Fatalf("session data decoded as %T, want map[string]any", session.Data)
	}
	if data["name"] != "alice" {
		t.Fatalf("unexpected session name: %#v", data["name"])
	}
	payload, ok := data["payload"].(bson.Binary)
	if !ok || payload.Subtype != 0 || !bytes.Equal(payload.Data, binary) {
		t.Fatalf("unexpected session binary data: %#v", data["payload"])
	}
}

func mustMarshalAdapterData(t *testing.T, data any) bson.Raw {
	t.Helper()
	raw, err := MarshalAdapterData(data)
	if err != nil {
		t.Fatal(err)
	}
	if raw.Type != bson.TypeEmbeddedDocument {
		t.Fatalf("unexpected BSON type: got %s, want embedded document", raw.Type)
	}
	return bson.Raw(raw.Value)
}

func mustMarshalNodeDocument(t *testing.T, document bson.D) bson.Raw {
	t.Helper()
	data, err := bson.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return bson.Raw(data)
}

func mustUnmarshalNodeResponse(t *testing.T, messageType adapter.MessageType, packet any) any {
	t.Helper()
	raw := mustMarshalNodeDocument(t, bson.D{
		{Key: "requestId", Value: "request-1"},
		{Key: "packet", Value: packet},
	})
	decoded, err := UnmarshalAdapterData(messageType, bson.RawValue{
		Type:  bson.TypeEmbeddedDocument,
		Value: raw,
	})
	if err != nil {
		t.Fatal(err)
	}
	return decoded
}

func responsePacket(t *testing.T, response any) any {
	t.Helper()
	switch value := response.(type) {
	case *adapter.ServerSideEmitResponse:
		return value.Packet
	case *adapter.BroadcastAck:
		return value.Packet
	default:
		t.Fatalf("unexpected response type: %T", response)
		return nil
	}
}

func assertMongoKeys(t *testing.T, raw bson.Raw, expected ...string) {
	t.Helper()
	elements, err := raw.Elements()
	if err != nil {
		t.Fatal(err)
	}
	keys := make([]string, len(elements))
	for i, element := range elements {
		keys[i] = element.Key()
	}
	slices.Sort(keys)
	slices.Sort(expected)
	if !slices.Equal(keys, expected) {
		t.Fatalf("unexpected BSON keys: got %v, want %v", keys, expected)
	}
}

func assertMongoMissing(t *testing.T, raw bson.Raw, key string) {
	t.Helper()
	if _, err := raw.LookupErr(key); err == nil {
		t.Fatalf("unexpected BSON key %q", key)
	}
}

func assertMongoEmptyArray(t *testing.T, value bson.RawValue) {
	t.Helper()
	if value.Type != bson.TypeArray {
		t.Fatalf("unexpected BSON type: got %s, want array", value.Type)
	}
	values, err := value.Array().Values()
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 0 {
		t.Fatalf("unexpected array values: %v", values)
	}
}
