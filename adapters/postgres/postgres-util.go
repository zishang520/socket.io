package postgres

import (
	"io"
	"maps"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	sliceUtils "github.com/zishang520/socket.io/v3/pkg/slices"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// MarshalAdapterData converts internal cluster data to the Node.js wire shape
// and reports whether the message must use the attachment table. Reader values
// are normalized so remote and local delivery observe the same data.
func MarshalAdapterData(data any) (any, bool) {
	switch value := data.(type) {
	case *adapter.BroadcastMessage:
		packet := value.Packet
		var binary bool
		if packet != nil {
			packet.Data, _, binary = marshalData(packet.Data)
		}
		return &PacketData[*parser.Packet]{
			Packet:    packet,
			Opts:      encodeOptions(value.Opts),
			RequestId: value.RequestId,
		}, binary
	case *adapter.SocketsJoinLeaveMessage:
		return &EventData{
			Opts:  encodeOptions(value.Opts),
			Rooms: new(nonNil(value.Rooms)),
		}, false
	case *adapter.DisconnectSocketsMessage:
		return &EventData{
			Opts:  encodeOptions(value.Opts),
			Close: new(value.Close),
		}, false
	case *adapter.FetchSocketsMessage:
		return &EventData{
			Opts:      encodeOptions(value.Opts),
			RequestId: value.RequestId,
		}, false
	case *adapter.FetchSocketsResponse:
		return &EventData{
			RequestId: value.RequestId,
			Sockets:   new(encodeSocketResponses(value.Sockets)),
		}, false
	case *adapter.ServerSideEmitMessage:
		packet, _, binary := marshalData(value.Packet)
		value.Packet = packet.([]any)
		return &PacketData[[]any]{
			RequestId: value.RequestId,
			Packet:    nonNil(value.Packet),
		}, binary
	case *adapter.ServerSideEmitResponse:
		packet, changed, binary := marshalData(sliceUtils.TryGet(value.Packet, 0))
		if changed && len(value.Packet) > 0 {
			value.Packet[0] = packet
		}
		return &PacketData[any]{
			RequestId: new(value.RequestId),
			Packet:    packet,
		}, binary
	case *adapter.BroadcastClientCount:
		return &EventData{
			RequestId:   value.RequestId,
			ClientCount: new(value.ClientCount),
		}, false
	case *adapter.BroadcastAck:
		packet, changed, binary := marshalData(sliceUtils.TryGet(value.Packet, 0))
		if changed && len(value.Packet) > 0 {
			value.Packet[0] = packet
		}
		return &PacketData[any]{
			RequestId: new(value.RequestId),
			Packet:    packet,
		}, binary
	default:
		return data, false
	}
}

// AdapterDataTarget returns the wire type used to decode a cluster message.
func AdapterDataTarget(messageType adapter.MessageType) any {
	switch messageType {
	case adapter.BROADCAST:
		return &PacketData[*parser.Packet]{}
	case adapter.SOCKETS_JOIN, adapter.SOCKETS_LEAVE, adapter.DISCONNECT_SOCKETS,
		adapter.FETCH_SOCKETS, adapter.FETCH_SOCKETS_RESPONSE, adapter.BROADCAST_CLIENT_COUNT:
		return &EventData{}
	case adapter.SERVER_SIDE_EMIT:
		return &PacketData[[]any]{}
	case adapter.SERVER_SIDE_EMIT_RESPONSE, adapter.BROADCAST_ACK:
		return &PacketData[any]{}
	default:
		return nil
	}
}

// UnmarshalAdapterData converts a decoded wire value to the internal cluster types.
func UnmarshalAdapterData(messageType adapter.MessageType, data any) any {
	switch value := data.(type) {
	case *PacketData[*parser.Packet]:
		return &adapter.BroadcastMessage{
			Packet:    value.Packet,
			Opts:      decodeOptions(value.Opts),
			RequestId: value.RequestId,
		}
	case *PacketData[[]any]:
		return &adapter.ServerSideEmitMessage{
			RequestId: value.RequestId,
			Packet:    nonNil(value.Packet),
		}
	case *PacketData[any]:
		packet := []any{value.Packet}
		requestId := ""
		if value.RequestId != nil {
			requestId = *value.RequestId
		}
		if messageType == adapter.BROADCAST_ACK {
			return &adapter.BroadcastAck{RequestId: requestId, Packet: packet}
		}
		return &adapter.ServerSideEmitResponse{RequestId: requestId, Packet: packet}
	case *EventData:
		return unmarshalEventData(messageType, value)
	default:
		return data
	}
}

func marshalData(data any) (any, bool, bool) {
	switch value := data.(type) {
	case nil:
		return nil, false, false
	case *strings.Reader:
		if value == nil {
			return nil, true, false
		}
		payload, _ := io.ReadAll(value)
		return string(payload), true, false
	case *types.StringBuffer:
		if value == nil || value.Buffer == nil {
			return nil, true, false
		}
		return value.String(), true, false
	case []byte:
		if value == nil {
			return []byte{}, true, true
		}
		return value, false, true
	case interface{ Bytes() []byte }:
		if !parser.IsBinary(data) || isNil(data) {
			return data, false, false
		}
		payload := value.Bytes()
		if payload == nil {
			payload = []byte{}
		}
		return payload, true, true
	case io.Reader:
		if !parser.IsBinary(data) || isNil(data) {
			return data, false, false
		}
		if closer, ok := data.(io.Closer); ok {
			defer func() { _ = closer.Close() }()
		}
		payload, _ := io.ReadAll(value)
		if payload == nil {
			payload = []byte{}
		}
		return payload, true, true
	case []any:
		var result []any
		var binary bool
		for i, item := range value {
			encoded, changed, hasBinary := marshalData(item)
			binary = binary || hasBinary
			if !changed {
				continue
			}
			if result == nil {
				result = slices.Clone(value)
			}
			result[i] = encoded
		}
		if result != nil {
			return result, true, binary
		}
		return data, false, binary
	case map[string]any:
		var result map[string]any
		var binary bool
		for key, item := range value {
			encoded, changed, hasBinary := marshalData(item)
			binary = binary || hasBinary
			if !changed {
				continue
			}
			if result == nil {
				result = maps.Clone(value)
			}
			result[key] = encoded
		}
		if result != nil {
			return result, true, binary
		}
		return data, false, binary
	}
	// Keep traversal aligned with parser.HasBinary so local and remote
	// Socket.IO delivery select the same binary encoding path.
	return data, false, false
}

func isNil(value any) bool {
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}

func unmarshalEventData(messageType adapter.MessageType, data *EventData) any {
	switch messageType {
	case adapter.SOCKETS_JOIN, adapter.SOCKETS_LEAVE:
		rooms := []socket.Room{}
		if data.Rooms != nil {
			rooms = nonNil(*data.Rooms)
		}
		return &adapter.SocketsJoinLeaveMessage{
			Opts:  decodeOptions(data.Opts),
			Rooms: rooms,
		}
	case adapter.DISCONNECT_SOCKETS:
		return &adapter.DisconnectSocketsMessage{
			Opts:  decodeOptions(data.Opts),
			Close: data.Close != nil && *data.Close,
		}
	case adapter.FETCH_SOCKETS:
		return &adapter.FetchSocketsMessage{
			Opts:      decodeOptions(data.Opts),
			RequestId: data.RequestId,
		}
	case adapter.FETCH_SOCKETS_RESPONSE:
		sockets := []adapter.SocketResponse{}
		if data.Sockets != nil {
			sockets = decodeSocketResponses(*data.Sockets)
		}
		return &adapter.FetchSocketsResponse{
			RequestId: data.RequestId,
			Sockets:   sockets,
		}
	case adapter.BROADCAST_CLIENT_COUNT:
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

func encodeOptions(opts *adapter.PacketOptions) *PacketOptions {
	if opts == nil {
		return &PacketOptions{Rooms: []socket.Room{}, Except: []socket.Room{}}
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

func decodeOptions(opts *PacketOptions) *adapter.PacketOptions {
	if opts == nil {
		return &adapter.PacketOptions{Rooms: []socket.Room{}, Except: []socket.Room{}}
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

func nonNil[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
}
