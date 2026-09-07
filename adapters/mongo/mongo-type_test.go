package mongo

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
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

func TestMarshalAdapterDataPreservesBroadcastBuffersForLocalEncoding(t *testing.T) {
	tests := []struct {
		name   string
		value  any
		binary bool
	}{
		{"bytes", []byte("payload"), true},
		{"bytes buffer", types.NewBytesBuffer([]byte("payload")), true},
		{"binary reader", bytes.NewReader([]byte("payload")), true},
		{"string buffer", types.NewStringBufferString("payload"), false},
		{"string reader", strings.NewReader("payload"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			packet := &parser.Packet{Type: parser.EVENT, Nsp: "/", Data: []any{"event", tt.value}}
			raw := mustMarshalAdapterData(t, &adapter.BroadcastMessage{Packet: packet})
			value := raw.Lookup("packet", "data", "1")
			if tt.binary {
				subtype, payload, ok := value.BinaryOK()
				if !ok || subtype != 0 || !bytes.Equal(payload, []byte("payload")) {
					t.Fatalf("wire payload = %v, want BSON binary payload", value)
				}
			} else if payload, ok := value.StringValueOK(); !ok || payload != "payload" {
				t.Fatalf("wire payload = %v, want string payload", value)
			}

			// Mongo publication precedes local delivery, which must still see the
			// reader's contents after BSON serialization has consumed the reader.
			buffers := parser.NewEncoder().Encode(packet)
			if tt.binary {
				if len(buffers) != 2 || !bytes.Equal(buffers[1].Bytes(), []byte("payload")) {
					t.Fatalf("local binary buffers = %v, want payload attachment", buffers)
				}
			} else if len(buffers) != 1 || buffers[0].String() != `2["event","payload"]` {
				t.Fatalf("local text buffers = %v, want event with payload", buffers)
			}
		})
	}
}

func TestMarshalAdapterDataMaterializesNestedMessagePayloads(t *testing.T) {
	payload := []byte{0x00, 0x7f, 0x80, 0xff}
	newData := func() map[string]any {
		return map[string]any{"nested": []any{types.NewBytesBuffer(payload), bytes.NewReader(payload), strings.NewReader("text")}}
	}
	tests := []struct {
		name string
		data any
		path []string
	}{
		{"broadcast with ack", &adapter.BroadcastMessage{
			Packet: &parser.Packet{Type: parser.EVENT, Data: []any{"event", newData()}}, RequestId: new("request"),
		}, []string{"packet", "data", "1"}},
		{"broadcast ack", &adapter.BroadcastAck{RequestId: "request", Packet: newData()}, []string{"packet"}},
		{"server-side emit", &adapter.ServerSideEmitMessage{Packet: []any{"event", newData()}}, []string{"packet", "1"}},
		{"server-side response", &adapter.ServerSideEmitResponse{RequestId: "request", Packet: newData()}, []string{"packet"}},
		{"fetch socket data", &adapter.FetchSocketsResponse{
			Sockets: []adapter.SocketResponse{{Id: "socket", Data: newData()}},
		}, []string{"sockets", "0", "data"}},
		{"fetch socket auth", &adapter.FetchSocketsResponse{
			Sockets: []adapter.SocketResponse{{Id: "socket", Handshake: &socket.Handshake{Auth: newData()}}},
		}, []string{"sockets", "0", "handshake", "auth"}},
		{"session", &SessionDocument{Sid: "socket", Pid: "private", Data: newData()}, []string{"data"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := mustMarshalAdapterData(t, tt.data).Lookup(tt.path...).Document().Lookup("nested").Array()
			for index := range uint(2) {
				value := raw.Index(index)
				subtype, got, ok := value.BinaryOK()
				if !ok || subtype != 0 || !bytes.Equal(got, payload) {
					t.Fatalf("nested payload %d = %v, want BSON binary %x", index, value, payload)
				}
			}
			if got := raw.Index(2).StringValue(); got != "text" {
				t.Fatalf("nested text = %q, want text", got)
			}
		})
	}
}

func TestMarshalAdapterDataPreservesBSONValues(t *testing.T) {
	document := bson.D{{Key: "value", Value: "document"}}
	rawDocument := mustMarshalNodeDocument(t, document)
	for _, value := range []any{
		bson.Binary{Subtype: 0x80, Data: []byte("binary")},
		document,
		rawDocument,
		mongoValueMarshaler{},
	} {
		wantType, wantValue, err := bson.MarshalValue(value)
		if err != nil {
			t.Fatal(err)
		}
		raw := mustMarshalAdapterData(t, &SessionDocument{Data: map[string]any{"value": value}})
		got := raw.Lookup("data", "value")
		if got.Type != wantType || !bytes.Equal(got.Value, wantValue) {
			t.Fatalf("%T encoded as %v, want original BSON representation", value, got)
		}
	}
}

type mongoValueMarshaler struct{}

