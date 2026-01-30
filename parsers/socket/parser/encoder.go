package parser

import (
	"encoding/json"
	"strconv"
	"strings"

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
func (e *encoder) Encode(packet *Packet) []types.BufferInterface {
	parserLog.Debug("encoding packet %v", packet)

	// Check if the packet contains binary data and upgrade packet type if needed
	if packet.Type == EVENT || packet.Type == ACK {
		if HasBinary(packet.Data) {
			if packet.Type == EVENT {
				packet.Type = BINARY_EVENT
			} else {
				packet.Type = BINARY_ACK
			}
			return e.encodeAsBinary(packet)
		}
	}

	return []types.BufferInterface{e.encodeAsString(packet)}
}

// encodeAsString encodes a packet as a string buffer.
// The format is: <type>[<attachments>-][/<namespace>,][<id>][<data>]
func (e *encoder) encodeAsString(packet *Packet) types.BufferInterface {
	// Start with packet type
	buffer := types.NewStringBuffer([]byte{byte(packet.Type) + '0'})

	// Add attachment count for binary packets
	if (packet.Type == BINARY_EVENT || packet.Type == BINARY_ACK) && packet.Attachments != nil {
		buffer.WriteString(strconv.FormatUint(*packet.Attachments, 10))
		buffer.WriteByte('-')
	}

	// Add namespace (if not the default "/")
	if len(packet.Nsp) > 0 && packet.Nsp != "/" {
		buffer.WriteString(packet.Nsp)
		buffer.WriteByte(',')
	}

	// Add packet ID for acknowledgments
	if packet.Id != nil {
		buffer.WriteString(strconv.FormatUint(*packet.Id, 10))
	}

	// Add JSON-encoded data
	if packet.Data != nil {
		processedData := preprocessData(packet.Data)
		if jsonBytes, err := json.Marshal(processedData); err == nil {
			buffer.Write(jsonBytes)
		}
	}

	parserLog.Debug("encoded %v as %v", packet, buffer)
	return buffer
}

// encodeAsBinary encodes a packet that contains binary data.
// It deconstructs the packet to extract binary data, then encodes the packet header
// followed by all binary buffers.
func (e *encoder) encodeAsBinary(packet *Packet) []types.BufferInterface {
	deconstructedPacket, buffers := DeconstructPacket(packet)
	header := e.encodeAsString(deconstructedPacket)
	return append([]types.BufferInterface{header}, buffers...)
}

// preprocessData recursively processes data to convert special types
// that need transformation before JSON encoding.
func preprocessData(data any) any {
	switch typedData := data.(type) {
	case nil:
		return nil
	case *strings.Reader:
		// Convert strings.Reader to StringBuffer for proper handling
		buffer, _ := types.NewStringBufferReader(typedData)
		return buffer
	case []any:
		result := make([]any, 0, len(typedData))
		for _, item := range typedData {
			result = append(result, preprocessData(item))
		}
		return result
	case map[string]any:
		result := make(map[string]any, len(typedData))
		for key, value := range typedData {
			result[key] = preprocessData(value)
		}
		return result
	default:
		return data
	}
}
