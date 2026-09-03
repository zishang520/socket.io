package emitter

import (
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// ShardedBroadcastOperator publishes cluster messages with Valkey sharded Pub/Sub.
type ShardedBroadcastOperator struct {
	valkeyClient     *valkey.ValkeyClient
	broadcastOptions *BroadcastOptions
	rooms            *types.Set[socket.Room]
	exceptRooms      *types.Set[socket.Room]
	flags            *socket.BroadcastFlags
}

func MakeShardedBroadcastOperator() *ShardedBroadcastOperator {
	return &ShardedBroadcastOperator{
		rooms:       types.NewSet[socket.Room](),
		exceptRooms: types.NewSet[socket.Room](),
		flags:       new(socket.BroadcastFlags),
	}
}

func NewShardedBroadcastOperator(
	client *valkey.ValkeyClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) *ShardedBroadcastOperator {
	b := MakeShardedBroadcastOperator()
	b.Construct(client, broadcastOptions, rooms, exceptRooms, flags)
	return b
}

func (b *ShardedBroadcastOperator) Construct(
	client *valkey.ValkeyClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) {
	if broadcastOptions == nil {
		broadcastOptions = new(BroadcastOptions)
	}
	b.valkeyClient = client
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

func (b *ShardedBroadcastOperator) Emit(ev string, args ...any) error {
	if socket.SOCKET_RESERVED_EVENTS.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
	}

	return b.publishMessage(&adapter.ClusterMessage{
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: &parser.Packet{
				Type: parser.EVENT,
				Nsp:  b.broadcastOptions.Nsp,
				Data: utils.EventPayload(ev, args),
			},
			Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
				Rooms:  b.rooms,
				Except: b.exceptRooms,
				Flags:  b.flags,
			}),
		},
	})
}

func (b *ShardedBroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
	return b.publishMessage(&adapter.ClusterMessage{
		Type: adapter.SOCKETS_JOIN,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
				Rooms:  b.rooms,
				Except: b.exceptRooms,
			}),
			Rooms: rooms,
		},
	})
}

func (b *ShardedBroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
	return b.publishMessage(&adapter.ClusterMessage{
		Type: adapter.SOCKETS_LEAVE,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
				Rooms:  b.rooms,
				Except: b.exceptRooms,
			}),
			Rooms: rooms,
		},
	})
}

func (b *ShardedBroadcastOperator) DisconnectSockets(close bool) error {
	return b.publishMessage(&adapter.ClusterMessage{
		Type: adapter.DISCONNECT_SOCKETS,
		Data: &adapter.DisconnectSocketsMessage{
			Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
				Rooms:  b.rooms,
				Except: b.exceptRooms,
			}),
			Close: close,
		},
	})
}

func (b *ShardedBroadcastOperator) ServerSideEmit(args ...any) error {
	if len(args) > 0 {
		if _, withAck := args[len(args)-1].(socket.Ack); withAck {
			return errAcknowledgementsNotSupported
		}
	}
	return b.publishMessage(&adapter.ClusterMessage{
		Type: adapter.SERVER_SIDE_EMIT,
		Data: &adapter.ServerSideEmitMessage{Packet: args},
	})
}

func (b *ShardedBroadcastOperator) publishMessage(message *adapter.ClusterMessage) error {
	message.Uid = adapter.EMITTER_UID
	message.Nsp = b.broadcastOptions.Nsp
	channel := b.broadcastOptions.BroadcastChannel

	if message.Type == adapter.BROADCAST {
		data := message.Data.(*adapter.BroadcastMessage)
		if data.RequestId == nil && len(data.Opts.Rooms) == 1 &&
			valkey.ShouldUseDynamicChannel(b.broadcastOptions.SubscriptionMode, data.Opts.Rooms[0]) {
			channel += string(data.Opts.Rooms[0]) + "#"
		}
	}

	payload, err := adapter.EncodeClusterMessage(message)
	if err != nil {
		return err
	}
	emitterLog.Debug("publishing message to channel %s via SPUBLISH", channel)
	return b.valkeyClient.SPublish(b.valkeyClient.Context(), channel, payload)
}
