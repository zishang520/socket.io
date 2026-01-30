package parser

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

var (
	// parserLog is the logger for the parser package.
	parserLog = log.NewLog("socket.io:parser")

	// ReservedEvents contains event names that have special meaning in Socket.IO
	// and cannot be used as custom event names.
	ReservedEvents = types.NewSet(
		"connect",       // Used on the client side to indicate connection
		"connect_error", // Used on the client side to indicate connection error
		"disconnect",    // Used on both sides to indicate disconnection
		"disconnecting", // Used on the server side during disconnection
	)
)

// Error definitions for decoder operations.
var (
	ErrPlaintextDuringReconstruction = errors.New("got plaintext data when reconstructing a packet")
	ErrBinaryWithoutReconstruction   = errors.New("got binary data when not reconstructing a packet")
	ErrInvalidPayload                = errors.New("invalid payload")
	ErrIllegalNamespace              = errors.New("illegal namespace")
	ErrIllegalID                     = errors.New("illegal id")
)

// decoder implements the Decoder interface for Socket.IO packet decoding.
type decoder struct {
	types.EventEmitter

	// reconstructor manages binary packet reconstruction state.
	reconstructor atomic.Pointer[binaryReconstructor]
}

// NewDecoder creates a new Decoder instance.
func NewDecoder() Decoder {
	return &decoder{EventEmitter: types.NewEventEmitter()}
}

// Add processes incoming data (string or binary) and emits decoded packets.
// For string data, it decodes immediately. For binary data, it accumulates
// buffers until the packet is complete, then emits the reconstructed packet.
func (d *decoder) Add(data any) error {
	switch typedData := data.(type) {
	case string:
		return d.handleStringData(types.NewStringBufferString(typedData))

	case *strings.Reader:
		buffer, err := types.NewStringBufferReader(typedData)
		if err != nil {
			return err
		}
		return d.handleStringData(buffer)

	case *types.StringBuffer:
		return d.handleStringData(typedData)

	default:
		return d.handleBinaryData(data)
	}
}

// handleStringData processes string-based packet data.
func (d *decoder) handleStringData(buffer types.BufferInterface) error {
	if d.reconstructor.Load() != nil {
		return ErrPlaintextDuringReconstruction
	}
	return d.decodeAsString(buffer)
}

// handleBinaryData processes binary packet data for reconstruction.
func (d *decoder) handleBinaryData(data any) error {
	if !IsBinary(data) {
		return fmt.Errorf("unknown type: %T", data)
	}

	reconstructor := d.reconstructor.Load()
	if reconstructor == nil {
		return ErrBinaryWithoutReconstruction
	}

	buffer, err := d.readBinaryData(data)
	if err != nil {
		return err
	}

	packet, err := reconstructor.takeBinaryData(buffer)
	if err != nil {
		return fmt.Errorf("decode error: %w", err)
	}

	if packet != nil {
		// Received final buffer, packet is complete
		d.reconstructor.Store(nil)
		d.Emit("decoded", packet)
	}

	return nil
}

// readBinaryData reads binary data from various source types into a buffer.
func (d *decoder) readBinaryData(data any) (types.BufferInterface, error) {
	buffer := types.NewBytesBuffer(nil)

	switch typedData := data.(type) {
	case io.Reader:
		if closer, ok := data.(io.Closer); ok {
			defer closer.Close()
		}
		if _, err := buffer.ReadFrom(typedData); err != nil {
			return nil, err
		}
	case []byte:
		if _, err := buffer.Write(typedData); err != nil {
			return nil, err
		}
	}

	return buffer, nil
}

// decodeAsString decodes a string buffer and handles binary packet initialization.
func (d *decoder) decodeAsString(buffer types.BufferInterface) error {
	packet, err := d.decodePacket(buffer)
	if err != nil {
		parserLog.Debug("decode error: %v", err)
		return err
	}

	if packet.Type == BINARY_EVENT || packet.Type == BINARY_ACK {
		d.reconstructor.Store(newBinaryReconstructor(packet))
		// If no attachments expected, emit immediately
		if packet.Attachments != nil && *packet.Attachments == 0 {
			d.Emit("decoded", packet)
		}
	} else {
		// Non-binary packet, emit immediately
		d.Emit("decoded", packet)
	}

	return nil
}

// decodePacket parses a packet from a string buffer.
func (d *decoder) decodePacket(buffer types.BufferInterface) (*Packet, error) {
	originalStr := buffer.String() // For debug logging
	packet := &Packet{}

	// Parse packet type
	if err := d.parsePacketType(buffer, packet); err != nil {
		return nil, err
	}

	// Parse attachments for binary packets
	if err := d.parseAttachments(buffer, packet); err != nil {
		return nil, err
	}

	// Parse namespace
	if err := d.parseNamespace(buffer, packet); err != nil {
		return nil, err
	}

	// Parse packet ID
	if err := d.parsePacketID(buffer, packet); err != nil {
		return nil, err
	}

	// Parse payload data
	if err := d.parsePayload(buffer, packet); err != nil {
		return nil, err
	}

	parserLog.Debug("decoded %s as %v", originalStr, packet)
	return packet, nil
}

