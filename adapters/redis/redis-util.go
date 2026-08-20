package redis

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	// ErrNilRedisPacket indicates an attempt to unmarshal into a nil RedisPacket.
	ErrNilRedisPacket = errors.New("cannot unmarshal into nil RedisPacket")

	errNilRedisRequest         = errors.New("cannot unmarshal into nil RedisRequest")
	errRedisRequestMissingType = errors.New("RedisRequest must contain a type")
)

type (
	redisRequest   RedisRequest
	redisResponse  RedisResponse
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

func wireOptions(opts *adapter.PacketOptions) *adapter.PacketOptions {
	if opts != nil && opts.Rooms != nil && opts.Except != nil && opts.Flags != nil {
		return opts
	}
	options := adapter.NormalizeOptions(opts)
	if options.Flags == nil {
		options.Flags = new(socket.BroadcastFlags)
	}
	return options
}

// ShouldUseDynamicChannel determines whether a room uses a dynamic Pub/Sub channel.
func ShouldUseDynamicChannel(mode SubscriptionMode, room socket.Room) bool {
	return mode == DynamicPrivateSubscriptionMode ||
		mode == DynamicSubscriptionMode && utils.Utf16CountString(string(room)) != PrivateRoomIdLength
}

func requestOptions(messageType adapter.MessageType, opts *adapter.PacketOptions) *adapter.PacketOptions {
	switch messageType {
	case REMOTE_JOIN, REMOTE_LEAVE, REMOTE_DISCONNECT, REMOTE_FETCH:
		if opts != nil && opts.Rooms != nil && opts.Except != nil && opts.Flags == nil {
			return opts
		}
		options := adapter.NormalizeOptions(opts)
		options.Flags = nil
		return options
	default:
		return wireOptions(opts)
	}
}

func prepareRequest(request *RedisRequest, jsonFormat bool) redisRequest {
	payload := redisRequest(*request)
	payload.Packet, _ = marshalPacket(payload.Packet, jsonFormat)
	if payload.Opts != nil {
		payload.Opts = requestOptions(payload.Type, payload.Opts)
	}
	if payload.Data != nil {
		data, _, _ := marshalData(payload.Data, jsonFormat)
		payload.Data = data.([]any)
	}

	switch payload.Type {
	case SOCKETS:
		payload.Rooms = utils.NonNilSlice(payload.Rooms)
	case REMOTE_JOIN, REMOTE_LEAVE:
		if payload.Opts != nil {
			payload.Rooms = utils.NonNilSlice(payload.Rooms)
		}
	case REMOTE_DISCONNECT:
		if payload.Close == nil {
			payload.Close = new(false)
		}
	case SERVER_SIDE_EMIT:
		payload.Data = utils.NonNilSlice(payload.Data)
	}
	return payload
}

func (r *RedisRequest) set(payload *redisRequest) error {
	if payload.Type == -1 {
		return errRedisRequestMissingType
	}

	*r = RedisRequest(*payload)
	return nil
}

func (r *RedisRequest) MarshalJSON() ([]byte, error) {
	if r == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(prepareRequest(r, true))
}

func (r *RedisRequest) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errNilRedisRequest
	}
	payload := redisRequest{Type: -1}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	return r.set(&payload)
}

func (r *RedisRequest) MarshalMsgpack() ([]byte, error) {
	if r == nil {
		return msgpack.Marshal(nil)
	}
	return msgpack.Marshal(prepareRequest(r, false))
}

func (r *RedisRequest) UnmarshalMsgpack(data []byte) error {
	if r == nil {
		return errNilRedisRequest
	}
	payload := redisRequest{Type: -1}
	if err := msgpack.Unmarshal(data, &payload); err != nil {
		return err
	}
	return r.set(&payload)
}

// MarshalJSON preserves Node.js Buffer values in Redis responses.
func (r *RedisResponse) MarshalJSON() ([]byte, error) {
	if r == nil {
		return json.Marshal(nil)
	}

	payload := redisResponse(*r)
	if sockets, ok := payload.Sockets.([]adapter.SocketResponse); ok {
		payload.Sockets, _ = marshalSocketResponses(sockets, true)
	}
	if payload.Data != nil {
		payload.Data = NormalizeJSONData(payload.Data)
	}
	if payload.Packet != nil {
		payload.Packet = NormalizeJSONData(payload.Packet)
	}
	return json.Marshal(payload)
}

