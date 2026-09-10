package adapter

import (
	"encoding/json"
	"errors"
	"slices"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// EncodeClusterMessage uses JSON for plaintext messages and MessagePack for
// messages containing binary values.
func EncodeClusterMessage(message *ClusterMessage) ([]byte, error) {
	wireMessage, binary, err := prepareClusterMessage(message)
	if err != nil {
		return nil, err
	}
	if binary {
		return msgpack.Marshal(wireMessage)
	}
	return json.Marshal(wireMessage)
}

// EncodeClusterMessageMsgpack encodes a cluster message as MessagePack.
func EncodeClusterMessageMsgpack(message *ClusterMessage) ([]byte, error) {
	wireMessage, _, err := prepareClusterMessage(message)
	if err != nil {
		return nil, err
	}
	return msgpack.Marshal(wireMessage)
}

// DecodeClusterMessage decodes a JSON or MessagePack cluster envelope and
// restores the concrete data type associated with the message type.
func DecodeClusterMessage(data []byte) (*ClusterMessage, error) {
	if len(data) == 0 {
		return nil, errors.New("empty cluster message")
	}

	var message ClusterMessage
	var rawData any
	if data[0] == '{' {
		var payload struct {
			Uid  ServerId        `json:"uid"`
			Nsp  string          `json:"nsp"`
			Type MessageType     `json:"type"`
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		message.Uid, message.Nsp, message.Type = payload.Uid, payload.Nsp, payload.Type
		rawData = payload.Data
	} else {
		var payload struct {
			Uid  ServerId           `msgpack:"uid"`
			Nsp  string             `msgpack:"nsp"`
			Type MessageType        `msgpack:"type"`
			Data msgpack.RawMessage `msgpack:"data"`
		}
		if err := msgpack.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		message.Uid, message.Nsp, message.Type = payload.Uid, payload.Nsp, payload.Type
		rawData = payload.Data
	}

	decoded, err := DecodeClusterMessageData(message.Type, rawData)
	if err != nil {
		return nil, err
	}
	message.Data = decoded
	return &message, nil
}

// snapshotClusterMessage freezes a message in its wire representation before
// an asynchronous transport task can outlive the caller's mutable values.
func snapshotClusterMessage(message *ClusterMessage) (*ClusterMessage, error) {
	payload, err := EncodeClusterMessage(message)
	if err != nil {
		return nil, err
	}
	return DecodeClusterMessage(payload)
}

func prepareClusterMessage(message *ClusterMessage) (ClusterMessage, bool, error) {
	wireMessage := *message
	var binary bool
	var err error
	wireMessage.Data, binary, err = EncodeClusterMessageData(message.Data, false)
	return wireMessage, binary, err
}

// EncodeClusterMessageData prepares typed cluster message data for a wire
// encoder. When onlyPlaintext is true, binary traversal is intentionally
// skipped for transports that cannot carry MessagePack payloads.
func EncodeClusterMessageData(data any, onlyPlaintext bool) (any, bool, error) {
	if onlyPlaintext {
		return preparePlaintextClusterData(data), false, nil
	}

	switch value := data.(type) {
	case *BroadcastMessage:
		binary, err := prepareClusterPacket(value.Packet)
		if err != nil {
			return nil, false, err
		}
		opts := clusterWireOptions(value.Opts)
		if opts == value.Opts {
			return value, binary, nil
		}
		payload := *value
		payload.Opts = opts
		return &payload, binary, nil
	case *SocketsJoinLeaveMessage:
		payload := *value
		payload.Opts = clusterWireOptions(value.Opts)
		payload.Rooms = utils.NonNilSlice(value.Rooms)
		return &payload, false, nil
	case *DisconnectSocketsMessage:
		payload := *value
		payload.Opts = clusterWireOptions(value.Opts)
		return &payload, false, nil
	case *FetchSocketsMessage:
		payload := *value
		payload.Opts = clusterWireOptions(value.Opts)
		return &payload, false, nil
	case *FetchSocketsResponse:
		payload := *value
		var binary bool
		var err error
		payload.Sockets, binary, err = prepareClusterSocketResponses(value.Sockets)
		if err != nil {
			return nil, false, err
		}
		return &payload, binary, nil
	case *ServerSideEmitMessage:
		payload := *value
		packet, _, binary, err := PrepareClusterData(value.Packet)
		if err != nil {
			return nil, false, err
		}
		payload.Packet = utils.NonNilSlice(packet.([]any))
		return &payload, binary, nil
	case *ServerSideEmitResponse:
		payload := *value
		packet, _, binary, err := PrepareClusterData(value.Packet)
		if err != nil {
			return nil, false, err
		}
		payload.Packet = packet
		return &payload, binary, nil
	case *BroadcastClientCount:
		return value, false, nil
	case *BroadcastAck:
		payload := *value
		packet, _, binary, err := PrepareClusterData(value.Packet)
		if err != nil {
			return nil, false, err
		}
		payload.Packet = packet
		return &payload, binary, nil
	default:
		return data, false, nil
	}
}

// DecodeClusterMessageData restores typed cluster message data from a
// json.RawMessage or msgpack.RawMessage value.
func DecodeClusterMessageData(messageType MessageType, rawData any) (any, error) {
	var target any
	switch messageType {
	case INITIAL_HEARTBEAT, HEARTBEAT, ADAPTER_CLOSE:
		return nil, nil
	case BROADCAST:
		target = new(BroadcastMessage)
	case SOCKETS_JOIN, SOCKETS_LEAVE:
		target = new(SocketsJoinLeaveMessage)
	case DISCONNECT_SOCKETS:
		target = new(DisconnectSocketsMessage)
	case FETCH_SOCKETS:
		target = new(FetchSocketsMessage)
	case FETCH_SOCKETS_RESPONSE:
		target = new(FetchSocketsResponse)
	case SERVER_SIDE_EMIT:
		target = new(ServerSideEmitMessage)
	case SERVER_SIDE_EMIT_RESPONSE:
		target = new(ServerSideEmitResponse)
	case BROADCAST_CLIENT_COUNT:
		target = new(BroadcastClientCount)
	case BROADCAST_ACK:
		target = new(BroadcastAck)
	default:
		return nil, errors.New("unknown message type")
	}

	switch raw := rawData.(type) {
	case json.RawMessage:
		if err := json.Unmarshal(raw, target); err != nil {
			return nil, err
		}
	case msgpack.RawMessage:
		if err := msgpack.Unmarshal(raw, target); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported data format")
	}

	switch value := target.(type) {
	case *SocketsJoinLeaveMessage:
		value.Rooms = utils.NonNilSlice(value.Rooms)
	case *FetchSocketsResponse:
		if value.Sockets == nil {
			return nil, errors.New("invalid fetch sockets response")
		}
		for i := range value.Sockets {
			value.Sockets[i].Rooms = utils.NonNilSlice(value.Sockets[i].Rooms)
		}
	case *ServerSideEmitMessage:
		value.Packet = utils.NonNilSlice(value.Packet)
	}
	return target, nil
}

func clusterWireOptions(opts *PacketOptions) *PacketOptions {
	if opts.IsValid() && opts.Flags != nil {
		return opts
	}
	options := NormalizeOptions(opts)
	if options.Flags == nil {
		options.Flags = new(socket.BroadcastFlags)
	}
	return options
}

func preparePlaintextClusterData(data any) any {
	switch value := data.(type) {
	case *BroadcastMessage:
		opts := clusterWireOptions(value.Opts)
		if opts == value.Opts {
			return value
		}
		payload := *value
		payload.Opts = opts
		return &payload
	case *SocketsJoinLeaveMessage:
		payload := *value
		payload.Opts = clusterWireOptions(value.Opts)
		payload.Rooms = utils.NonNilSlice(value.Rooms)
		return &payload
	case *DisconnectSocketsMessage:
		payload := *value
		payload.Opts = clusterWireOptions(value.Opts)
		return &payload
	case *FetchSocketsMessage:
		payload := *value
		payload.Opts = clusterWireOptions(value.Opts)
		return &payload
	case *FetchSocketsResponse:
		payload := *value
		payload.Sockets = plaintextClusterSocketResponses(value.Sockets)
		return &payload
	case *ServerSideEmitMessage:
		if value.Packet != nil {
			return value
		}
		payload := *value
		payload.Packet = []any{}
		return &payload
	default:
		return data
	}
}

func prepareClusterSocketResponses(sockets []SocketResponse) ([]SocketResponse, bool, error) {
	var normalized []SocketResponse
	var binary bool
	for i := range sockets {
		details := sockets[i]
		data, changed, hasBinary, err := PrepareClusterData(details.Data)
		if err != nil {
			return nil, false, err
		}
		binary = binary || hasBinary
		if changed {
			details.Data = data
		}
		if handshake := details.Handshake; handshake != nil && handshake.Auth != nil {
			auth, authChanged, authBinary, err := PrepareClusterData(handshake.Auth)
			if err != nil {
				return nil, false, err
			}
			binary = binary || authBinary
			if authChanged {
				details.Handshake = new(*handshake)
				details.Handshake.Auth = auth.(map[string]any)
				changed = true
			}
		}
		if details.Rooms == nil {
			details.Rooms = []socket.Room{}
			changed = true
		}
		if changed {
			if normalized == nil {
				normalized = slices.Clone(sockets)
			}
			normalized[i] = details
		}
	}
	if normalized == nil {
		normalized = sockets
	}
	return utils.NonNilSlice(normalized), binary, nil
}

func plaintextClusterSocketResponses(sockets []SocketResponse) []SocketResponse {
	var normalized []SocketResponse
	for i := range sockets {
		if sockets[i].Rooms != nil {
			continue
		}
		if normalized == nil {
			normalized = slices.Clone(sockets)
		}
		normalized[i].Rooms = []socket.Room{}
	}
	if normalized != nil {
		return normalized
	}
	return utils.NonNilSlice(sockets)
}

func prepareClusterPacket(packet *parser.Packet) (bool, error) {
	if packet == nil {
		return false, nil
	}
	data, changed, binary, err := PrepareClusterData(packet.Data)
	if err != nil {
		return false, err
	}
	if changed {
		// Readers are consumed while being materialized. Keep the native value on
		// the original packet so a subsequent local broadcast sees the same data.
		packet.Data = data
	}
	return binary, nil
}

// PrepareClusterData materializes supported text and binary readers for cluster encoding.
// It returns the value, whether it changed, whether it contains binary, and any read error.
func PrepareClusterData(data any) (any, bool, bool, error) {
	return types.MaterializeData(data)
}
