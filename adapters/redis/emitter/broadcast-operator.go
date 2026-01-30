// Package emitter provides broadcast capabilities for Socket.IO via Redis pub/sub.
package emitter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
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

// BroadcastOperator provides a fluent API for broadcasting events to Socket.IO clients via Redis.
// It supports room targeting, exclusions, and broadcast flags through method chaining.
type BroadcastOperator struct {
	redisClient      *redis.RedisClient      // Redis client for publishing messages
	broadcastOptions *BroadcastOptions       // Configuration for broadcasting
	rooms            *types.Set[socket.Room] // Target rooms for the broadcast
	exceptRooms      *types.Set[socket.Room] // Rooms to exclude from the broadcast
	flags            *socket.BroadcastFlags  // Broadcast flags (compress, volatile, etc.)
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
	client *redis.RedisClient,
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
	client *redis.RedisClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) {
	b.redisClient = client

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
func (b *BroadcastOperator) To(room ...socket.Room) *BroadcastOperator {
	rooms := types.NewSet(b.rooms.Keys()...)
	rooms.Add(room...)
	return NewBroadcastOperator(b.redisClient, b.broadcastOptions, rooms, b.exceptRooms, b.flags)
}

// In is an alias for To, targeting one or more rooms for the broadcast.
func (b *BroadcastOperator) In(room ...socket.Room) *BroadcastOperator {
	return b.To(room...)
}

// Except excludes one or more rooms from the broadcast.
// Returns a new BroadcastOperator with the rooms added to the exclusion list.
func (b *BroadcastOperator) Except(room ...socket.Room) *BroadcastOperator {
	exceptRooms := types.NewSet(b.exceptRooms.Keys()...)
	exceptRooms.Add(room...)
	return NewBroadcastOperator(b.redisClient, b.broadcastOptions, b.rooms, exceptRooms, b.flags)
}

// Compress sets the compress flag for the broadcast.
// When true, the message will be compressed before transmission.
func (b *BroadcastOperator) Compress(compress bool) *BroadcastOperator {
	flags := *b.flags
	flags.Compress = &compress
	return NewBroadcastOperator(b.redisClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Volatile sets the volatile flag for the broadcast.
// When set, the event data may be lost if the client is not ready to receive.
func (b *BroadcastOperator) Volatile() *BroadcastOperator {
	flags := *b.flags
	flags.Volatile = true
	return NewBroadcastOperator(b.redisClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Emit broadcasts an event with the given name and arguments to all targeted clients.
// Returns an error if the event name is reserved or if broadcasting fails.
func (b *BroadcastOperator) Emit(ev string, args ...any) error {
	if reservedEvents.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
	}

	if b.broadcastOptions.Parser == nil {
		return errors.New("broadcastOptions.Parser is not set")
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

	// Encode the packet using the configured parser
	msg, err := b.broadcastOptions.Parser.Encode(&Packet{
		Uid:    emitterUID,
		Packet: packet,
		Opts:   opts,
	})
	if err != nil {
		return err
	}

	// Determine the channel: use room-specific channel if targeting exactly one room
	channel := b.broadcastOptions.BroadcastChannel
	if b.rooms != nil && b.rooms.Len() == 1 {
		for _, room := range b.rooms.Keys() {
			channel += string(room) + "#"
			break
		}
	}

	emitterLog.Debug("publishing message to channel %s", channel)

	return b.redisClient.Client.Publish(b.redisClient.Context, channel, msg).Err()
}

// SocketsJoin makes all matching socket instances join the specified rooms.
// This sends a REMOTE_JOIN request to all Socket.IO servers in the cluster.
func (b *BroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
	request, err := json.Marshal(&Request{
		Type: redis.REMOTE_JOIN,
		Opts: &adapter.PacketOptions{
			Rooms:  b.rooms.Keys(),
			Except: b.exceptRooms.Keys(),
		},
		Rooms: rooms,
	})
	if err != nil {
		return err
	}

	return b.redisClient.Client.Publish(b.redisClient.Context, b.broadcastOptions.RequestChannel, request).Err()
}

// SocketsLeave makes all matching socket instances leave the specified rooms.
// This sends a REMOTE_LEAVE request to all Socket.IO servers in the cluster.
func (b *BroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
	request, err := json.Marshal(&Request{
		Type: redis.REMOTE_LEAVE,
		Opts: &adapter.PacketOptions{
			Rooms:  b.rooms.Keys(),
			Except: b.exceptRooms.Keys(),
		},
		Rooms: rooms,
	})
	if err != nil {
		return err
	}

	return b.redisClient.Client.Publish(b.redisClient.Context, b.broadcastOptions.RequestChannel, request).Err()
}

// DisconnectSockets disconnects all matching socket instances.
// If state is true, the underlying transport connection will be closed.
// This sends a REMOTE_DISCONNECT request to all Socket.IO servers in the cluster.
func (b *BroadcastOperator) DisconnectSockets(state bool) error {
	request, err := json.Marshal(&Request{
		Type: redis.REMOTE_DISCONNECT,
		Opts: &adapter.PacketOptions{
			Rooms:  b.rooms.Keys(),
			Except: b.exceptRooms.Keys(),
		},
		Close: state,
	})
	if err != nil {
		return err
	}

	return b.redisClient.Client.Publish(b.redisClient.Context, b.broadcastOptions.RequestChannel, request).Err()
}