// UnmarshalJSON preserves sockets for request-specific decoding.
func (r *RedisResponse) UnmarshalJSON(data []byte) error {
	payload := struct {
		*redisResponse
		Sockets json.RawMessage `json:"sockets"`
	}{redisResponse: (*redisResponse)(r)}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	r.Sockets = nil
	if payload.Sockets != nil {
		r.Sockets = payload.Sockets
	}
	return nil
}

// MarshalJSON serializes RedisPacket as [uid, packet, opts].
func (r *RedisPacket) MarshalJSON() ([]byte, error) {
	if r == nil {
		return json.Marshal(nil)
	}
	packet, _ := marshalPacket(r.Packet, true)
	return json.Marshal([3]any{r.Uid, packet, wireOptions(r.Opts)})
}

// UnmarshalJSON deserializes RedisPacket from [uid, packet?, opts?].
func (r *RedisPacket) UnmarshalJSON(data []byte) error {
	if r == nil {
		return ErrNilRedisPacket
	}

	var payload [3]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal RedisPacket array: %w", err)
	}
	if payload[0] == nil {
		return errors.New("RedisPacket array must contain at least 1 element (Uid), got 0")
	}
	if err := json.Unmarshal(payload[0], &r.Uid); err != nil {
		return fmt.Errorf("failed to unmarshal RedisPacket Uid: %w", err)
	}

	r.Packet = nil
	if payload[1] != nil {
		if err := json.Unmarshal(payload[1], &r.Packet); err != nil {
			return fmt.Errorf("failed to unmarshal RedisPacket Packet: %w", err)
		}
	}

	r.Opts = nil
	if payload[2] != nil {
		if err := json.Unmarshal(payload[2], &r.Opts); err != nil {
			return fmt.Errorf("failed to unmarshal RedisPacket Opts: %w", err)
		}
	}
	return nil
}

func (r RedisPacket) MarshalMsgpack() ([]byte, error) {
	packet, _ := marshalPacket(r.Packet, false)
	return msgpack.Marshal([3]any{r.Uid, packet, wireOptions(r.Opts)})
}

func (r RawClusterMessage) stringValue(key string) string {
	value, _ := r[key].(string)
	return value
}

func (r RawClusterMessage) Uid() string  { return r.stringValue("uid") }
func (r RawClusterMessage) Nsp() string  { return r.stringValue("nsp") }
func (r RawClusterMessage) Type() string { return r.stringValue("type") }
func (r RawClusterMessage) Data() string { return r.stringValue("data") }

func marshalClusterMessage(message *adapter.ClusterMessage) (adapter.ClusterMessage, bool) {
	wireMessage := *message
	var binary bool
	wireMessage.Data, binary = marshalClusterData(message.Data, false)
	return wireMessage, binary
}

// EncodeClusterMessage uses JSON for plaintext messages and MessagePack for binary messages.
func EncodeClusterMessage(message *adapter.ClusterMessage) ([]byte, error) {
	wireMessage, binary := marshalClusterMessage(message)
	if binary {
		return msgpack.Marshal(wireMessage)
	}
	return json.Marshal(wireMessage)
}

// EncodeClusterMessageMsgpack encodes a cluster message in the MessagePack format used by Streams PUB/SUB.
func EncodeClusterMessageMsgpack(message *adapter.ClusterMessage) ([]byte, error) {
	wireMessage, _ := marshalClusterMessage(message)
	return msgpack.Marshal(wireMessage)
}

