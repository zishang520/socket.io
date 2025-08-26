// Package redis provides Redis-based adapter types and interfaces for Socket.IO clustering.
package redis

import (
	"encoding/json"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type (
	// RedisPacket represents a packet to be sent via Redis for broadcasting.
	RedisPacket struct {
		_msgpack struct{} `json:"-" msgpack:",as_array"`

		Uid    adapter.ServerId       `json:"-"`
		Packet *parser.Packet         `json:"-"`
		Opts   *adapter.PacketOptions `json:"-"`
	}

	// RedisRequest represents a request message sent between servers via Redis.
	RedisRequest struct {
		Type      adapter.MessageType    `json:"type,omitempty" msgpack:"type,omitempty"`
		RequestId string                 `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Rooms     []socket.Room          `json:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Opts      *adapter.PacketOptions `json:"opts,omitempty" msgpack:"opts,omitempty"`
		Sid       socket.SocketId        `json:"sid,omitempty" msgpack:"sid,omitempty"`
		Room      socket.Room            `json:"room,omitempty" msgpack:"room,omitempty"`
		Close     bool                   `json:"close,omitempty" msgpack:"close,omitempty"`
		Uid       adapter.ServerId       `json:"uid,omitempty" msgpack:"uid,omitempty"`
		Data      []any                  `json:"data,omitempty" msgpack:"data,omitempty"`
		Packet    *parser.Packet         `json:"packet,omitempty" msgpack:"packet,omitempty"`
	}

	// RedisResponse represents a response message sent between servers via Redis.
	RedisResponse struct {
		Type        adapter.MessageType       `json:"type,omitempty" msgpack:"type,omitempty"`
		RequestId   string                    `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Rooms       []socket.Room             `json:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Sockets     []*adapter.SocketResponse `json:"sockets,omitempty" msgpack:"sockets,omitempty"`
		Data        []any                     `json:"data,omitempty" msgpack:"data,omitempty"`
		ClientCount uint64                    `json:"clientcount,omitempty" msgpack:"clientcount,omitempty"`
		Packet      []any                     `json:"packet,omitempty" msgpack:"packet,omitempty"`
	}

	// Parser defines the interface for encoding and decoding data for Redis communication.
	Parser interface {
		Encode(any) ([]byte, error)
		Decode([]byte, any) error
	}
)

// MarshalJSON implements the json.Marshaler interface for RedisPacket.
// It serializes the RedisPacket as a JSON array containing [Uid, Packet, Opts].
func (r *RedisPacket) MarshalJSON() ([]byte, error) {
	if r == nil {
		return json.Marshal(nil)
	}
	return json.Marshal([]any{r.Uid, r.Packet, r.Opts})
}

// UnmarshalJSON implements the json.Unmarshaler interface for RedisPacket.
// It deserializes a JSON array back into the RedisPacket struct fields.
func (r *RedisPacket) UnmarshalJSON(data []byte) error {
	if r == nil {
		return fmt.Errorf("cannot unmarshal into nil RedisPacket")
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("failed to unmarshal RedisPacket array: %w", err)
	}

	// Validate minimum required fields
	if len(arr) < 1 {
		return fmt.Errorf("RedisPacket array must contain at least 1 element (Uid), got %d", len(arr))
	}

	// Unmarshal Uid (required)
	if err := json.Unmarshal(arr[0], &r.Uid); err != nil {
		return fmt.Errorf("failed to unmarshal RedisPacket Uid: %w", err)
	}

	// Unmarshal Packet (optional)
	if len(arr) > 1 {
		var p *parser.Packet
		if err := json.Unmarshal(arr[1], &p); err != nil {
			return fmt.Errorf("failed to unmarshal RedisPacket Packet: %w", err)
		}
		r.Packet = p
	}

	// Unmarshal Opts (optional)
	if len(arr) > 2 {
		var o *adapter.PacketOptions
		if err := json.Unmarshal(arr[2], &o); err != nil {
			return fmt.Errorf("failed to unmarshal RedisPacket Opts: %w", err)
		}
		r.Opts = o
	}

	return nil
}
