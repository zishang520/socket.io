package postgres

import (
	"encoding/json"
	"io"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

type (
	socketResponse SocketResponse
	nodeBufferJSON []byte
)

func (b nodeBufferJSON) MarshalJSON() ([]byte, error) {
	data := make([]byte, 0, len(b)*4+27)
	data = append(data, `{"type":"Buffer","data":[`...)
	for i, value := range b {
		if i > 0 {
			data = append(data, ',')
		}
		data = strconv.AppendUint(data, uint64(value), 10)
	}
	return append(data, ']', '}'), nil
}

// MarshalJSON preserves the JSON.stringify(Buffer) shape used by Node.js.
func (s SocketResponse) MarshalJSON() ([]byte, error) {
	return json.Marshal(socketResponse(marshalSocketResponse(s, true)))
}

func marshalSocketResponse(response SocketResponse, jsonFormat bool) SocketResponse {
	if data, changed, _ := marshalData(response.Data, jsonFormat); changed {
		response.Data = data
	}
	if handshake := response.Handshake; handshake != nil && handshake.Auth != nil {
		auth, changed, _ := marshalData(handshake.Auth, jsonFormat)
		if changed {
			response.Handshake = new(*handshake)
			response.Handshake.Auth = auth.(map[string]any)
		}
	}
	response.Rooms = utils.NonNilSlice(response.Rooms)
	return response
}

// MarshalAdapterData converts internal cluster data to the Node.js wire shape
// and reports whether the message must use the attachment table. Reader values
// are normalized so remote and local delivery observe the same data.
func MarshalAdapterData(data any) (any, bool) {
	switch value := data.(type) {
	case *adapter.BroadcastMessage:
		packet := value.Packet
		var binary bool
		if packet != nil {
			packet.Data, _, binary = marshalData(packet.Data, false)
		}
		return &PacketData[*parser.Packet]{
			Packet:    packet,
			Opts:      adapter.NormalizeOptions(value.Opts),
			RequestId: value.RequestId,
		}, binary
	case *adapter.SocketsJoinLeaveMessage:
		return &EventData{
			Opts:  adapter.NormalizeOptions(value.Opts),
			Rooms: new(utils.NonNilSlice(value.Rooms)),
		}, false
	case *adapter.DisconnectSocketsMessage:
		return &EventData{
			Opts:  adapter.NormalizeOptions(value.Opts),
			Close: new(value.Close),
		}, false
	case *adapter.FetchSocketsMessage:
		return &EventData{
			Opts:      adapter.NormalizeOptions(value.Opts),
			RequestId: value.RequestId,
		}, false
	case *adapter.FetchSocketsResponse:
		return &EventData{
			RequestId: value.RequestId,
			Sockets:   new(encodeSocketResponses(value.Sockets)),
		}, false
	case *adapter.ServerSideEmitMessage:
		packet, _, binary := marshalData(value.Packet, false)
		value.Packet = packet.([]any)
		return &PacketData[[]any]{
			RequestId: value.RequestId,
			Packet:    utils.NonNilSlice(value.Packet),
		}, binary
	case *adapter.ServerSideEmitResponse:
		payload := *value
		packet, _, binary := marshalData(value.Packet, false)
		payload.Packet = packet
		return &payload, binary
	case *adapter.BroadcastClientCount:
		return &EventData{
			RequestId:   value.RequestId,
			ClientCount: new(value.ClientCount),
		}, false
	case *adapter.BroadcastAck:
		payload := *value
		packet, _, binary := marshalData(value.Packet, false)
		payload.Packet = packet
		return &payload, binary
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
	case adapter.SERVER_SIDE_EMIT_RESPONSE:
		return new(adapter.ServerSideEmitResponse)
	case adapter.BROADCAST_ACK:
		return new(adapter.BroadcastAck)
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
			Opts:      adapter.NormalizeOptions(value.Opts),
			RequestId: value.RequestId,
		}
	case *PacketData[[]any]:
		return &adapter.ServerSideEmitMessage{
			RequestId: value.RequestId,
			Packet:    utils.NonNilSlice(value.Packet),
		}
	case *EventData:
		return unmarshalEventData(messageType, value)
	default:
		return data
	}
}

