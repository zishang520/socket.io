package mongo

import (
	"bytes"
	"reflect"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/slices"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var decoderRegistry = func() *bson.Registry {
	registry := bson.NewRegistry()
	registry.RegisterTypeMapEntry(bson.TypeArray, reflect.TypeFor[[]any]())
	registry.RegisterTypeMapEntry(bson.TypeEmbeddedDocument, reflect.TypeFor[map[string]any]())
	return registry
}()

// MarshalAdapterData encodes message data with the exact field names and value
// shapes used by the Node.js MongoDB adapter.
func MarshalAdapterData(data any) (bson.RawValue, error) {
	switch value := data.(type) {
	case *adapter.BroadcastMessage:
		data = &PacketData[*SocketPacket]{
			Packet:    (*SocketPacket)(value.Packet),
			Opts:      serializeOptions(value.Opts),
			RequestId: value.RequestId,
		}
	case *adapter.SocketsJoinLeaveMessage:
		data = &EventData{
			Opts:  serializeOptions(value.Opts),
			Rooms: new(nonNil(value.Rooms)),
		}
	case *adapter.DisconnectSocketsMessage:
		data = &EventData{
			Opts:  serializeOptions(value.Opts),
			Close: new(value.Close),
		}
	case *adapter.FetchSocketsMessage:
		data = &EventData{
			Opts:      serializeOptions(value.Opts),
			RequestId: value.RequestId,
		}
	case *adapter.FetchSocketsResponse:
		data = &EventData{
			RequestId: value.RequestId,
			Sockets:   new(encodeSocketResponses(value.Sockets)),
		}
	case *adapter.ServerSideEmitMessage:
		data = &PacketData[[]any]{
			RequestId: value.RequestId,
			Packet:    nonNil(value.Packet),
		}
	case *adapter.ServerSideEmitResponse:
		data = &PacketData[any]{
			RequestId: new(value.RequestId),
			Packet:    slices.TryGet(value.Packet, 0),
		}
	case *adapter.BroadcastClientCount:
		data = &EventData{
			RequestId:   value.RequestId,
			ClientCount: new(value.ClientCount),
		}
	case *adapter.BroadcastAck:
		data = &PacketData[any]{
			RequestId: new(value.RequestId),
			Packet:    slices.TryGet(value.Packet, 0),
		}
	case *SessionDocument:
		session := *value
		session.Rooms = nonNil(session.Rooms)
		data = &session
	}

	var buffer bytes.Buffer
	encoder := bson.NewEncoder(bson.NewDocumentWriter(&buffer))
	encoder.UseJSONStructTags()
	if err := encoder.Encode(data); err != nil {
		return bson.RawValue{}, err
	}

	return bson.RawValue{Type: bson.TypeEmbeddedDocument, Value: buffer.Bytes()}, nil
}

// UnmarshalAdapterData decodes a MongoDB message into the internal cluster
// types while preserving the Node.js wire semantics.
func UnmarshalAdapterData(messageType adapter.MessageType, raw bson.RawValue) (any, error) {
	var target any
	switch messageType {
	case INITIAL_HEARTBEAT, HEARTBEAT:
		return nil, nil
	case BROADCAST:
		target = &PacketData[*SocketPacket]{}
	case SOCKETS_JOIN, SOCKETS_LEAVE, DISCONNECT_SOCKETS,
		FETCH_SOCKETS, FETCH_SOCKETS_RESPONSE, BROADCAST_CLIENT_COUNT:
		target = &EventData{}
	case SERVER_SIDE_EMIT:
		target = &PacketData[[]any]{}
	case SERVER_SIDE_EMIT_RESPONSE, BROADCAST_ACK:
		target = &PacketData[any]{}
	case SESSION:
		target = &SessionDocument{}
	default:
		return nil, nil
	}

	if err := raw.UnmarshalWithRegistry(decoderRegistry, target); err != nil {
		return nil, err
	}

	switch value := target.(type) {
	case *PacketData[*SocketPacket]:
		return &adapter.BroadcastMessage{
			Packet:    deserializePacket(value.Packet),
			Opts:      deserializeOptions(value.Opts),
			RequestId: value.RequestId,
		}, nil
	case *PacketData[[]any]:
		return &adapter.ServerSideEmitMessage{
			RequestId: value.RequestId,
			Packet:    nonNil(value.Packet),
		}, nil
	case *PacketData[any]:
		packet := value.Packet
		if messageType == BROADCAST_ACK {
			packet = replaceBinaryObjectsByBuffers(packet)
		}
		if _, undefined := packet.(bson.Undefined); undefined {
			packet = nil
		}
		requestId := ""
		if value.RequestId != nil {
			requestId = *value.RequestId
		}
		if messageType == BROADCAST_ACK {
			return &adapter.BroadcastAck{
				RequestId: requestId,
				Packet:    []any{packet},
			}, nil
		}
		return &adapter.ServerSideEmitResponse{
			RequestId: requestId,
			Packet:    []any{packet},
		}, nil
	case *EventData:
		return decodeEventData(messageType, value), nil
	case *SessionDocument:
		value.Rooms = nonNil(value.Rooms)
	}

	return target, nil
}