// parsePacketType reads and validates the packet type.
func (d *decoder) parsePacketType(buffer types.BufferInterface, packet *Packet) error {
	typeByte, err := buffer.ReadByte()
	if err != nil {
		return ErrInvalidPayload
	}

	packet.Type = PacketType(int(typeByte) - '0')
	if !packet.Type.Valid() {
		return fmt.Errorf("unknown packet type %d", packet.Type)
	}

	return nil
}

// parseAttachments reads attachment count for binary packets.
func (d *decoder) parseAttachments(buffer types.BufferInterface, packet *Packet) error {
	if packet.Type != BINARY_EVENT && packet.Type != BINARY_ACK {
		return nil
	}

	attachmentStr, err := buffer.ReadString('-')
	if err != nil {
		return ErrIllegalAttachments
	}

	strLen := len(attachmentStr)
	if strLen < 2 { // Must be at least "X-" where X is a digit
		return ErrIllegalAttachments
	}

	attachmentCount, err := strconv.ParseUint(attachmentStr[:strLen-1], 10, 64)
	if err != nil {
		return ErrIllegalAttachments
	}

	packet.Attachments = &attachmentCount
	return nil
}

// parseNamespace reads the namespace from the buffer.
func (d *decoder) parseNamespace(buffer types.BufferInterface, packet *Packet) error {
	firstByte, err := buffer.ReadByte()
	if err != nil {
		if err == io.EOF {
			packet.Nsp = "/"
			return nil
		}
		return ErrIllegalNamespace
	}

	if firstByte != '/' {
		// No namespace specified, use default and put byte back
		if err := buffer.UnreadByte(); err != nil {
			return ErrIllegalNamespace
		}
		packet.Nsp = "/"
		return nil
	}

	// Read the rest of the namespace until comma
	nspSuffix, err := buffer.ReadString(',')
	if err != nil {
		if err == io.EOF {
			packet.Nsp = "/" + nspSuffix
			return nil
		}
		return ErrIllegalNamespace
	}

	// Remove trailing comma
	packet.Nsp = "/" + nspSuffix[:len(nspSuffix)-1]
	return nil
}

// parsePacketID reads the optional packet ID for acknowledgments.
func (d *decoder) parsePacketID(buffer types.BufferInterface, packet *Packet) error {
	if buffer.Len() == 0 {
		return nil
	}

	var idBuilder strings.Builder

	for {
		b, err := buffer.ReadByte()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if b >= '0' && b <= '9' {
			if err := idBuilder.WriteByte(b); err != nil {
				return err
			}
		} else {
			if err := buffer.UnreadByte(); err != nil {
				return ErrIllegalID
			}
			break
		}
	}

	if idBuilder.Len() > 0 {
		packetID, err := strconv.ParseUint(idBuilder.String(), 10, 64)
		if err != nil {
			return err
		}
		packet.Id = &packetID
	}

	return nil
}

// parsePayload reads and validates the JSON payload.
func (d *decoder) parsePayload(buffer types.BufferInterface, packet *Packet) error {
	if buffer.Len() == 0 {
		return d.validatePayload(packet.Type, nil)
	}

	var payload any
	if err := json.NewDecoder(buffer).Decode(&payload); err != nil {
		return ErrInvalidPayload
	}

	if err := d.validatePayload(packet.Type, payload); err != nil {
		return err
	}

	packet.Data = payload
	return nil
}

// validatePayload checks if the payload is valid for the given packet type.
func (d *decoder) validatePayload(packetType PacketType, payload any) error {
	if !isPayloadValid(packetType, payload) {
		return ErrInvalidPayload
	}
	return nil
}

// Destroy releases the decoder's resources and stops any ongoing reconstruction.
func (d *decoder) Destroy() {
	if reconstructor := d.reconstructor.Load(); reconstructor != nil {
		reconstructor.finishedReconstruction()
	}
}

// Payload validation helpers

// isPayloadValid checks if the payload matches the expected format for the packet type.
func isPayloadValid(packetType PacketType, payload any) bool {
	switch packetType {
	case CONNECT:
		return payload == nil || isMap(payload)
	case DISCONNECT:
		return payload == nil
	case CONNECT_ERROR:
		return isMap(payload) || isString(payload)
	case EVENT, BINARY_EVENT:
		return isValidEventPayload(payload)
	case ACK, BINARY_ACK:
		return isSlice(payload)
	default:
		return false
	}
}

// isMap checks if the payload is a map[string]any.
func isMap(payload any) bool {
	_, ok := payload.(map[string]any)
	return ok
}

// isString checks if the payload is a string.
func isString(payload any) bool {
	_, ok := payload.(string)
	return ok
}

// isSlice checks if the payload is a slice.
func isSlice(payload any) bool {
	_, ok := payload.([]any)
	return ok
}

// isValidEventPayload validates that an event payload has a valid event name.
func isValidEventPayload(payload any) bool {
	data, ok := payload.([]any)
	if !ok || len(data) == 0 {
		return false
	}

	eventName, isString := data[0].(string)
	return isString && !ReservedEvents.Has(eventName)
}
