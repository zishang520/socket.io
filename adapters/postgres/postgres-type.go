// Package postgres provides PostgreSQL-based adapter types and interfaces for Socket.IO clustering.
// These types define the message structures used for inter-node communication via PostgreSQL LISTEN/NOTIFY.
package postgres

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// ErrNilPostgresPacket indicates an attempt to unmarshal into a nil PostgresPacket.
var ErrNilPostgresPacket = errors.New("cannot unmarshal into nil PostgresPacket")

type (
	// PostgresPacket represents a packet to be broadcast via PostgreSQL.
	// It contains the server UID, the Socket.IO packet, and broadcast options.
	PostgresPacket struct {
		// Uid identifies the source server that sent this packet.
		Uid adapter.ServerId `json:"-"`

		// Packet is the Socket.IO packet to be broadcast.
		Packet *parser.Packet `json:"-"`

		// Opts contains the broadcast options including target rooms and exclusions.
		Opts *adapter.PacketOptions `json:"-"`
	}

	// PostgresRequest represents a request message sent between servers via PostgreSQL.
	// It is used for various inter-node operations such as remote joins, leaves, and fetches.
	PostgresRequest struct {
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

	// PostgresResponse represents a response message sent between servers via PostgreSQL.
	// It contains the response data for various inter-node requests.
	PostgresResponse struct {
		Type        adapter.MessageType       `json:"type,omitempty"`
		RequestId   string                    `json:"requestId,omitempty"`
		Rooms       []socket.Room             `json:"rooms,omitempty"`
		Sockets     []*adapter.SocketResponse `json:"sockets,omitempty"`
		Data        []any                     `json:"data,omitempty"`
		ClientCount uint64                    `json:"clientcount,omitempty"`
		Packet      []any                     `json:"packet,omitempty"`
	}

	// Parser defines the interface for encoding and decoding data for PostgreSQL communication.
	// Implementations must be thread-safe as they may be called from multiple goroutines.
	Parser interface {
		// Encode serializes the given value into a byte slice.
		Encode(any) ([]byte, error)

		// Decode deserializes the byte slice into the given value.
		Decode([]byte, any) error
	}
)

// MarshalJSON implements the json.Marshaler interface for PostgresPacket.
// It serializes the PostgresPacket as a JSON array in the format [Uid, Packet, Opts].
func (p *PostgresPacket) MarshalJSON() ([]byte, error) {
	if p == nil {
		return json.Marshal(nil)
	}
	return json.Marshal([]any{p.Uid, p.Packet, p.Opts})
}

// UnmarshalJSON implements the json.Unmarshaler interface for PostgresPacket.
// It deserializes a JSON array [Uid, Packet?, Opts?] back into the PostgresPacket struct.
// The Uid field is required; Packet and Opts are optional.
func (p *PostgresPacket) UnmarshalJSON(data []byte) error {
	if p == nil {
		return ErrNilPostgresPacket
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("failed to unmarshal PostgresPacket array: %w", err)
	}

	// Validate minimum required fields
	if len(arr) < 1 {
		return fmt.Errorf("PostgresPacket array must contain at least 1 element (Uid), got %d", len(arr))
	}

	// Unmarshal Uid (required)
	if err := json.Unmarshal(arr[0], &p.Uid); err != nil {
		return fmt.Errorf("failed to unmarshal PostgresPacket Uid: %w", err)
	}

	// Unmarshal Packet (optional)
	if len(arr) > 1 {
		var pkt *parser.Packet
		if err := json.Unmarshal(arr[1], &pkt); err != nil {
			return fmt.Errorf("failed to unmarshal PostgresPacket Packet: %w", err)
		}
		p.Packet = pkt
	}

	// Unmarshal Opts (optional)
	if len(arr) > 2 {
		var o *adapter.PacketOptions
		if err := json.Unmarshal(arr[2], &o); err != nil {
			return fmt.Errorf("failed to unmarshal PostgresPacket Opts: %w", err)
		}
		p.Opts = o
	}

	return nil
}
