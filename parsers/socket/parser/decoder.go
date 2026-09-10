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

const (
	// DefaultMaxAttachments is the default maximum number of binary attachments allowed per packet.
	// This prevents resource exhaustion from malicious clients sending excessively large attachment counts.
	DefaultMaxAttachments uint64 = 10
	// DefaultMaxNamespaceLength is the default maximum allowed length of a namespace name.
	// This prevents resource exhaustion from malicious clients sending excessively long namespace names.
	DefaultMaxNamespaceLength int = 512

	// DefaultMaxPacketIDLength is the default maximum allowed length of a packet ID string.
	// uint64 max is 18446744073709551615 (20 digits).
	DefaultMaxPacketIDLength int = 20
)

var (
	// parserLog is the logger for the parser package.
	parserLog = log.NewLog("socket.io:parser")

	// ReservedEvents contains event names that have special meaning in Socket.IO
	// and cannot be used as custom event names.
	ReservedEvents = types.NewSet(
		"connect",        // Used on the client side to indicate connection
		"connect_error",  // Used on the client side to indicate connection error
		"disconnect",     // Used on both sides to indicate disconnection
		"disconnecting",  // Used on the server side during disconnection
		"newListener",    // Used by the Node.js EventEmitter
		"removeListener", // Used by the Node.js EventEmitter
	)
)

// Error definitions for decoder operations.
var (
	ErrPlaintextDuringReconstruction = errors.New("got plaintext data when reconstructing a packet")
	ErrBinaryWithoutReconstruction   = errors.New("got binary data when not reconstructing a packet")
	ErrInvalidPayload                = errors.New("invalid payload")
	ErrIllegalNamespace              = errors.New("illegal namespace")
	ErrIllegalID                     = errors.New("illegal id")
	ErrTooManyAttachments            = errors.New("too many attachments")
)

// decoder implements the Decoder interface for Socket.IO packet decoding.
type decoder struct {
	types.EventEmitter

	// reconstructor manages binary packet reconstruction state.
	reconstructor atomic.Pointer[binaryReconstructor]

	opts DecoderOptionsInterface
}

// NewDecoder creates a new Decoder instance.
// An optional DecoderOptions can be provided to configure the decoder.
func NewDecoder(opts ...DecoderOptionsInterface) Decoder {
	options := DefaultDecoderOptions()

	if len(opts) > 0 && opts[0] != nil {
		options.Assign(opts[0])
	}

	if options.MaxAttachments() == 0 {
		options.SetMaxAttachments(DefaultMaxAttachments)
	}
	if options.MaxNamespaceLength() <= 0 {
		options.SetMaxNamespaceLength(DefaultMaxNamespaceLength)
	}
	if options.MaxPacketIDLength() <= 0 {
		options.SetMaxPacketIDLength(DefaultMaxPacketIDLength)
	}
	return &decoder{
		EventEmitter: types.NewEventEmitter(),
		opts:         options,
	}
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

	buffer := types.NewBytesBuffer(nil)
	switch typedData := data.(type) {
	case []byte:
		_, _ = buffer.Write(typedData)
	case io.Reader:
		_, readErr := buffer.ReadFrom(typedData)
		if closer, ok := data.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				parserLog.Debug("failed to close binary reader: %v", err)
			}
		}
		if readErr != nil {
			return readErr
		}
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

// decodeAsString decodes a string buffer and handles binary packet initialization.
func (d *decoder) decodeAsString(buffer types.BufferInterface) error {
	packet, err := d.decodePacket(buffer)
	if err != nil {
		parserLog.Debug("decode error: %v", err)
		return err
	}

	if packet.Type == BINARY_EVENT || packet.Type == BINARY_ACK {
		if packet.Type == BINARY_EVENT {
			packet.Type = EVENT
		} else {
			packet.Type = ACK
		}
		d.reconstructor.Store(newBinaryReconstructor(packet))
	} else {
		// Non-binary packet, emit immediately
		d.Emit("decoded", packet)
	}

	return nil
}

