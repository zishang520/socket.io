package emitter

import (
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/redis/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// RedisStreamsBroadcastOperator publishes cluster messages with Redis Streams.
type RedisStreamsBroadcastOperator struct {
	redisClient *redis.RedisClient
	nsp         string
	opts        RedisStreamsEmitterOptions
	rooms       *types.Set[socket.Room]
	exceptRooms *types.Set[socket.Room]
	flags       *socket.BroadcastFlags
}

func MakeRedisStreamsBroadcastOperator() *RedisStreamsBroadcastOperator {
	return &RedisStreamsBroadcastOperator{
		rooms:       types.NewSet[socket.Room](),
		exceptRooms: types.NewSet[socket.Room](),
		flags:       new(socket.BroadcastFlags),
	}
}

func NewRedisStreamsBroadcastOperator(
	client *redis.RedisClient,
	nsp string,
	opts *RedisStreamsEmitterOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) *RedisStreamsBroadcastOperator {
	b := MakeRedisStreamsBroadcastOperator()
	b.Construct(client, nsp, opts, rooms, exceptRooms, flags)
	return b
}

func (b *RedisStreamsBroadcastOperator) Construct(
	client *redis.RedisClient,
	nsp string,
	opts *RedisStreamsEmitterOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) {
	b.redisClient = client
	b.nsp = nsp
	if opts != nil {
		b.opts = *opts
	}
	if b.opts.GetRawStreamName() == nil {
		b.opts.SetStreamName(DefaultStreamName)
	}
	if b.opts.GetRawStreamCount() == nil {
		b.opts.SetStreamCount(DefaultStreamCount)
	}
	if b.opts.GetRawMaxLen() == nil {
		b.opts.SetMaxLen(DefaultStreamMaxLen)
	}
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

func (b *RedisStreamsBroadcastOperator) To(room ...socket.Room) BroadcastOperatorInterface {
	rooms := types.NewSet(b.rooms.Keys()...)
	rooms.Add(room...)
	return NewRedisStreamsBroadcastOperator(
		b.redisClient,
		b.nsp,
		&b.opts,
		rooms,
		b.exceptRooms,
		b.flags,
	)
}

func (b *RedisStreamsBroadcastOperator) In(room ...socket.Room) BroadcastOperatorInterface {
	return b.To(room...)
}

func (b *RedisStreamsBroadcastOperator) Except(room ...socket.Room) BroadcastOperatorInterface {
	exceptRooms := types.NewSet(b.exceptRooms.Keys()...)
	exceptRooms.Add(room...)
	return NewRedisStreamsBroadcastOperator(
		b.redisClient,
		b.nsp,
		&b.opts,
		b.rooms,
		exceptRooms,
		b.flags,
	)
}

func (b *RedisStreamsBroadcastOperator) Compress(compress bool) BroadcastOperatorInterface {
	flags := new(*b.flags)
	flags.Compress = new(compress)
	return NewRedisStreamsBroadcastOperator(
		b.redisClient,
		b.nsp,
		&b.opts,
		b.rooms,
		b.exceptRooms,
		flags,
	)
}

func (b *RedisStreamsBroadcastOperator) Volatile() BroadcastOperatorInterface {
	flags := new(*b.flags)
	flags.Volatile = true
	return NewRedisStreamsBroadcastOperator(
		b.redisClient,
		b.nsp,
		&b.opts,
		b.rooms,
		b.exceptRooms,
		flags,
	)
}

func (b *RedisStreamsBroadcastOperator) Emit(ev string, args ...any) error {
	if reservedEvents.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
	}

	return b.publishMessage(&adapter.ClusterMessage{
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: &parser.Packet{
				Type: parser.EVENT,
				Nsp:  b.nsp,
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

func (b *RedisStreamsBroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
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

func (b *RedisStreamsBroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
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

func (b *RedisStreamsBroadcastOperator) DisconnectSockets(close bool) error {
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

func (b *RedisStreamsBroadcastOperator) ServerSideEmit(args ...any) error {
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

func (b *RedisStreamsBroadcastOperator) publishMessage(message *adapter.ClusterMessage) error {
	message.Uid = adapter.EMITTER_UID
	message.Nsp = b.nsp

	payload, err := redis.EncodeStreamMessage(message, false)
	if err != nil {
		return err
	}
	streamName := redis.StreamNameForNamespace(
		b.opts.StreamName(),
		b.nsp,
		b.opts.StreamCount(),
	)
	redisStreamsEmitterLog.Debug("publishing message %d to stream %s", message.Type, streamName)
	_, err = redis.XAdd(b.redisClient, streamName, payload, b.opts.MaxLen())
	return err
}