// UnmarshalClusterMessage decodes the JSON or MessagePack cluster envelope.
func UnmarshalClusterMessage(data []byte) (*adapter.ClusterMessage, error) {
	if len(data) == 0 {
		return nil, errors.New("empty cluster message")
	}

	var message adapter.ClusterMessage
	var rawData any
	if data[0] == '{' {
		var payload struct {
			Uid  adapter.ServerId    `json:"uid"`
			Nsp  string              `json:"nsp"`
			Type adapter.MessageType `json:"type"`
			Data json.RawMessage     `json:"data"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		message.Uid, message.Nsp, message.Type = payload.Uid, payload.Nsp, payload.Type
		rawData = payload.Data
	} else {
		var payload struct {
			Uid  adapter.ServerId    `msgpack:"uid"`
			Nsp  string              `msgpack:"nsp"`
			Type adapter.MessageType `msgpack:"type"`
			Data msgpack.RawMessage  `msgpack:"data"`
		}
		if err := msgpack.Unmarshal(data, &payload); err != nil {
			return nil, err
		}
		message.Uid, message.Nsp, message.Type = payload.Uid, payload.Nsp, payload.Type
		rawData = payload.Data
	}

	decoded, err := unmarshalClusterData(message.Type, rawData)
	if err != nil {
		return nil, err
	}
	message.Data = decoded
	return &message, nil
}

// EncodeStreamMessage converts a cluster message to the flat Redis Streams
// field-value format used by the Node.js adapter and emitter.
func EncodeStreamMessage(message *adapter.ClusterMessage, onlyPlaintext bool) (RawClusterMessage, error) {
	wireData, binary := marshalClusterData(message.Data, onlyPlaintext)
	rawMessage := RawClusterMessage{
		"uid":  string(message.Uid),
		"nsp":  message.Nsp,
		"type": strconv.Itoa(int(message.Type)),
	}
	if wireData == nil {
		return rawMessage, nil
	}

	if binary {
		data, err := msgpack.Marshal(wireData)
		if err != nil {
			return nil, err
		}
		rawMessage["data"] = base64.StdEncoding.EncodeToString(data)
		return rawMessage, nil
	}

	data, err := json.Marshal(wireData)
	if err != nil {
		return nil, err
	}
	rawMessage["data"] = string(data)
	return rawMessage, nil
}

// DecodeStreamMessage decodes the flat field-value format stored in a Redis
// Stream entry and restores the concrete cluster message data type.
func DecodeStreamMessage(rawMessage RawClusterMessage) (*adapter.ClusterMessage, error) {
	messageType, err := strconv.Atoi(rawMessage.Type())
	if err != nil {
		return nil, err
	}

	message := &adapter.ClusterMessage{
		Uid:  adapter.ServerId(rawMessage.Uid()),
		Nsp:  rawMessage.Nsp(),
		Type: adapter.MessageType(messageType),
	}
	data := rawMessage.Data()
	if data == "" {
		return message, nil
	}

	var rawData any
	if data[0] == '{' {
		rawData = json.RawMessage(data)
	} else {
		decoded, decodeErr := base64.StdEncoding.DecodeString(data)
		if decodeErr != nil {
			return nil, decodeErr
		}
		rawData = msgpack.RawMessage(decoded)
	}
	message.Data, err = unmarshalClusterData(message.Type, rawData)
	if err != nil {
		return nil, err
	}
	return message, nil
}

// XAdd appends a Socket.IO message with the same unconditional MAXLEN clause as the Node.js implementation.
func XAdd(client *RedisClient, stream string, message RawClusterMessage, maxLen int64) (string, error) {
	args := make([]any, 0, 14)
	args = append(args, "XADD", stream, "MAXLEN", "~", maxLen, "*")
	for _, field := range [...]string{"uid", "nsp", "type", "data"} {
		if value, ok := message[field]; ok {
			args = append(args, field, value)
		}
	}
	return client.Client().Do(client.Context(), args...).Text()
}

func marshalSocketResponses(sockets []adapter.SocketResponse, jsonFormat bool) ([]adapter.SocketResponse, bool) {
	var normalized []adapter.SocketResponse
	var binary bool
	for i := range sockets {
		details := sockets[i]
		data, changed, hasBinary := marshalData(details.Data, jsonFormat)
		binary = binary || hasBinary
		if changed {
			details.Data = data
		}
		if handshake := details.Handshake; handshake != nil && handshake.Auth != nil {
			auth, authChanged, authBinary := marshalData(handshake.Auth, jsonFormat)
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
	return utils.NonNilSlice(normalized), binary
}

func marshalClusterData(data any, onlyPlaintext bool) (any, bool) {
	switch value := data.(type) {
	case *adapter.BroadcastMessage:
		packet, binary := marshalPacket(value.Packet, onlyPlaintext)
		opts := wireOptions(value.Opts)
		if packet == value.Packet && opts == value.Opts {
			return value, !onlyPlaintext && binary
		}
		payload := *value
		payload.Packet, payload.Opts = packet, opts
		return &payload, !onlyPlaintext && binary
	case *adapter.SocketsJoinLeaveMessage:
		payload := *value
		payload.Opts = wireOptions(value.Opts)
		payload.Rooms = utils.NonNilSlice(value.Rooms)
		return &payload, false
	case *adapter.DisconnectSocketsMessage:
		payload := *value
		payload.Opts = wireOptions(value.Opts)
		return &payload, false
	case *adapter.FetchSocketsMessage:
		payload := *value
		payload.Opts = wireOptions(value.Opts)
		return &payload, false
	case *adapter.FetchSocketsResponse:
		payload := *value
		var binary bool
		payload.Sockets, binary = marshalSocketResponses(value.Sockets, onlyPlaintext)
		return &payload, !onlyPlaintext && binary
	case *adapter.ServerSideEmitMessage:
		payload := *value
		packet, _, binary := marshalData(value.Packet, onlyPlaintext)
		payload.Packet = utils.NonNilSlice(packet.([]any))
		return &payload, !onlyPlaintext && binary
	case *adapter.ServerSideEmitResponse:
		payload := *value
		packet, _, binary := marshalData(value.Packet, onlyPlaintext)
		payload.Packet = packet
		return &payload, !onlyPlaintext && binary
	case *adapter.BroadcastClientCount:
		return value, false
	case *adapter.BroadcastAck:
		payload := *value
		packet, _, binary := marshalData(value.Packet, onlyPlaintext)
		payload.Packet = packet
		return &payload, !onlyPlaintext && binary
	default:
		return data, false
	}
}

func unmarshalClusterData(messageType adapter.MessageType, rawData any) (any, error) {
	var target any
	switch messageType {
	case adapter.INITIAL_HEARTBEAT, adapter.HEARTBEAT, adapter.ADAPTER_CLOSE:
		return nil, nil
	case adapter.BROADCAST:
		target = new(adapter.BroadcastMessage)
	case adapter.SOCKETS_JOIN, adapter.SOCKETS_LEAVE:
		target = new(adapter.SocketsJoinLeaveMessage)
	case adapter.DISCONNECT_SOCKETS:
		target = new(adapter.DisconnectSocketsMessage)
	case adapter.FETCH_SOCKETS:
		target = new(adapter.FetchSocketsMessage)
	case adapter.FETCH_SOCKETS_RESPONSE:
		target = new(adapter.FetchSocketsResponse)
	case adapter.SERVER_SIDE_EMIT:
		target = new(adapter.ServerSideEmitMessage)
	case adapter.SERVER_SIDE_EMIT_RESPONSE:
		target = new(adapter.ServerSideEmitResponse)
	case adapter.BROADCAST_CLIENT_COUNT:
		target = new(adapter.BroadcastClientCount)
	case adapter.BROADCAST_ACK:
		target = new(adapter.BroadcastAck)
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
	case *adapter.SocketsJoinLeaveMessage:
		value.Rooms = utils.NonNilSlice(value.Rooms)
	case *adapter.FetchSocketsResponse:
		value.Sockets = utils.NonNilSlice(value.Sockets)
		for i := range value.Sockets {
			value.Sockets[i].Rooms = utils.NonNilSlice(value.Sockets[i].Rooms)
		}
	case *adapter.ServerSideEmitMessage:
		value.Packet = utils.NonNilSlice(value.Packet)
	}
	return target, nil
}

// NormalizeData materializes supported readers while preserving slices and maps.
// Compound cross-language payloads must use []any or map[string]any so nested binary values can be traversed.
func NormalizeData(data any) any {
	normalized, _, _ := marshalData(data, false)
	return normalized
}

// NormalizeJSONData converts binary values to the JSON.stringify(Buffer) shape.
// Compound cross-language payloads must use []any or map[string]any so nested binary values can be traversed.
func NormalizeJSONData(data any) any {
	normalized, _, _ := marshalData(data, true)
	return normalized
}

func marshalPacket(packet *parser.Packet, jsonFormat bool) (*parser.Packet, bool) {
	if packet == nil {
		return nil, false
	}
	data, changed, binary := marshalData(packet.Data, false)
	if changed {
		// Readers are consumed while being materialized. Keep the native value on
		// the original packet so a subsequent local broadcast sees the same data.
		packet.Data = data
	}
	if !jsonFormat || !binary {
		return packet, binary
	}

	data, changed, _ = marshalData(packet.Data, true)
	if !changed {
		return packet, binary
	}
	wirePacket := new(*packet)
	wirePacket.Data = data
	return wirePacket, binary
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
		if jsonFormat {
			return nodeBufferJSON(value), true, true
		}
		return utils.NonNilSlice(value), value == nil, true
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
		if utils.IsNil(data) {
			return data, false, false
		}
		payload := utils.NonNilSlice(value.Bytes())
		if jsonFormat {
			return nodeBufferJSON(payload), true, true
		}
		return payload, true, true
	case io.Reader:
		if utils.IsNil(data) {
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
	default:
		return data, false, false
	}
}