// UnmarshalDocument decodes a raw BSON document using the same dynamic value
// rules as adapter messages.
func UnmarshalDocument(raw bson.Raw, target any) error {
	decoder := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(raw)))
	decoder.SetRegistry(decoderRegistry)
	decoder.UseJSONStructTags()
	decoder.DefaultDocumentMap()
	return decoder.Decode(target)
}

func decodeEventData(messageType adapter.MessageType, data *EventData) any {
	switch messageType {
	case SOCKETS_JOIN, SOCKETS_LEAVE:
		rooms := []socket.Room{}
		if data.Rooms != nil {
			rooms = nonNil(*data.Rooms)
		}
		return &adapter.SocketsJoinLeaveMessage{
			Opts:  deserializeOptions(data.Opts),
			Rooms: rooms,
		}
	case DISCONNECT_SOCKETS:
		return &adapter.DisconnectSocketsMessage{
			Opts:  deserializeOptions(data.Opts),
			Close: data.Close != nil && *data.Close,
		}
	case FETCH_SOCKETS:
		return &adapter.FetchSocketsMessage{
			Opts:      deserializeOptions(data.Opts),
			RequestId: data.RequestId,
		}
	case FETCH_SOCKETS_RESPONSE:
		sockets := []adapter.SocketResponse{}
		if data.Sockets != nil {
			sockets = decodeSocketResponses(*data.Sockets)
		}
		return &adapter.FetchSocketsResponse{
			RequestId: data.RequestId,
			Sockets:   sockets,
		}
	case BROADCAST_CLIENT_COUNT:
		var clientCount uint64
		if data.ClientCount != nil {
			clientCount = *data.ClientCount
		}
		return &adapter.BroadcastClientCount{
			RequestId:   data.RequestId,
			ClientCount: clientCount,
		}
	default:
		return nil
	}
}

func encodeSocketResponses(sockets []adapter.SocketResponse) []SocketResponse {
	responses := make([]SocketResponse, len(sockets))
	for i, details := range sockets {
		responses[i] = SocketResponse(details)
		responses[i].Rooms = nonNil(responses[i].Rooms)
	}
	return responses
}

func decodeSocketResponses(sockets []SocketResponse) []adapter.SocketResponse {
	responses := make([]adapter.SocketResponse, len(sockets))
	for i, details := range sockets {
		responses[i] = adapter.SocketResponse(details)
		responses[i].Rooms = nonNil(responses[i].Rooms)
	}
	return responses
}

func serializeOptions(opts *adapter.PacketOptions) *PacketOptions {
	if opts == nil {
		return &PacketOptions{
			Rooms:  []socket.Room{},
			Except: []socket.Room{},
		}
	}

	result := &PacketOptions{
		Rooms:  nonNil(opts.Rooms),
		Except: nonNil(opts.Except),
	}
	if flags := opts.Flags; flags != nil {
		result.Flags = &BroadcastFlags{
			Compress:             flags.Compress,
			Volatile:             flags.Volatile,
			Local:                flags.Local,
			Broadcast:            flags.Broadcast,
			Binary:               flags.Binary,
			ExpectSingleResponse: flags.ExpectSingleResponse,
		}
		if flags.Timeout != nil {
			result.Flags.Timeout = new(flags.Timeout.Milliseconds())
		}
	}
	return result
}

func deserializeOptions(opts *PacketOptions) *adapter.PacketOptions {
	if opts == nil {
		return &adapter.PacketOptions{
			Rooms:  []socket.Room{},
			Except: []socket.Room{},
		}
	}

	result := &adapter.PacketOptions{
		Rooms:  nonNil(opts.Rooms),
		Except: nonNil(opts.Except),
	}
	if flags := opts.Flags; flags != nil {
		result.Flags = &socket.BroadcastFlags{
			Local:                flags.Local,
			Broadcast:            flags.Broadcast,
			Binary:               flags.Binary,
			ExpectSingleResponse: flags.ExpectSingleResponse,
		}
		result.Flags.Compress = flags.Compress
		result.Flags.Volatile = flags.Volatile
		if flags.Timeout != nil {
			result.Flags.Timeout = new(time.Duration(*flags.Timeout) * time.Millisecond)
		}
	}
	return result
}

func deserializePacket(packet *SocketPacket) *parser.Packet {
	if packet == nil {
		return nil
	}
	packet.Data = replaceBinaryObjectsByBuffers(packet.Data)
	return (*parser.Packet)(packet)
}

func replaceBinaryObjectsByBuffers(value any) any {
	switch value := value.(type) {
	case bson.Binary:
		return value.Data
	case []any:
		for i := range value {
			value[i] = replaceBinaryObjectsByBuffers(value[i])
		}
	case map[string]any:
		for key, item := range value {
			value[key] = replaceBinaryObjectsByBuffers(item)
		}
	}
	return value
}

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