func (mongoValueMarshaler) MarshalBSONValue() (byte, []byte, error) {
	valueType, value, err := bson.MarshalValue("custom BSON value")
	return byte(valueType), value, err
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
	timeout := 1_750.0
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
	if got := wireFlags.Lookup("timeout").Double(); got != timeout {
		t.Fatalf("timeout = %g milliseconds, want %g", got, timeout)
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

func TestUnmarshalAdapterDataPreservesNodeTimeout(t *testing.T) {
	for _, tt := range []struct {
		name    string
		timeout any
		want    float64
	}{
		{"fractional", 16.5, 16.5},
		{"int32", int32(16), 16},
		{"int64", int64(16), 16},
	} {
		t.Run(tt.name, func(t *testing.T) {
			raw := mustMarshalNodeDocument(t, bson.D{
				{Key: "packet", Value: bson.D{{Key: "type", Value: int32(parser.EVENT)}, {Key: "data", Value: bson.A{"event"}}}},
				{Key: "opts", Value: bson.D{
					{Key: "rooms", Value: bson.A{}},
					{Key: "except", Value: bson.A{}},
					{Key: "flags", Value: bson.D{{Key: "timeout", Value: tt.timeout}}},
				}},
			})
			decoded, err := UnmarshalAdapterData(BROADCAST, bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: raw})
			if err != nil {
				t.Fatal(err)
			}
			timeout := decoded.(*adapter.BroadcastMessage).Opts.Flags.Timeout
			if timeout == nil || *timeout != tt.want {
				t.Fatalf("timeout = %v, want %g milliseconds", timeout, tt.want)
			}
			encoded := mustMarshalAdapterData(t, decoded)
			if got := encoded.Lookup("opts", "flags", "timeout").Double(); got != tt.want {
				t.Fatalf("round-trip timeout = %g milliseconds, want %g", got, tt.want)
			}
		})
	}
}

func TestUnmarshalAdapterDataRejectsFractionalClientCount(t *testing.T) {
	raw := mustMarshalNodeDocument(t, bson.D{
		{Key: "requestId", Value: "request"},
		{Key: "clientCount", Value: 2.5},
	})
	if _, err := UnmarshalAdapterData(BROADCAST_CLIENT_COUNT, bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: raw}); err == nil {
		t.Fatal("fractional clientCount was accepted")
	}
}

func TestUnmarshalAdapterDataPreservesInvalidOptions(t *testing.T) {
	tests := []struct {
		name        string
		messageType adapter.MessageType
		data        bson.D
		wantOpts    bool
	}{
		{
			name:        "broadcast without opts",
			messageType: BROADCAST,
			data:        bson.D{{Key: "packet", Value: bson.D{}}},
		},
		{
			name:        "leave with missing option fields",
			messageType: SOCKETS_LEAVE,
			data: bson.D{
				{Key: "opts", Value: bson.D{}},
				{Key: "rooms", Value: bson.A{}},
			},
			wantOpts: true,
		},
		{
			name:        "disconnect with null rooms",
			messageType: DISCONNECT_SOCKETS,
			data: bson.D{
				{Key: "opts", Value: bson.D{
					{Key: "rooms", Value: nil},
					{Key: "except", Value: bson.A{}},
				}},
				{Key: "close", Value: false},
			},
			wantOpts: true,
		},
		{
			name:        "fetch with null except",
			messageType: FETCH_SOCKETS,
			data: bson.D{
				{Key: "opts", Value: bson.D{
					{Key: "rooms", Value: bson.A{}},
					{Key: "except", Value: nil},
				}},
				{Key: "requestId", Value: "request-1"},
			},
			wantOpts: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := mustMarshalNodeDocument(t, tt.data)
			decoded, err := UnmarshalAdapterData(tt.messageType, bson.RawValue{
				Type:  bson.TypeEmbeddedDocument,
				Value: raw,
			})
			if err != nil {
				t.Fatal(err)
			}

			opts := decodedPacketOptions(t, decoded)
			if (opts != nil) != tt.wantOpts {
				t.Fatalf("options = %#v, want present %t", opts, tt.wantOpts)
			}
			if opts.IsValid() {
				t.Fatalf("invalid wire options were normalized: %#v", opts)
			}
		})
	}
}

func TestUnmarshalAdapterDataRequiresFetchSockets(t *testing.T) {
	for _, tt := range []struct {
		name         string
		data         bson.D
		wantResponse bool
	}{
		{name: "missing", data: bson.D{{Key: "requestId", Value: "request"}}},
		{name: "null", data: bson.D{{Key: "requestId", Value: "request"}, {Key: "sockets", Value: nil}}},
		{name: "empty", data: bson.D{{Key: "requestId", Value: "request"}, {Key: "sockets", Value: bson.A{}}}, wantResponse: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			decoded, err := UnmarshalAdapterData(FETCH_SOCKETS_RESPONSE, bson.RawValue{
				Type:  bson.TypeEmbeddedDocument,
				Value: mustMarshalNodeDocument(t, tt.data),
			})
			if err != nil {
				t.Fatal(err)
			}
			if !tt.wantResponse {
				if decoded != nil {
					t.Fatalf("decoded response = %T, want nil", decoded)
				}
				return
			}
			response, ok := decoded.(*adapter.FetchSocketsResponse)
			if !ok {
				t.Fatalf("decoded response type = %T", decoded)
			}
			if response.Sockets == nil || len(response.Sockets) != 0 {
				t.Fatalf("sockets = %#v, want non-nil empty slice", response.Sockets)
			}
		})
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

func decodedPacketOptions(t *testing.T, data any) *adapter.PacketOptions {
	t.Helper()
	switch value := data.(type) {
	case *adapter.BroadcastMessage:
		return value.Opts
	case *adapter.SocketsJoinLeaveMessage:
		return value.Opts
	case *adapter.DisconnectSocketsMessage:
		return value.Opts
	case *adapter.FetchSocketsMessage:
		return value.Opts
	default:
		t.Fatalf("unexpected adapter data type: %T", data)
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
