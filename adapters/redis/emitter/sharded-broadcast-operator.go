// Package emitter provides a sharded broadcast operator for emitting events via Redis Sharded Pub/Sub.
package emitter

import (
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// ShardedBroadcastOperator allows targeting, excluding, and flagging rooms for event emission via Redis Sharded Pub/Sub.
// This operator uses SPUBLISH instead of PUBLISH, which is required for Redis Cluster mode.
// It encodes messages using the ClusterMessage format, which is compatible with ShardedRedisAdapter.
type ShardedBroadcastOperator struct {
	redisClient      *redis.RedisClient      // Redis client for publishing messages.
	broadcastOptions *BroadcastOptions       // Options for broadcasting.
	rooms            *types.Set[socket.Room] // Targeted rooms.
	exceptRooms      *types.Set[socket.Room] // Rooms to exclude.
	flags            *socket.BroadcastFlags  // Broadcast flags (e.g., compress, volatile).
}

// MakeShardedBroadcastOperator creates a new ShardedBroadcastOperator with default values.
func MakeShardedBroadcastOperator() *ShardedBroadcastOperator {
	return &ShardedBroadcastOperator{
		rooms:       types.NewSet[socket.Room](),
		exceptRooms: types.NewSet[socket.Room](),
		flags:       &socket.BroadcastFlags{},
	}
}

// NewShardedBroadcastOperator creates and initializes a new ShardedBroadcastOperator.
func NewShardedBroadcastOperator(
	redisClient *redis.RedisClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) *ShardedBroadcastOperator {
	b := MakeShardedBroadcastOperator()
	b.Construct(redisClient, broadcastOptions, rooms, exceptRooms, flags)
	return b
}

// Construct initializes the ShardedBroadcastOperator with the given parameters.
func (b *ShardedBroadcastOperator) Construct(
	redisClient *redis.RedisClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) {
	b.redisClient = redisClient

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

// To targets one or more rooms for event emission.
func (b *ShardedBroadcastOperator) To(room ...socket.Room) BroadcastOperatorInterface {
	rooms := types.NewSet(b.rooms.Keys()...)
	rooms.Add(room...)
	return NewShardedBroadcastOperator(b.redisClient, b.broadcastOptions, rooms, b.exceptRooms, b.flags)
}

// In is an alias for To, targeting one or more rooms for event emission.
func (b *ShardedBroadcastOperator) In(room ...socket.Room) BroadcastOperatorInterface {
	return b.To(room...)
}

// Except excludes one or more rooms from event emission.
func (b *ShardedBroadcastOperator) Except(room ...socket.Room) BroadcastOperatorInterface {
	exceptRooms := types.NewSet(b.exceptRooms.Keys()...)
	exceptRooms.Add(room...)
	return NewShardedBroadcastOperator(b.redisClient, b.broadcastOptions, b.rooms, exceptRooms, b.flags)
}

// Compress sets the compress flag for the broadcast.
func (b *ShardedBroadcastOperator) Compress(compress bool) BroadcastOperatorInterface {
	flags := *b.flags
	flags.Compress = &compress
	return NewShardedBroadcastOperator(b.redisClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Volatile sets the volatile flag, allowing event data to be lost if the client is not ready.
func (b *ShardedBroadcastOperator) Volatile() BroadcastOperatorInterface {
	flags := *b.flags
	flags.Volatile = true
	return NewShardedBroadcastOperator(b.redisClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Emit emits an event to all targeted clients using SPUBLISH (for Redis Cluster).
// This method uses the ClusterMessage format, which is compatible with ShardedRedisAdapter.
func (b *ShardedBroadcastOperator) Emit(ev string, args ...any) error {
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

	// Use ClusterMessage format for sharded mode (compatible with ShardedRedisAdapter)
	message := &ClusterMessage{
		Uid:  emitterUID,
		Nsp:  b.broadcastOptions.Nsp,
		Type: adapter.BROADCAST,
		Data: &BroadcastMessage{
			Opts:   opts,
			Packet: packet,
		},
	}

	channel := b.computeChannel()

	msg, err := b.broadcastOptions.Parser.Encode(message)
	if err != nil {
		return err
	}

	emitterLog.Debug("publishing message to channel %s via SPUBLISH", channel)

	// Use SPublish for Redis Cluster
	return b.redisClient.Client.SPublish(b.redisClient.Context, channel, msg).Err()
}

// computeChannel computes the channel to publish to.
func (b *ShardedBroadcastOperator) computeChannel() string {
	// In dynamic subscription mode, if there's only one room, use a room-specific channel
	if b.rooms != nil && b.rooms.Len() == 1 {
		for _, room := range b.rooms.Keys() {
			if redis.ShouldUseDynamicChannel(b.broadcastOptions.SubscriptionMode, room) {
				return b.broadcastOptions.BroadcastChannel + string(room) + "#"
			}
			break
		}
	}
	return b.broadcastOptions.BroadcastChannel
}

// SocketsJoin makes the matching socket instances join the specified rooms.
func (b *ShardedBroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
	message := &ClusterMessage{
		Uid:  emitterUID,
		Nsp:  b.broadcastOptions.Nsp,
		Type: adapter.SOCKETS_JOIN,
		Data: &SocketsJoinLeaveMessage{
			Opts: &adapter.PacketOptions{
				Rooms:  b.rooms.Keys(),
				Except: b.exceptRooms.Keys(),
			},
			Rooms: rooms,
		},
	}

	return b.publishMessage(message)
}

// SocketsLeave makes the matching socket instances leave the specified rooms.
func (b *ShardedBroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
	message := &ClusterMessage{
		Uid:  emitterUID,
		Nsp:  b.broadcastOptions.Nsp,
		Type: adapter.SOCKETS_LEAVE,
		Data: &SocketsJoinLeaveMessage{
			Opts: &adapter.PacketOptions{
				Rooms:  b.rooms.Keys(),
				Except: b.exceptRooms.Keys(),
			},
			Rooms: rooms,
		},
	}

	return b.publishMessage(message)
}

// DisconnectSockets disconnects the matching socket instances.
func (b *ShardedBroadcastOperator) DisconnectSockets(state bool) error {
	message := &ClusterMessage{
		Uid:  emitterUID,
		Nsp:  b.broadcastOptions.Nsp,
		Type: adapter.DISCONNECT_SOCKETS,
		Data: &DisconnectSocketsMessage{
			Opts: &adapter.PacketOptions{
				Rooms:  b.rooms.Keys(),
				Except: b.exceptRooms.Keys(),
			},
			Close: state,
		},
	}

	return b.publishMessage(message)
}

// publishMessage publishes a ClusterMessage using SPUBLISH.
func (b *ShardedBroadcastOperator) publishMessage(message *ClusterMessage) error {
	msg, err := b.broadcastOptions.Parser.Encode(message)
	if err != nil {
		return err
	}

	emitterLog.Debug("publishing message of type %v to %s via SPUBLISH", message.Type, b.broadcastOptions.BroadcastChannel)

	return b.redisClient.Client.SPublish(b.redisClient.Context, b.broadcastOptions.BroadcastChannel, msg).Err()
}
