// Package mongo provides MongoDB-based adapter types and interfaces for Socket.IO clustering.
// These types define the message structures used for inter-node communication via MongoDB Change Streams.
package mongo

import (
	"errors"
	"sync"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/utils"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrNilMongoPacket indicates an attempt to unmarshal into a nil MongoPacket.
var ErrNilMongoPacket = errors.New("cannot unmarshal into nil MongoPacket")

type (
	// EventType is the type of an event exchanged between MongoDB adapter nodes.
	EventType = adapter.MessageType

	// AdapterEvent represents a document stored in MongoDB for inter-node communication.
	// This structure matches the Node.js @socket.io/mongo-adapter document format.
	AdapterEvent struct {
		ID        bson.ObjectID    `bson:"_id,omitempty"`
		Uid       adapter.ServerId `bson:"uid,omitempty"`
		Nsp       string           `bson:"nsp,omitempty"`
		Type      EventType        `bson:"type,omitempty"`
		Data      bson.RawValue    `bson:"data,omitempty"`
		CreatedAt bson.DateTime    `bson:"createdAt,omitempty"`
	}

	// Request tracks a pending inter-node request.
	Request struct {
		sync.Mutex

		Type      EventType
		Resolve   func([]any)
		Timeout   *utils.Timer
		Expected  int64
		Current   int64
		Responses []any
	}

	// AckRequest tracks callbacks for a broadcast with acknowledgements.
	AckRequest struct {
		Type                EventType
		ClientCountCallback func(uint64)
		Ack                 socket.Ack
	}

	// SessionDocument is the data stored in a SESSION event.
	SessionDocument struct {
		Sid   socket.SocketId         `bson:"sid,omitempty"`
		Pid   socket.PrivateSessionId `bson:"pid"`
		Rooms []socket.Room           `bson:"rooms"`
		Data  any                     `bson:"data"`
	}

	// SessionTombstone prevents a session in a capped collection from being
	// restored more than once.
	SessionTombstone struct {
		Pid       socket.PrivateSessionId `bson:"pid"`
		Tombstone bool                    `bson:"tombstone"`
	}

	// PacketOptions contains the rooms, exclusions and flags encoded in an event.
	PacketOptions = adapter.PacketOptions

	// PacketData contains a packet and its optional request metadata.
	PacketData[T any] struct {
		Packet    T              `bson:"packet"`
		Opts      *PacketOptions `bson:"opts,omitempty"`
		RequestId *string        `bson:"requestId,omitempty"`
	}

	// SocketResponse contains the details returned by a fetch sockets request.
	SocketResponse struct {
		Id        socket.SocketId   `bson:"id"`
		Handshake *socket.Handshake `bson:"handshake"`
		Rooms     []socket.Room     `bson:"rooms"`
		Data      any               `bson:"data"`
	}

	// EventData contains the optional fields shared by MongoDB adapter events.
	EventData struct {
		Opts        *PacketOptions    `bson:"opts,omitempty"`
		RequestId   string            `bson:"requestId,omitempty"`
		Rooms       *[]socket.Room    `bson:"rooms,omitempty"`
		Close       *bool             `bson:"close,omitempty"`
		Sockets     *[]SocketResponse `bson:"sockets,omitempty"`
		ClientCount *uint64           `bson:"clientCount,omitempty"`
	}

	// SocketPacket is the Socket.IO packet representation stored in MongoDB.
	SocketPacket struct {
		Type        parser.PacketType `bson:"type"`
		Nsp         string            `bson:"nsp,omitempty"`
		Data        any               `bson:"data,omitempty"`
		Id          *uint64           `bson:"id,omitempty"`
		Attachments *uint64           `bson:"attachments,omitempty"`
	}
)
