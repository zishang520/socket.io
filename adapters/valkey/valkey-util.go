package valkey

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

	vk "github.com/valkey-io/valkey-go"
	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	// ErrNilValkeyPacket indicates an attempt to unmarshal into a nil ValkeyPacket.
	ErrNilValkeyPacket = errors.New("cannot unmarshal into nil ValkeyPacket")

	errNilValkeyRequest         = errors.New("cannot unmarshal into nil ValkeyRequest")
	errValkeyRequestMissingType = errors.New("ValkeyRequest must contain a type")
)

type (
	valkeyRequest  ValkeyRequest
	valkeyResponse ValkeyResponse
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

func prepareRequest(request *ValkeyRequest, jsonFormat bool) valkeyRequest {
	payload := valkeyRequest(*request)
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

func (r *ValkeyRequest) set(payload *valkeyRequest) error {
	if payload.Type == -1 {
		return errValkeyRequestMissingType
	}

	*r = ValkeyRequest(*payload)
	return nil
}

func (r *ValkeyRequest) MarshalJSON() ([]byte, error) {
	if r == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(prepareRequest(r, true))
}

func (r *ValkeyRequest) UnmarshalJSON(data []byte) error {
	if r == nil {
		return errNilValkeyRequest
	}
	payload := valkeyRequest{Type: -1}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	return r.set(&payload)
}

func (r *ValkeyRequest) MarshalMsgpack() ([]byte, error) {
	if r == nil {
		return msgpack.Marshal(nil)
	}
	return msgpack.Marshal(prepareRequest(r, false))
}

func (r *ValkeyRequest) UnmarshalMsgpack(data []byte) error {
	if r == nil {
		return errNilValkeyRequest
	}
	payload := valkeyRequest{Type: -1}
	if err := msgpack.Unmarshal(data, &payload); err != nil {
		return err
	}
	return r.set(&payload)
}

// MarshalJSON preserves Node.js Buffer values in Valkey responses.
func (r *ValkeyResponse) MarshalJSON() ([]byte, error) {
	if r == nil {
		return json.Marshal(nil)
	}

	payload := valkeyResponse(*r)
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
func (r *ValkeyResponse) UnmarshalJSON(data []byte) error {
	payload := struct {
		*valkeyResponse
		Sockets json.RawMessage `json:"sockets"`
	}{valkeyResponse: (*valkeyResponse)(r)}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	r.Sockets = nil
	if payload.Sockets != nil {
		r.Sockets = payload.Sockets
	}
	return nil
}

// MarshalJSON serializes ValkeyPacket as [uid, packet, opts].
func (r *ValkeyPacket) MarshalJSON() ([]byte, error) {
	if r == nil {
		return json.Marshal(nil)
	}
	packet := marshalPacket(r.Packet, true)
	return json.Marshal([3]any{r.Uid, packet, wireOptions(r.Opts)})
}

// UnmarshalJSON deserializes ValkeyPacket from [uid, packet?, opts?].
func (r *ValkeyPacket) UnmarshalJSON(data []byte) error {
	if r == nil {
		return ErrNilValkeyPacket
	}

	var payload [3]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("failed to unmarshal ValkeyPacket array: %w", err)
	}
	if payload[0] == nil {
		return errors.New("ValkeyPacket array must contain at least 1 element (Uid), got 0")
	}
	if err := json.Unmarshal(payload[0], &r.Uid); err != nil {
		return fmt.Errorf("failed to unmarshal ValkeyPacket Uid: %w", err)
	}

	r.Packet = nil
	if payload[1] != nil {
		if err := json.Unmarshal(payload[1], &r.Packet); err != nil {
			return fmt.Errorf("failed to unmarshal ValkeyPacket Packet: %w", err)
		}
	}

	r.Opts = nil
	if payload[2] != nil {
		if err := json.Unmarshal(payload[2], &r.Opts); err != nil {
			return fmt.Errorf("failed to unmarshal ValkeyPacket Opts: %w", err)
		}
	}
	return nil
}

func (r ValkeyPacket) MarshalMsgpack() ([]byte, error) {
	packet := marshalPacket(r.Packet, false)
	return msgpack.Marshal([3]any{r.Uid, packet, wireOptions(r.Opts)})
}

func (r RawClusterMessage) Uid() string  { return r["uid"] }
func (r RawClusterMessage) Nsp() string  { return r["nsp"] }
func (r RawClusterMessage) Type() string { return r["type"] }
func (r RawClusterMessage) Data() string { return r["data"] }

// EncodeStreamMessage converts a cluster message to the flat Valkey Streams
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
	rawMessage["data"] = vk.BinaryString(data)
	return rawMessage, nil
}

// DecodeStreamMessage decodes the flat field-value format stored in a Valkey
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