// decodePacket parses a packet from a string buffer.
func (d *decoder) decodePacket(buffer types.BufferInterface) (*Packet, error) {
	debug := log.DEBUG.Load()
	var originalStr string
	if debug {
		originalStr = buffer.String()
	}
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

	if debug {
		parserLog.Debug("decoded %s as %v", originalStr, packet)
	}
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

	if attachmentCount == 0 {
		return ErrIllegalAttachments
	}
	if attachmentCount > d.opts.MaxAttachments() {
		return ErrTooManyAttachments
	}

	packet.Attachments = new(attachmentCount)
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
		if unreadErr := buffer.UnreadByte(); unreadErr != nil {
			return ErrIllegalNamespace
		}
		packet.Nsp = "/"
		return nil
	}

	// Read the rest of the namespace until comma
	nspSuffix, err := buffer.ReadString(',')
	if err != nil {
		if err == io.EOF {
			if len(nspSuffix)+1 > d.opts.MaxNamespaceLength() {
				return ErrIllegalNamespace
			}
			packet.Nsp = "/" + nspSuffix
			return nil
		}
		return ErrIllegalNamespace
	}

	// Remove trailing comma
	nsp := "/" + nspSuffix[:len(nspSuffix)-1]
	if len(nsp) > d.opts.MaxNamespaceLength() {
		return ErrIllegalNamespace
	}
	packet.Nsp = nsp
	return nil
}

// parsePacketID reads the optional packet ID for acknowledgments.
func (d *decoder) parsePacketID(buffer types.BufferInterface, packet *Packet) error {
	if buffer.Len() == 0 {
		return nil
	}

	maxLength := d.opts.MaxPacketIDLength()

	data := buffer.Bytes()
	idLength := 0
	for idLength < len(data) && data[idLength] >= '0' && data[idLength] <= '9' {
		idLength++
		if idLength > maxLength {
			buffer.Next(idLength)
			return ErrIllegalID
		}
	}

	if idLength == 0 {
		return nil
	}

	buffer.Next(idLength)
	packetID, err := strconv.ParseUint(string(data[:idLength]), 10, 64)
	if err != nil {
		return err
	}
	packet.Id = new(packetID)

	return nil
}

// parsePayload reads and validates the JSON payload.
func (d *decoder) parsePayload(buffer types.BufferInterface, packet *Packet) error {
	if buffer.Len() == 0 {
		if !isPayloadValid(packet.Type, nil) {
			return ErrInvalidPayload
		}
		return nil
	}

	var payload any
	if err := json.Unmarshal(buffer.Next(buffer.Len()), &payload); err != nil {
		return ErrInvalidPayload
	}

	if payload == nil {
		return ErrInvalidPayload
	}
	if !isPayloadValid(packet.Type, payload) {
		return ErrInvalidPayload
	}

	packet.Data = payload
	return nil
}

// Destroy releases the decoder's resources and stops any ongoing reconstruction.
func (d *decoder) Destroy() {
	if reconstructor := d.reconstructor.Swap(nil); reconstructor != nil {
		reconstructor.finishedReconstruction()
	}
}

// Payload validation helpers

// isPayloadValid checks if the payload matches the expected format for the packet type.
func isPayloadValid(packetType PacketType, payload any) bool {
	switch packetType {
	case CONNECT:
		_, ok := payload.(map[string]any)
		return payload == nil || ok
	case DISCONNECT:
		return payload == nil
	case CONNECT_ERROR:
		switch payload.(type) {
		case map[string]any, string:
			return true
		default:
			return false
		}
	case EVENT, BINARY_EVENT:
		return isValidEventPayload(payload)
	case ACK, BINARY_ACK:
		_, ok := payload.([]any)
		return ok
	default:
		return false
	}
}

// isValidEventPayload validates that an event payload has a valid event name.
// The event name can be either a string (not in reserved events) or a number.
func isValidEventPayload(payload any) bool {
	data, ok := payload.([]any)
	if !ok || len(data) == 0 {
		return false
	}

	// Event name can be a string or a number
	switch eventName := data[0].(type) {
	case string:
		return !ReservedEvents.Has(eventName)
	case float64: // JSON numbers are decoded as float64 in Go
		return true
	case int, int64, int32:
		return true
	default:
		return false
	}
}