func marshalData(data any, jsonFormat bool) (any, bool, bool) {
	switch value := data.(type) {
	case nil:
		return nil, false, false
	case *strings.Reader:
		if value == nil {
			return nil, true, false
		}
		var payload strings.Builder
		payload.Grow(value.Len())
		_, _ = value.WriteTo(&payload)
		return payload.String(), true, false
	case *types.StringBuffer:
		if value == nil || value.Buffer == nil {
			return nil, true, false
		}
		return value.String(), true, false
	case []byte:
		payload := utils.NonNilSlice(value)
		if jsonFormat {
			return nodeBufferJSON(payload), true, true
		}
		return payload, value == nil, true
	case *types.BytesBuffer:
		if value == nil {
			return nil, true, false
		}
		var payload []byte
		if value.Buffer != nil {
			payload = value.Bytes()
		}
		payload = utils.NonNilSlice(payload)
		if jsonFormat {
			return nodeBufferJSON(payload), true, true
		}
		return payload, true, true
	case interface {
		io.Reader
		Bytes() []byte
	}:
		if isNil(data) {
			return data, false, false
		}
		payload := utils.NonNilSlice(value.Bytes())
		if jsonFormat {
			return nodeBufferJSON(payload), true, true
		}
		return payload, true, true
	case io.Reader:
		if isNil(data) {
			return data, false, false
		}
		payload, _ := io.ReadAll(value)
		if closer, ok := data.(io.Closer); ok {
			_ = closer.Close()
		}
		payload = utils.NonNilSlice(payload)
		if jsonFormat {
			return nodeBufferJSON(payload), true, true
		}
		return payload, true, true
	case []any:
		var result []any
		var binary bool
		for i, item := range value {
			encoded, changed, hasBinary := marshalData(item, jsonFormat)
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
			encoded, changed, hasBinary := marshalData(item, jsonFormat)
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
		return &adapter.SocketsJoinLeaveMessage{
			Opts:  adapter.NormalizeOptions(data.Opts),
			Rooms: utils.NonNilSlice(utils.FromPtr(data.Rooms)),
		}
	case adapter.DISCONNECT_SOCKETS:
		return &adapter.DisconnectSocketsMessage{
			Opts:  adapter.NormalizeOptions(data.Opts),
			Close: utils.FromPtr(data.Close),
		}
	case adapter.FETCH_SOCKETS:
		return &adapter.FetchSocketsMessage{
			Opts:      adapter.NormalizeOptions(data.Opts),
			RequestId: data.RequestId,
		}
	case adapter.FETCH_SOCKETS_RESPONSE:
		return &adapter.FetchSocketsResponse{
			RequestId: data.RequestId,
			Sockets:   decodeSocketResponses(utils.FromPtr(data.Sockets)),
		}
	case adapter.BROADCAST_CLIENT_COUNT:
		return &adapter.BroadcastClientCount{
			RequestId:   data.RequestId,
			ClientCount: utils.FromPtr(data.ClientCount),
		}
	default:
		return nil
	}
}

func encodeSocketResponses(sockets []adapter.SocketResponse) []SocketResponse {
	responses := make([]SocketResponse, len(sockets))
	for i, details := range sockets {
		responses[i] = marshalSocketResponse(SocketResponse(details), false)
	}
	return responses
}

func decodeSocketResponses(sockets []SocketResponse) []adapter.SocketResponse {
	responses := make([]adapter.SocketResponse, len(sockets))
	for i, details := range sockets {
		responses[i] = adapter.SocketResponse(details)
		responses[i].Rooms = utils.NonNilSlice(responses[i].Rooms)
	}
	return responses
}
