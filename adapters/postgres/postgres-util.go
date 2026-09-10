package postgres

import (
	"encoding/json"
	"maps"
	"slices"
	"strconv"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
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
	response, err := marshalSocketResponse(s, true)
	if err != nil {
		return nil, err
	}
	return json.Marshal(socketResponse(response))
}

func marshalSocketResponse(response SocketResponse, jsonFormat bool) (SocketResponse, error) {
	data, changed, _, err := marshalData(response.Data, jsonFormat)
	if err != nil {
		return response, err
	}
	if changed {
		response.Data = data
	}
	if handshake := response.Handshake; handshake != nil && handshake.Auth != nil {
		auth, changed, _, err := marshalData(handshake.Auth, jsonFormat)
		if err != nil {
			return response, err
		}
		if changed {
			response.Handshake = new(*handshake)
			response.Handshake.Auth = auth.(map[string]any)
		}
	}
	response.Rooms = utils.NonNilSlice(response.Rooms)
	return response, nil
}

// MarshalAdapterData converts internal cluster data to the Node.js wire shape
// and reports whether the message must use the attachment table. Reader values
// are normalized so remote and local delivery observe the same data.
func MarshalAdapterData(data any) (any, bool, error) {
	switch value := data.(type) {
	case *adapter.BroadcastMessage:
		packet := value.Packet
		var binary bool
		if packet != nil {
			prepared, _, hasBinary, err := marshalData(packet.Data, false)
			if err != nil {
				return nil, false, err
			}
			packet.Data, binary = prepared, hasBinary
		}
		return &PacketData[*parser.Packet]{
			Packet:    packet,
			Opts:      adapter.NormalizeOptions(value.Opts),
			RequestId: value.RequestId,
		}, binary, nil
	case *adapter.SocketsJoinLeaveMessage:
		return &EventData{
			Opts:  adapter.NormalizeOptions(value.Opts),
			Rooms: new(utils.NonNilSlice(value.Rooms)),
		}, false, nil
	case *adapter.DisconnectSocketsMessage:
		return &EventData{
			Opts:  adapter.NormalizeOptions(value.Opts),
			Close: new(value.Close),
		}, false, nil
	case *adapter.FetchSocketsMessage:
		return &EventData{
			Opts:      adapter.NormalizeOptions(value.Opts),
			RequestId: value.RequestId,
		}, false, nil
	case *adapter.FetchSocketsResponse:
		sockets, err := encodeSocketResponses(value.Sockets)
		if err != nil {
			return nil, false, err
		}
		return &EventData{
			RequestId: value.RequestId,
			Sockets:   new(sockets),
		}, false, nil
	case *adapter.ServerSideEmitMessage:
		packet, _, binary, err := marshalData(value.Packet, false)
		if err != nil {
			return nil, false, err
		}
		value.Packet = packet.([]any)
		return &PacketData[[]any]{
			RequestId: value.RequestId,
			Packet:    utils.NonNilSlice(value.Packet),
		}, binary, nil
	case *adapter.ServerSideEmitResponse:
		payload := *value
		packet, _, binary, err := marshalData(value.Packet, false)
		if err != nil {
			return nil, false, err
		}
		payload.Packet = packet
		return &payload, binary, nil
	case *adapter.BroadcastClientCount:
		return &EventData{
			RequestId:   value.RequestId,
			ClientCount: new(value.ClientCount),
		}, false, nil
	case *adapter.BroadcastAck:
		payload := *value
		packet, _, binary, err := marshalData(value.Packet, false)
		if err != nil {
			return nil, false, err
		}
		payload.Packet = packet
		return &payload, binary, nil
	default:
		return data, false, nil
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
			Opts:      value.Opts,
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

func marshalData(data any, jsonFormat bool) (any, bool, bool, error) {
	switch value := data.(type) {
	case []any:
		var result []any
		var binary bool
		for i, item := range value {
			encoded, changed, hasBinary, err := marshalData(item, jsonFormat)
			if err != nil {
				return nil, false, false, err
			}
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
			return result, true, binary, nil
		}
		return data, false, binary, nil
	case map[string]any:
		var result map[string]any
		var binary bool
		for key, item := range value {
			encoded, changed, hasBinary, err := marshalData(item, jsonFormat)
			if err != nil {
				return nil, false, false, err
			}
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
			return result, true, binary, nil
		}
		return data, false, binary, nil
	}
	prepared, changed, binary, err := adapter.PrepareClusterData(data)
	if err != nil {
		return nil, false, false, err
	}
	if jsonFormat && binary {
		return nodeBufferJSON(prepared.([]byte)), true, true, nil
	}
	return prepared, changed, binary, nil
}

func unmarshalEventData(messageType adapter.MessageType, data *EventData) any {
	switch messageType {
	case adapter.SOCKETS_JOIN, adapter.SOCKETS_LEAVE:
		return &adapter.SocketsJoinLeaveMessage{
			Opts:  data.Opts,
			Rooms: utils.NonNilSlice(utils.FromPtr(data.Rooms)),
		}
	case adapter.DISCONNECT_SOCKETS:
		return &adapter.DisconnectSocketsMessage{
			Opts:  data.Opts,
			Close: utils.FromPtr(data.Close),
		}
	case adapter.FETCH_SOCKETS:
		return &adapter.FetchSocketsMessage{
			Opts:      data.Opts,
			RequestId: data.RequestId,
		}
	case adapter.FETCH_SOCKETS_RESPONSE:
		if data.Sockets == nil {
			return nil
		}
		return &adapter.FetchSocketsResponse{
			RequestId: data.RequestId,
			Sockets:   decodeSocketResponses(*data.Sockets),
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

func encodeSocketResponses(sockets []adapter.SocketResponse) ([]SocketResponse, error) {
	responses := make([]SocketResponse, len(sockets))
	for i, details := range sockets {
		var err error
		responses[i], err = marshalSocketResponse(SocketResponse(details), false)
		if err != nil {
			return nil, err
		}
	}
	return responses, nil
}

func decodeSocketResponses(sockets []SocketResponse) []adapter.SocketResponse {
	responses := make([]adapter.SocketResponse, len(sockets))
	for i, details := range sockets {
		responses[i] = adapter.SocketResponse(details)
		responses[i].Rooms = utils.NonNilSlice(responses[i].Rooms)
	}
	return responses
}
