// Package cache provides shared wire types and utilities for the cache-based
// Socket.IO cluster adapter.
package cache

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// ErrNilCachePacket is returned when attempting to unmarshal into a nil CachePacket.
var ErrNilCachePacket = errors.New("cannot unmarshal into nil CachePacket")

type (
	// CachePacket represents a Socket.IO packet broadcast across cluster nodes.
	// It carries the originating server UID, the packet itself, and routing options.
	CachePacket struct {
		_msgpack struct{} `json:"-" msgpack:",as_array"` //nolint:unused

		// Uid identifies the server that originated this packet.
		Uid adapter.ServerId `json:"-"`

		// Packet is the Socket.IO packet to broadcast.
		Packet *parser.Packet `json:"-"`

		// Opts contains the broadcast routing options (rooms, exclusions, flags).
		Opts *adapter.PacketOptions `json:"-"`
	}

	// CacheRequest is an inter-node request message.
	// It is used for remote joins, leaves, fetches, and other cluster operations.
	CacheRequest struct {
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

	// CacheResponse is an inter-node response message.
	CacheResponse struct {
		Type        adapter.MessageType       `json:"type,omitempty" msgpack:"type,omitempty"`
		RequestId   string                    `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Rooms       []socket.Room             `json:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Sockets     []*adapter.SocketResponse `json:"sockets,omitempty" msgpack:"sockets,omitempty"`
		Data        []any                     `json:"data,omitempty" msgpack:"data,omitempty"`
		ClientCount uint64                    `json:"clientcount,omitempty" msgpack:"clientcount,omitempty"`
		Packet      []any                     `json:"packet,omitempty" msgpack:"packet,omitempty"`
	}

	// Parser defines the interface for encoding/decoding inter-node messages.
	// Implementations must be safe for concurrent use.
	Parser interface {
		// Encode serializes v into a byte slice.
		Encode(v any) ([]byte, error)

		// Decode deserializes data into v.
		Decode(data []byte, v any) error
	}
)

// MarshalJSON serializes the CachePacket as a JSON array [Uid, Packet, Opts].
func (c *CachePacket) MarshalJSON() ([]byte, error) {
	if c == nil {
		return json.Marshal(nil)
	}
	return json.Marshal([]any{c.Uid, c.Packet, c.Opts})
}

// UnmarshalJSON deserializes a JSON array [Uid, Packet?, Opts?] into the CachePacket.
// Only Uid is required; Packet and Opts are optional.
func (c *CachePacket) UnmarshalJSON(data []byte) error {
	if c == nil {
		return ErrNilCachePacket
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("failed to unmarshal CachePacket array: %w", err)
	}

	if len(arr) < 1 {
		return fmt.Errorf("CachePacket array must have at least 1 element (Uid), got %d", len(arr))
	}

	if err := json.Unmarshal(arr[0], &c.Uid); err != nil {
		return fmt.Errorf("failed to unmarshal CachePacket Uid: %w", err)
	}

	if len(arr) > 1 {
		var p *parser.Packet
		if err := json.Unmarshal(arr[1], &p); err != nil {
			return fmt.Errorf("failed to unmarshal CachePacket Packet: %w", err)
		}
		c.Packet = p
	}

	if len(arr) > 2 {
		var o *adapter.PacketOptions
		if err := json.Unmarshal(arr[2], &o); err != nil {
			return fmt.Errorf("failed to unmarshal CachePacket Opts: %w", err)
		}
		c.Opts = o
	}

	return nil
}

// SubscriptionMode controls how pub/sub channels are allocated per namespace.
type SubscriptionMode string

const (
	// StaticSubscriptionMode uses two fixed channels per namespace.
	StaticSubscriptionMode SubscriptionMode = "static"

	// DynamicSubscriptionMode uses 2 + 1 channel per public room per namespace.
	// Private rooms (socket IDs) share the main channel.
	DynamicSubscriptionMode SubscriptionMode = "dynamic"

	// DynamicPrivateSubscriptionMode creates a dedicated channel for every room,
	// including private rooms (socket IDs).
	DynamicPrivateSubscriptionMode SubscriptionMode = "dynamic-private"

	// DefaultSubscriptionMode is the out-of-the-box subscription mode.
	DefaultSubscriptionMode = DynamicSubscriptionMode
)

// PrivateRoomIdLength is the byte length of a socket ID.
// Rooms of this length are considered private and are excluded from dynamic
// channel routing in DynamicSubscriptionMode.
const PrivateRoomIdLength = 20

// ShouldUseDynamicChannel reports whether a dedicated per-room channel should
// be used for the given room under the given subscription mode.
func ShouldUseDynamicChannel(mode SubscriptionMode, room socket.Room) bool {
	switch mode {
	case DynamicSubscriptionMode:
		return len(string(room)) != PrivateRoomIdLength
	case DynamicPrivateSubscriptionMode:
		return true
	default:
		return false
	}
}
