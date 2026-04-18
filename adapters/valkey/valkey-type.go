// Package valkey provides Valkey-based adapter types and interfaces for Socket.IO clustering.
// These types define the message structures used for inter-node communication via Valkey.
package valkey

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

// ErrNilValkeyPacket indicates an attempt to unmarshal into a nil ValkeyPacket.
var ErrNilValkeyPacket = errors.New("cannot unmarshal into nil ValkeyPacket")

type (
	// ValkeyPacket represents a packet to be broadcast via Valkey.
	// It contains the server UID, the Socket.IO packet, and broadcast options.
	ValkeyPacket struct {
		_msgpack struct{} `json:"-" msgpack:",as_array"` //nolint:unused

		// Uid identifies the source server that sent this packet.
		Uid adapter.ServerId `json:"-"`

		// Packet is the Socket.IO packet to be broadcast.
		Packet *parser.Packet `json:"-"`

		// Opts contains the broadcast options including target rooms and exclusions.
		Opts *adapter.PacketOptions `json:"-"`
	}

	// ValkeyRequest represents a request message sent between servers via Valkey.
	// It is used for various inter-node operations such as remote joins, leaves, and fetches.
	ValkeyRequest struct {
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

	// ValkeyResponse represents a response message sent between servers via Valkey.
	// It contains the response data for various inter-node requests.
	ValkeyResponse struct {
		Type        adapter.MessageType       `json:"type,omitempty" msgpack:"type,omitempty"`
		RequestId   string                    `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Rooms       []socket.Room             `json:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Sockets     []*adapter.SocketResponse `json:"sockets,omitempty" msgpack:"sockets,omitempty"`
		Data        []any                     `json:"data,omitempty" msgpack:"data,omitempty"`
		ClientCount uint64                    `json:"clientcount,omitempty" msgpack:"clientcount,omitempty"`
		Packet      []any                     `json:"packet,omitempty" msgpack:"packet,omitempty"`
	}

	// Parser defines the interface for encoding and decoding data for Valkey communication.
	// Implementations must be thread-safe as they may be called from multiple goroutines.
	Parser interface {
		// Encode serializes the given value into a byte slice.
		Encode(any) ([]byte, error)

		// Decode deserializes the byte slice into the given value.
		Decode([]byte, any) error
	}
)

// MarshalJSON implements the json.Marshaler interface for ValkeyPacket.
// It serializes the ValkeyPacket as a JSON array in the format [Uid, Packet, Opts].
func (r *ValkeyPacket) MarshalJSON() ([]byte, error) {
	if r == nil {
		return json.Marshal(nil)
	}
	return json.Marshal([]any{r.Uid, r.Packet, r.Opts})
}

// UnmarshalJSON implements the json.Unmarshaler interface for ValkeyPacket.
// It deserializes a JSON array [Uid, Packet?, Opts?] back into the ValkeyPacket struct.
// The Uid field is required; Packet and Opts are optional.
func (r *ValkeyPacket) UnmarshalJSON(data []byte) error {
	if r == nil {
		return ErrNilValkeyPacket
	}

	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return fmt.Errorf("failed to unmarshal ValkeyPacket array: %w", err)
	}

	if len(arr) < 1 {
		return fmt.Errorf("ValkeyPacket array must contain at least 1 element (Uid), got %d", len(arr))
	}

	if err := json.Unmarshal(arr[0], &r.Uid); err != nil {
		return fmt.Errorf("failed to unmarshal ValkeyPacket Uid: %w", err)
	}

	if len(arr) > 1 {
		var p *parser.Packet
		if err := json.Unmarshal(arr[1], &p); err != nil {
			return fmt.Errorf("failed to unmarshal ValkeyPacket Packet: %w", err)
		}
		r.Packet = p
	}

	if len(arr) > 2 {
		var o *adapter.PacketOptions
		if err := json.Unmarshal(arr[2], &o); err != nil {
			return fmt.Errorf("failed to unmarshal ValkeyPacket Opts: %w", err)
		}
		r.Opts = o
	}

	return nil
}

// SubscriptionMode determines how Valkey Pub/Sub channels are managed.
// This type is shared between the adapter and emitter packages.
type SubscriptionMode string

// Subscription mode constants for Valkey adapter.
const (
	// StaticSubscriptionMode uses 2 fixed channels per namespace.
	StaticSubscriptionMode SubscriptionMode = "static"

	// DynamicSubscriptionMode uses 2 + 1 channel per public room per namespace.
	DynamicSubscriptionMode SubscriptionMode = "dynamic"

	// DynamicPrivateSubscriptionMode creates separate channels for both public and private rooms.
	DynamicPrivateSubscriptionMode SubscriptionMode = "dynamic-private"

	// DefaultSubscriptionMode is the default subscription mode.
	DefaultSubscriptionMode = DynamicSubscriptionMode
)

// PrivateRoomIdLength is the length of a socket ID, used to determine if a room is private.
const PrivateRoomIdLength = 20

// ShouldUseDynamicChannel determines if a dynamic channel should be used for the given room.
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
