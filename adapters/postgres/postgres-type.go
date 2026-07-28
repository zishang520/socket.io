// Package postgres provides PostgreSQL-based adapter types and interfaces for Socket.IO clustering.
// These types define the message structures used for inter-node communication via PostgreSQL LISTEN/NOTIFY.
package postgres

import (
	"encoding/json"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/servers/socket/v3"
)

type (
	// NotificationMessage represents a message received via PostgreSQL LISTEN/NOTIFY.
	// It can either contain the full payload or a reference to an attachment.
	NotificationMessage struct {
		Uid          adapter.ServerId    `json:"uid,omitempty" msgpack:"uid,omitempty"`
		Nsp          string              `json:"nsp,omitempty" msgpack:"nsp,omitempty"`
		Type         adapter.MessageType `json:"type,omitempty" msgpack:"type,omitempty"`
		Data         json.RawMessage     `json:"data,omitempty" msgpack:"-"`
		AttachmentId string              `json:"attachmentId,omitempty" msgpack:"attachmentId,omitempty"`
	}

	// BroadcastFlags is the representation exchanged with Node.js adapters.
	// Timeout is encoded in milliseconds, while Go uses time.Duration internally.
	BroadcastFlags struct {
		Compress             *bool  `json:"compress,omitempty" msgpack:"compress,omitempty"`
		Volatile             bool   `json:"volatile,omitempty" msgpack:"volatile,omitempty"`
		Local                bool   `json:"local,omitempty" msgpack:"local,omitempty"`
		Broadcast            bool   `json:"broadcast,omitempty" msgpack:"broadcast,omitempty"`
		Binary               bool   `json:"binary,omitempty" msgpack:"binary,omitempty"`
		Timeout              *int64 `json:"timeout,omitempty" msgpack:"timeout,omitempty"`
		ExpectSingleResponse bool   `json:"expectSingleResponse,omitempty" msgpack:"expectSingleResponse,omitempty"`
	}

	// PacketOptions is the Node.js wire representation of broadcast options.
	PacketOptions struct {
		Rooms  []socket.Room   `json:"rooms" msgpack:"rooms"`
		Except []socket.Room   `json:"except" msgpack:"except"`
		Flags  *BroadcastFlags `json:"flags,omitempty" msgpack:"flags,omitempty"`
	}

	// PacketData contains a packet and its optional request metadata.
	PacketData[T any] struct {
		Packet    T              `json:"packet" msgpack:"packet"`
		Opts      *PacketOptions `json:"opts,omitempty" msgpack:"opts,omitempty"`
		RequestId *string        `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
	}

	// EventData contains the fields used by non-packet cluster messages.
	EventData struct {
		Opts        *PacketOptions    `json:"opts,omitempty" msgpack:"opts,omitempty"`
		RequestId   string            `json:"requestId,omitempty" msgpack:"requestId,omitempty"`
		Rooms       *[]socket.Room    `json:"rooms,omitempty" msgpack:"rooms,omitempty"`
		Close       *bool             `json:"close,omitempty" msgpack:"close,omitempty"`
		Sockets     *[]SocketResponse `json:"sockets,omitempty" msgpack:"sockets,omitempty"`
		ClientCount *uint64           `json:"clientCount,omitempty" msgpack:"clientCount,omitempty"`
	}

	// SocketResponse is the socket detail representation used on the wire.
	SocketResponse struct {
		Id        socket.SocketId   `json:"id" msgpack:"id"`
		Handshake *socket.Handshake `json:"handshake" msgpack:"handshake"`
		Rooms     []socket.Room     `json:"rooms" msgpack:"rooms"`
		Data      any               `json:"data" msgpack:"data"`
	}
)
