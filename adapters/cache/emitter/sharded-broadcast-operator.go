// Package emitter provides a sharded broadcast operator for emitting events
// via the cache sharded pub/sub layer (Redis 7+ / Valkey cluster mode).
package emitter

import (
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// ShardedBroadcastOperator publishes events using SPublish (sharded pub/sub).
// Messages use the ClusterMessage wire format, which is compatible with
// ShardedCacheAdapter on the receiving side.
type ShardedBroadcastOperator struct {
	cacheClient      cache.CacheClient
	broadcastOptions *BroadcastOptions
	rooms            *types.Set[socket.Room]
	exceptRooms      *types.Set[socket.Room]
	flags            *socket.BroadcastFlags
}

// MakeShardedBroadcastOperator returns a ShardedBroadcastOperator with empty sets.
func MakeShardedBroadcastOperator() *ShardedBroadcastOperator {
	return &ShardedBroadcastOperator{
		rooms:       types.NewSet[socket.Room](),
		exceptRooms: types.NewSet[socket.Room](),
		flags:       &socket.BroadcastFlags{},
	}
}

// NewShardedBroadcastOperator creates and initializes a ShardedBroadcastOperator.
func NewShardedBroadcastOperator(
	client cache.CacheClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) *ShardedBroadcastOperator {
	b := MakeShardedBroadcastOperator()
	b.Construct(client, broadcastOptions, rooms, exceptRooms, flags)
	return b
}

// Construct initializes the ShardedBroadcastOperator; nil parameters use safe defaults.
func (b *ShardedBroadcastOperator) Construct(
	client cache.CacheClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) {
	b.cacheClient = client
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

func (b *ShardedBroadcastOperator) To(room ...socket.Room) BroadcastOperatorInterface {
	rooms := types.NewSet(b.rooms.Keys()...)
	rooms.Add(room...)
	return NewShardedBroadcastOperator(b.cacheClient, b.broadcastOptions, rooms, b.exceptRooms, b.flags)
}

func (b *ShardedBroadcastOperator) In(room ...socket.Room) BroadcastOperatorInterface {
	return b.To(room...)
}

func (b *ShardedBroadcastOperator) Except(room ...socket.Room) BroadcastOperatorInterface {
	exceptRooms := types.NewSet(b.exceptRooms.Keys()...)
	exceptRooms.Add(room...)
	return NewShardedBroadcastOperator(b.cacheClient, b.broadcastOptions, b.rooms, exceptRooms, b.flags)
}

func (b *ShardedBroadcastOperator) Compress(compress bool) BroadcastOperatorInterface {
	flags := *b.flags
	flags.Compress = &compress
	return NewShardedBroadcastOperator(b.cacheClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

func (b *ShardedBroadcastOperator) Volatile() BroadcastOperatorInterface {
	flags := *b.flags
	flags.Volatile = true
	return NewShardedBroadcastOperator(b.cacheClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Emit broadcasts an event using SPUBLISH with the ClusterMessage wire format.
func (b *ShardedBroadcastOperator) Emit(ev string, args ...any) error {
	if reservedEvents.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
	}
	if b.broadcastOptions.Parser == nil {
		return errors.New("broadcastOptions.Parser is not set")
	}

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
	return b.cacheClient.SPublish(b.cacheClient.Context(), channel, msg)
}

func (b *ShardedBroadcastOperator) computeChannel() string {
	if b.rooms != nil && b.rooms.Len() == 1 {
		keys := b.rooms.Keys()
		if cache.ShouldUseDynamicChannel(b.broadcastOptions.SubscriptionMode, keys[0]) {
			return b.broadcastOptions.BroadcastChannel + string(keys[0]) + "#"
		}
	}
	return b.broadcastOptions.BroadcastChannel
}

// SocketsJoin makes matching sockets join the specified rooms via SPUBLISH.
func (b *ShardedBroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
	return b.publishMessage(&ClusterMessage{
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
	})
}

// SocketsLeave makes matching sockets leave the specified rooms via SPUBLISH.
func (b *ShardedBroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
	return b.publishMessage(&ClusterMessage{
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
	})
}

// DisconnectSockets disconnects matching sockets via SPUBLISH.
func (b *ShardedBroadcastOperator) DisconnectSockets(state bool) error {
	return b.publishMessage(&ClusterMessage{
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
	})
}

func (b *ShardedBroadcastOperator) publishMessage(message *ClusterMessage) error {
	msg, err := b.broadcastOptions.Parser.Encode(message)
	if err != nil {
		return err
	}
	emitterLog.Debug("publishing message of type %v to %s via SPUBLISH", message.Type, b.broadcastOptions.BroadcastChannel)
	return b.cacheClient.SPublish(b.cacheClient.Context(), b.broadcastOptions.BroadcastChannel, msg)
}
