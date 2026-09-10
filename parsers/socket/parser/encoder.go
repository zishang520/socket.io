package parser

import (
	"encoding/json"
	"errors"
	"strconv"

	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// encoder implements the Encoder interface for Socket.IO packet encoding.
type encoder struct{}

// NewEncoder creates a new Encoder instance.
func NewEncoder() Encoder {
	return &encoder{}
}

// Encode encodes a Socket.IO packet into a sequence of buffers.
// For non-binary packets, it returns a single string buffer.
// For binary packets, it returns the encoded packet header followed by binary buffers.
func (e *encoder) Encode(packet *Packet) ([]types.BufferInterface, error) {
	if log.DEBUG.Load() {
		parserLog.Debug("encoding packet %v", packet)
	}
	prepared := *packet
	var buffers []types.BufferInterface
	var attachments *[]types.BufferInterface
	if packet.Type == EVENT || packet.Type == ACK {
		attachments = &buffers
	}
	data, err := prepareData(packet.Data, attachments)
	if err != nil {
		return nil, err
	}
	prepared.Data = data
	if len(buffers) > 0 {
		if packet.Type == EVENT {
			prepared.Type = BINARY_EVENT
		} else {
			prepared.Type = BINARY_ACK
		}
		prepared.Attachments = new(uint64(len(buffers)))
	}
	header, err := e.encodeAsString(prepared)
	if err != nil {
		return nil, err
	}
	return append([]types.BufferInterface{header}, buffers...), nil
}

// ErrPayloadTooLarge indicates that the encoded JSON exceeds the local payload limit.
var ErrPayloadTooLarge = errors.New("encoded payload exceeds maximum size")

// encodeAsString encodes a packet as a string buffer.
// The format is: <type>[<attachments>-][/<namespace>,][<id>][<data>]
func (e *encoder) encodeAsString(packet Packet) (types.BufferInterface, error) {
	// Start with packet type
	buffer := types.NewStringBuffer([]byte{byte(packet.Type) + '0'})

	// Add attachment count for binary packets
	if (packet.Type == BINARY_EVENT || packet.Type == BINARY_ACK) && packet.Attachments != nil {
		buffer.Grow(21)
		encoded := strconv.AppendUint(buffer.AvailableBuffer(), *packet.Attachments, 10)
		encoded = append(encoded, '-')
		_, _ = buffer.Write(encoded)
	}

	// Add namespace (if not the default "/")
	if len(packet.Nsp) > 0 && packet.Nsp != "/" {
		_, _ = buffer.WriteString(packet.Nsp)
		_ = buffer.WriteByte(',')
	}

	// Add packet ID for acknowledgments
	if packet.Id != nil {
		buffer.Grow(20)
		encoded := strconv.AppendUint(buffer.AvailableBuffer(), *packet.Id, 10)
		_, _ = buffer.Write(encoded)
	}

	// Add JSON-encoded data
	if packet.Data != nil {
		jsonBytes, err := json.Marshal(packet.Data)
		if err != nil {
			return nil, err
		}
		if len(jsonBytes) > types.MaxPayloadSize {
			return nil, ErrPayloadTooLarge
		}
		_, _ = buffer.Write(jsonBytes)
	}

	if log.DEBUG.Load() {
		parserLog.Debug("encoded %v as %v", packet, buffer)
	}
	return buffer, nil
}
