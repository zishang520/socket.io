// Package valkey provides Valkey-based adapter types and interfaces for Socket.IO clustering.
// These types define the message structures used for inter-node communication via Valkey.
package valkey

import (
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

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
		Type      RequestType            `json:"type" msgpack:"type"`
		RequestId string                 `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Rooms     []socket.Room          `json:"rooms,omitzero" msgpack:"rooms,omitempty"`
		Opts      *adapter.PacketOptions `json:"opts,omitempty" msgpack:"opts,omitempty"`
		Close     *bool                  `json:"close,omitempty" msgpack:"close,omitempty"`
		Uid       adapter.ServerId       `json:"uid,omitempty" msgpack:"uid,omitempty"`
		Data      []any                  `json:"data,omitzero" msgpack:"data,omitempty"`
		Packet    *parser.Packet         `json:"packet,omitempty" msgpack:"packet,omitempty"`
	}

	// ValkeyResponse represents a response message sent between servers via Valkey.
	// It contains the response data for various inter-node requests.
	ValkeyResponse struct {
		Type        RequestType   `json:"type,omitempty" msgpack:"type,omitempty"`
		RequestId   string        `json:"requestId" msgpack:"requestId"`
		Rooms       []socket.Room `json:"rooms,omitzero" msgpack:"rooms,omitempty"`
		Sockets     any           `json:"sockets,omitempty" msgpack:"sockets,omitempty"`
		Data        any           `json:"data" msgpack:"data"`
		ClientCount *uint64       `json:"clientCount,omitzero" msgpack:"clientCount,omitempty"`
		Packet      any           `json:"packet" msgpack:"packet"`
	}

	// RawClusterMessage is the flat field-value shape stored in Valkey Streams.
	RawClusterMessage map[string]string

	// Encoder defines the outbound serialization contract used by Valkey emitters.
	// Implementations must be thread-safe as they may be called from multiple goroutines.
	Encoder interface {
		// Encode serializes the given value into a byte slice.
		Encode(any) ([]byte, error)
	}

	// Parser defines the encoding and decoding contract used by Valkey adapters.
	// Implementations must be thread-safe as they may be called from multiple goroutines.
	Parser interface {
		Encoder

		// Decode deserializes the byte slice into the given value.
		Decode([]byte, any) error
	}
)

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

// PrivateRoomIdLength is the length of a Node.js Socket.IO socket ID.
const PrivateRoomIdLength = 20
