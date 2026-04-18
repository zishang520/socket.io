// Package mongo provides MongoDB-based adapter types and interfaces for Socket.IO clustering.
// These types define the message structures used for inter-node communication via MongoDB Change Streams.
package mongo

import (
	"errors"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// ErrNilMongoPacket indicates an attempt to unmarshal into a nil MongoPacket.
var ErrNilMongoPacket = errors.New("cannot unmarshal into nil MongoPacket")

type (
	// AdapterEvent represents a document stored in MongoDB for inter-node communication.
	// This structure matches the Node.js @socket.io/mongo-adapter document format.
	AdapterEvent struct {
		ID        bson.ObjectID       `bson:"_id,omitempty"`
		Uid       adapter.ServerId    `bson:"uid,omitempty"`
		Nsp       string              `bson:"nsp,omitempty"`
		Type      adapter.MessageType `bson:"type,omitempty"`
		Data      bson.RawValue       `bson:"data,omitempty"`
		CreatedAt bson.DateTime       `bson:"createdAt,omitempty"`
	}
)
