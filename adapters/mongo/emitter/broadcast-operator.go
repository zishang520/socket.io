// Package emitter provides broadcast capabilities for Socket.IO via MongoDB.
package emitter

import (
	"fmt"
	"time"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/mongo/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// reservedEvents contains event names that are reserved by Socket.IO and cannot be emitted.
var reservedEvents = types.NewSet(
	"connect",
	"connect_error",
	"disconnect",
	"disconnecting",
	"newListener",
	"removeListener",
)

// BroadcastOperator provides a fluent API for broadcasting events to Socket.IO clients via MongoDB.
// It supports room targeting, exclusions, and broadcast flags through method chaining.
type BroadcastOperator struct {
	mongoClient      *mongo.MongoClient       // MongoDB client for publishing messages
	broadcastOptions *BroadcastOptions        // Configuration for broadcasting
	rooms            *types.Set[socket.Room]  // Target rooms for the broadcast
	exceptRooms      *types.Set[socket.Room]  // Rooms to exclude from the broadcast
	flags            *socket.BroadcastFlags   // Broadcast flags (compress, volatile, etc.)
}

// MakeBroadcastOperator creates a new BroadcastOperator with empty room sets and default flags.
func MakeBroadcastOperator() *BroadcastOperator {
	return &BroadcastOperator{
		rooms:       types.NewSet[socket.Room](),
		exceptRooms: types.NewSet[socket.Room](),
		flags:       &socket.BroadcastFlags{},
	}
}

// NewBroadcastOperator creates and initializes a new BroadcastOperator with the given configuration.
// Nil parameters are replaced with safe defaults.
func NewBroadcastOperator(
	client *mongo.MongoClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) *BroadcastOperator {
	b := MakeBroadcastOperator()
	b.Construct(client, broadcastOptions, rooms, exceptRooms, flags)
	return b
}

// Construct initializes the BroadcastOperator with the given parameters.
// This method is called by NewBroadcastOperator and handles nil safety.
func (b *BroadcastOperator) Construct(
	client *mongo.MongoClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) {
	b.mongoClient = client

	if broadcastOptions == nil {
		broadcastOptions = &BroadcastOptions{}
	}
	b.broadcastOptions = broadcastOptions

	if rooms != nil {
		b.rooms = rooms
	}
	if exceptRooms != nil {
		b.exceptRooms = exceptRooms
	}
	if flags != nil {
		b.flags = flags
	}
}

// To targets one or more rooms for the broadcast.
// Returns a new BroadcastOperator with the additional rooms included.
func (b *BroadcastOperator) To(room ...socket.Room) BroadcastOperatorInterface {
	rooms := types.NewSet(b.rooms.Keys()...)
	rooms.Add(room...)
	return NewBroadcastOperator(b.mongoClient, b.broadcastOptions, rooms, b.exceptRooms, b.flags)
}

// In is an alias for To, targeting one or more rooms for the broadcast.
func (b *BroadcastOperator) In(room ...socket.Room) BroadcastOperatorInterface {
	return b.To(room...)
}

// Except excludes one or more rooms from the broadcast.
// Returns a new BroadcastOperator with the rooms added to the exclusion list.
func (b *BroadcastOperator) Except(room ...socket.Room) BroadcastOperatorInterface {
	exceptRooms := types.NewSet(b.exceptRooms.Keys()...)
	exceptRooms.Add(room...)
	return NewBroadcastOperator(b.mongoClient, b.broadcastOptions, b.rooms, exceptRooms, b.flags)
}

// Compress sets the compress flag for the broadcast.
// When true, the message will be compressed before transmission.
func (b *BroadcastOperator) Compress(compress bool) BroadcastOperatorInterface {
	flags := *b.flags
	flags.Compress = &compress
	return NewBroadcastOperator(b.mongoClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Volatile sets the volatile flag for the broadcast.
// When set, the event data may be lost if the client is not ready to receive.
func (b *BroadcastOperator) Volatile() BroadcastOperatorInterface {
	flags := *b.flags
	flags.Volatile = true
	return NewBroadcastOperator(b.mongoClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Emit broadcasts an event with the given name and arguments to all targeted clients.
// Returns an error if the event name is reserved or if broadcasting fails.
//
// The message is sent by inserting a document into the MongoDB collection,
// matching the Node.js @socket.io/mongo-emitter wire protocol.
func (b *BroadcastOperator) Emit(ev string, args ...any) error {
	if reservedEvents.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
	}

	// Construct the packet data
	data := append([]any{ev}, args...)

	packet := &parser.Packet{
		Type: parser.EVENT,
		Nsp:  b.broadcastOptions.Nsp,
		Data: data,
	}

	opts := &adapter.PacketOptions{
		Rooms:  b.rooms.Keys(),
		Except: b.exceptRooms.Keys(),
		Flags:  b.flags,
	}

	// Build document matching Node.js format:
	// {uid: "emitter", nsp: "/", type: BROADCAST, data: {packet, opts}}
	return b.publish(&adapter.ClusterMessage{
		Uid:  emitterUID,
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: packet,
			Opts:   opts,
		},
	})
}

// publish inserts a ClusterMessage document into the MongoDB collection.
// This matches the Node.js emitter's _publish() method behavior exactly.
func (b *BroadcastOperator) publish(message *adapter.ClusterMessage) error {
	doc := bson.D{
		{Key: "uid", Value: string(emitterUID)},
		{Key: "nsp", Value: b.broadcastOptions.Nsp},
		{Key: "type", Value: message.Type},
	}

	if message.Data != nil {
		doc = append(doc, bson.E{Key: "data", Value: message.Data})
	}

	if b.broadcastOptions.AddCreatedAtField {
		doc = append(doc, bson.E{Key: "createdAt", Value: time.Now()})
	}

	emitterLog.Debug("publishing message to collection")

	_, err := b.mongoClient.Collection.InsertOne(b.mongoClient.Context, doc)
	return err
}

// SocketsJoin makes all matching socket instances join the specified rooms.
// This sends a SOCKETS_JOIN document to all Socket.IO servers in the cluster.
func (b *BroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
	return b.publish(&adapter.ClusterMessage{
		Uid:  emitterUID,
		Type: adapter.SOCKETS_JOIN,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts: &adapter.PacketOptions{
				Rooms:  b.rooms.Keys(),
				Except: b.exceptRooms.Keys(),
			},
			Rooms: rooms,
		},
	})
}

// SocketsLeave makes all matching socket instances leave the specified rooms.
// This sends a SOCKETS_LEAVE document to all Socket.IO servers in the cluster.
func (b *BroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
	return b.publish(&adapter.ClusterMessage{
		Uid:  emitterUID,
		Type: adapter.SOCKETS_LEAVE,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts: &adapter.PacketOptions{
				Rooms:  b.rooms.Keys(),
				Except: b.exceptRooms.Keys(),
			},
			Rooms: rooms,
		},
	})
}

// DisconnectSockets disconnects all matching socket instances.
// If state is true, the underlying transport connection will be closed.
// This sends a DISCONNECT_SOCKETS document to all Socket.IO servers in the cluster.
func (b *BroadcastOperator) DisconnectSockets(state bool) error {
	return b.publish(&adapter.ClusterMessage{
		Uid:  emitterUID,
		Type: adapter.DISCONNECT_SOCKETS,
		Data: &adapter.DisconnectSocketsMessage{
			Opts: &adapter.PacketOptions{
				Rooms:  b.rooms.Keys(),
				Except: b.exceptRooms.Keys(),
			},
			Close: state,
		},
	})
}

// ServerSideEmit sends a message to all Socket.IO servers in the cluster.
// The first argument should be the event name, followed by any data arguments.
// Note: Acknowledgements are not supported when using the emitter.
func (b *BroadcastOperator) ServerSideEmit(args ...any) error {
	if len(args) > 0 {
		if _, withAck := args[len(args)-1].(socket.Ack); withAck {
			return fmt.Errorf("acknowledgements are not supported when using emitter")
		}
	}

	return b.publish(&adapter.ClusterMessage{
		Uid:  emitterUID,
		Type: adapter.SERVER_SIDE_EMIT,
		Data: &adapter.ServerSideEmitMessage{
			Packet: args,
		},
	})
}
