package redis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
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

func requestOptions(messageType RequestType, opts *adapter.PacketOptions) *adapter.PacketOptions {
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
	payload.Packet = marshalPacket(payload.Packet, jsonFormat)
	if payload.Opts != nil {
		payload.Opts = requestOptions(payload.Type, payload.Opts)
	}
	if payload.Data != nil {
		data, _, _ := marshalData(payload.Data, jsonFormat)
		payload.Data = data.([]any)
	}

	switch payload.Type {
	case SOCKETS, REMOTE_JOIN, REMOTE_LEAVE:
		payload.Rooms = utils.NonNilSlice(payload.Rooms)
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
		payload.Sockets = marshalJSONSocketResponses(sockets)
	}
	if payload.Data != nil {
		payload.Data = normalizeJSONData(payload.Data)
	}
	if payload.Packet != nil {
		payload.Packet = normalizeJSONData(payload.Packet)
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
	packet := marshalPacket(r.Packet, true)
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
	packet := marshalPacket(r.Packet, false)
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

// EncodeStreamMessage converts a cluster message to the flat Redis Streams
// field-value format used by the Node.js adapter and emitter.
func EncodeStreamMessage(message *adapter.ClusterMessage, onlyPlaintext bool) (RawClusterMessage, error) {
	wireData, binary := adapter.EncodeClusterMessageData(message.Data, onlyPlaintext)
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
	message.Data, err = adapter.DecodeClusterMessageData(message.Type, rawData)
	if err != nil {
		return nil, err
	}
	return message, nil
}

// XAdd appends a Socket.IO message with the same unconditional MAXLEN clause as the Node.js implementation.
func XAdd(client *RedisClient, stream string, message RawClusterMessage, maxLen int64) (string, error) {
	return XAddContext(client.Context(), client, stream, message, maxLen)
}

// XAddContext appends a Socket.IO message with the given operation context.
func XAddContext(ctx context.Context, client *RedisClient, stream string, message RawClusterMessage, maxLen int64) (string, error) {
	args := make([]any, 0, 14)
	args = append(args, "XADD", stream, "MAXLEN", "~", maxLen, "*")
	for _, field := range [...]string{"uid", "nsp", "type", "data"} {
		if value, ok := message[field]; ok {
			args = append(args, field, value)
		}
	}
	return client.Client().Do(ctx, args...).Text()
}

func marshalJSONSocketResponses(sockets []adapter.SocketResponse) []adapter.SocketResponse {
	var normalized []adapter.SocketResponse
	for i := range sockets {
		details := sockets[i]
		data, changed, _ := marshalData(details.Data, true)
		if changed {
			details.Data = data
		}
		if handshake := details.Handshake; handshake != nil && handshake.Auth != nil {
			auth, authChanged, _ := marshalData(handshake.Auth, true)
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
	return utils.NonNilSlice(normalized)
}

// NormalizeData materializes supported readers while preserving slices and maps.
// Compound cross-language payloads must use []any or map[string]any so nested binary values can be traversed.
func NormalizeData(data any) any {
	normalized, _, _ := marshalData(data, false)
	return normalized
}

// normalizeJSONData converts binary values to the JSON.stringify(Buffer) shape.
// Compound cross-language payloads must use []any or map[string]any so nested binary values can be traversed.
func normalizeJSONData(data any) any {
	normalized, _, _ := marshalData(data, true)
	return normalized
}

func marshalPacket(packet *parser.Packet, jsonFormat bool) *parser.Packet {
	if packet == nil {
		return nil
	}
	data, changed, binary := marshalData(packet.Data, false)
	if changed {
		// Readers are consumed while being materialized. Keep the native value on
		// the original packet so a subsequent local broadcast sees the same data.
		packet.Data = data
	}
	if !jsonFormat || !binary {
		return packet
	}

	data, changed, _ = marshalData(packet.Data, true)
	if !changed {
		return packet
	}
	wirePacket := new(*packet)
	wirePacket.Data = data
	return wirePacket
}

func marshalData(data any, jsonFormat bool) (any, bool, bool) {
	switch value := data.(type) {
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
	prepared, changed, binary := adapter.PrepareClusterData(data)
	if jsonFormat && binary {
		return nodeBufferJSON(prepared.([]byte)), true, true
	}
	return prepared, changed, binary
}
