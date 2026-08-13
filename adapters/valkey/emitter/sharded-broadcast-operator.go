// Package emitter provides a sharded broadcast operator for emitting events via Valkey Sharded Pub/Sub.
package emitter

import (
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// ShardedBroadcastOperator broadcasts events via Valkey Sharded Pub/Sub (SPUBLISH).
type ShardedBroadcastOperator struct {
	valkeyClient     *valkey.ValkeyClient
	broadcastOptions *BroadcastOptions
	rooms            *types.Set[socket.Room]
	exceptRooms      *types.Set[socket.Room]
	flags            *socket.BroadcastFlags
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
	valkeyClient *valkey.ValkeyClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) *ShardedBroadcastOperator {
	b := MakeShardedBroadcastOperator()
	b.Construct(valkeyClient, broadcastOptions, rooms, exceptRooms, flags)
	return b
}

// Construct initializes the ShardedBroadcastOperator.
func (b *ShardedBroadcastOperator) Construct(
	valkeyClient *valkey.ValkeyClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) {
	b.valkeyClient = valkeyClient

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
	return NewShardedBroadcastOperator(b.valkeyClient, b.broadcastOptions, rooms, b.exceptRooms, b.flags)
}

func (b *ShardedBroadcastOperator) In(room ...socket.Room) BroadcastOperatorInterface {
	return b.To(room...)
}

func (b *ShardedBroadcastOperator) Except(room ...socket.Room) BroadcastOperatorInterface {
	exceptRooms := types.NewSet(b.exceptRooms.Keys()...)
	exceptRooms.Add(room...)
	return NewShardedBroadcastOperator(b.valkeyClient, b.broadcastOptions, b.rooms, exceptRooms, b.flags)
}

func (b *ShardedBroadcastOperator) Compress(compress bool) BroadcastOperatorInterface {
	flags := new(*b.flags)
	flags.Compress = new(compress)
	return NewShardedBroadcastOperator(b.valkeyClient, b.broadcastOptions, b.rooms, b.exceptRooms, flags)
}

func (b *ShardedBroadcastOperator) Volatile() BroadcastOperatorInterface {
	flags := new(*b.flags)
	flags.Volatile = true
	return NewShardedBroadcastOperator(b.valkeyClient, b.broadcastOptions, b.rooms, b.exceptRooms, flags)
}

// Emit emits an event to all targeted clients using SPUBLISH.
func (b *ShardedBroadcastOperator) Emit(ev string, args ...any) error {
	if reservedEvents.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
	}

	if b.broadcastOptions.Parser == nil {
		return errors.New("broadcastOptions.Parser is not set")
	}

	packet := &parser.Packet{
		Type: parser.EVENT,
		Nsp:  b.broadcastOptions.Nsp,
		Data: utils.EventPayload(ev, args),
	}

	opts := adapter.EncodeOptions(&socket.BroadcastOptions{
		Rooms:  b.rooms,
		Except: b.exceptRooms,
		Flags:  b.flags,
	})

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

	return b.valkeyClient.SPublish(b.valkeyClient.Context, channel, msg)
}

func (b *ShardedBroadcastOperator) computeChannel() string {
	if b.rooms != nil {
		rooms := b.rooms.Keys()
		if len(rooms) == 1 && valkey.ShouldUseDynamicChannel(b.broadcastOptions.SubscriptionMode, rooms[0]) {
			return b.broadcastOptions.BroadcastChannel + string(rooms[0]) + "#"
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
			Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
				Rooms:  b.rooms,
				Except: b.exceptRooms,
			}),
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
			Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
				Rooms:  b.rooms,
				Except: b.exceptRooms,
			}),
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
			Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
				Rooms:  b.rooms,
				Except: b.exceptRooms,
			}),
			Close: state,
		},
	}
	return b.publishMessage(message)
}

func (b *ShardedBroadcastOperator) publishMessage(message *ClusterMessage) error {
	msg, err := b.broadcastOptions.Parser.Encode(message)
	if err != nil {
		return err
	}

	emitterLog.Debug("publishing message of type %v to %s via SPUBLISH", message.Type, b.broadcastOptions.BroadcastChannel)

	return b.valkeyClient.SPublish(b.valkeyClient.Context, b.broadcastOptions.BroadcastChannel, msg)
}
