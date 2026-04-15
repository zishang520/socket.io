// Package unix provides Unix Domain Socket-based adapter types and interfaces for Socket.IO clustering.
// These types define the message structures used for inter-node communication via Unix Domain Sockets.
package unix

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// ErrNilUnixPacket indicates an attempt to unmarshal into a nil UnixPacket.
var ErrNilUnixPacket = errors.New("cannot unmarshal into nil UnixPacket")

type (
	// UnixPacket represents a packet to be broadcast via Unix Domain Socket.
	// It contains the server UID, the Socket.IO packet, and broadcast options.
	UnixPacket struct {
		// Uid identifies the source server that sent this packet.
		Uid adapter.ServerId `json:"-"`

		// Packet is the Socket.IO packet to be broadcast.
		Packet *parser.Packet `json:"-"`

		// Opts contains the broadcast options including target rooms and exclusions.
		Opts *adapter.PacketOptions `json:"-"`
	}

	// UnixRequest represents a request message sent between servers via Unix Domain Socket.
	// It is used for various inter-node operations such as remote joins, leaves, and fetches.
	UnixRequest struct {
		Type      adapter.MessageType    `json:"type,omitempty"`
		RequestId string                 `json:"requestId,omitempty"`
		Rooms     []socket.Room          `json:"rooms,omitempty"`
		Opts      *adapter.PacketOptions `json:"opts,omitempty"`
		Sid       socket.SocketId        `json:"sid,omitempty"`
		Room      socket.Room            `json:"room,omitempty"`
		Close     bool                   `json:"close,omitempty"`
		Uid       adapter.ServerId       `json:"uid,omitempty"`
		Data      []any                  `json:"data,omitempty"`
		Packet    *parser.Packet         `json:"packet,omitempty"`
	}

	// UnixResponse represents a response message sent between servers via Unix Domain Socket.
	// It contains the response data for various inter-node requests.
	UnixResponse struct {
		Type        adapter.MessageType       `json:"type,omitempty"`
		RequestId   string                    `json:"requestId,omitempty"`
		Rooms       []socket.Room             `json:"rooms,omitempty"`
		Sockets     []*adapter.SocketResponse `json:"sockets,omitempty"`
		Data        []any                     `json:"data,omitempty"`
		ClientCount uint64                    `json:"clientcount,omitempty"`
		Packet      []any                     `json:"packet,omitempty"`
	}

	// Parser defines the interface for encoding and decoding data for Unix Domain Socket communication.
	// Implementations must be thread-safe as they may be called from multiple goroutines.
	Parser interface {
		// Encode serializes the given value into a byte slice.
		Encode(any) ([]byte, error)

		// Decode deserializes the byte slice into the given value.
		Decode([]byte, any) error
	}
)

// MarshalJSON implements the json.Marshaler interface for UnixPacket.
// It serializes the UnixPacket as a JSON array in the format [Uid, Packet, Opts].
func (p *UnixPacket) MarshalJSON() ([]byte, error) {
	if p == nil {
		return json.Marshal(nil)
	}
	return json.Marshal([]any{p.Uid, p.Packet, p.Opts})
}

// UnmarshalJSON implements the json.Unmarshaler interface for UnixPacket.
// It deserializes a JSON array [Uid, Packet?, Opts?] back into the UnixPacket struct.
// The Uid field is required; Packet and Opts are optional.
func (p *UnixPacket) UnmarshalJSON(data []byte) error {
	if p == nil {
		return ErrNilUnixPacket
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("failed to unmarshal UnixPacket array: %w", err)
	}

	// Validate minimum required fields
	if len(arr) < 1 {
		return fmt.Errorf("UnixPacket array must contain at least 1 element (Uid), got %d", len(arr))
	}

	// Unmarshal Uid (required)
	if err := json.Unmarshal(arr[0], &p.Uid); err != nil {
		return fmt.Errorf("failed to unmarshal UnixPacket Uid: %w", err)
	}

	// Unmarshal Packet (optional)
	if len(arr) > 1 {
		var pkt *parser.Packet
		if err := json.Unmarshal(arr[1], &pkt); err != nil {
			return fmt.Errorf("failed to unmarshal UnixPacket Packet: %w", err)
		}
		p.Packet = pkt
	}

	// Unmarshal Opts (optional)
	if len(arr) > 2 {
		var o *adapter.PacketOptions
		if err := json.Unmarshal(arr[2], &o); err != nil {
			return fmt.Errorf("failed to unmarshal UnixPacket Opts: %w", err)
		}
		p.Opts = o
	}

	return nil
}
