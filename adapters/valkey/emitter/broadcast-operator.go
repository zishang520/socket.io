// Package emitter provides broadcast capabilities for Socket.IO via Valkey.
package emitter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var (
	errAcknowledgementsNotSupported = errors.New("Acknowledgements are not supported") //nolint:staticcheck // Node.js API text
	errEncoderNotSet                = errors.New("broadcastOptions.Encoder is not set")
)

// BroadcastOperator publishes packets with the classic Valkey emitter protocol.
type BroadcastOperator struct {
	valkeyClient     *valkey.ValkeyClient
	broadcastOptions *BroadcastOptions
	rooms            *types.Set[socket.Room]
	exceptRooms      *types.Set[socket.Room]
	flags            *socket.BroadcastFlags
}

func MakeBroadcastOperator() *BroadcastOperator {
	return &BroadcastOperator{
		rooms:       types.NewSet[socket.Room](),
		exceptRooms: types.NewSet[socket.Room](),
		flags:       new(socket.BroadcastFlags),
	}
}

func NewBroadcastOperator(
	client *valkey.ValkeyClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) *BroadcastOperator {
	b := MakeBroadcastOperator()
	b.Construct(client, broadcastOptions, rooms, exceptRooms, flags)
	return b
}

func (b *BroadcastOperator) Construct(
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

func (b *BroadcastOperator) To(room ...socket.Room) BroadcastOperatorInterface {
	rooms := types.NewSet(b.rooms.Keys()...)
	rooms.Add(room...)
	return NewBroadcastOperator(b.valkeyClient, b.broadcastOptions, rooms, b.exceptRooms, b.flags)
}

func (b *BroadcastOperator) In(room ...socket.Room) BroadcastOperatorInterface {
	return b.To(room...)
}

func (b *BroadcastOperator) Except(room ...socket.Room) BroadcastOperatorInterface {
	exceptRooms := types.NewSet(b.exceptRooms.Keys()...)
	exceptRooms.Add(room...)
	return NewBroadcastOperator(b.valkeyClient, b.broadcastOptions, b.rooms, exceptRooms, b.flags)
}

func (b *BroadcastOperator) Compress(compress bool) BroadcastOperatorInterface {
	flags := new(*b.flags)
	flags.Compress = new(compress)
	return NewBroadcastOperator(b.valkeyClient, b.broadcastOptions, b.rooms, b.exceptRooms, flags)
}

func (b *BroadcastOperator) Volatile() BroadcastOperatorInterface {
	flags := new(*b.flags)
	flags.Volatile = true
	return NewBroadcastOperator(b.valkeyClient, b.broadcastOptions, b.rooms, b.exceptRooms, flags)
}

func (b *BroadcastOperator) Emit(ev string, args ...any) error {
	if socket.SOCKET_RESERVED_EVENTS.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
	}
	if utils.IsNil(b.broadcastOptions.Encoder) {
		return errEncoderNotSet
	}

	opts := adapter.EncodeOptions(&socket.BroadcastOptions{
		Rooms:  b.rooms,
		Except: b.exceptRooms,
		Flags:  b.flags,
	})
	payload, err := b.broadcastOptions.Encoder.Encode(&valkey.ValkeyPacket{
		Uid: adapter.EMITTER_UID,
		Packet: &parser.Packet{
			Type: parser.EVENT,
			Nsp:  b.broadcastOptions.Nsp,
			Data: utils.EventPayload(ev, args),
		},
		Opts: opts,
	})
	if err != nil {
		return err
	}

	channel := b.broadcastOptions.BroadcastChannel
	if len(opts.Rooms) == 1 {
		channel += string(opts.Rooms[0]) + "#"
	}
	emitterLog.Debug("publishing message to channel %s", channel)
	return b.valkeyClient.Publish(b.valkeyClient.Context(), channel, payload)
}

func (b *BroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
	return b.publishRequest(&valkey.ValkeyRequest{
		Type: valkey.REMOTE_JOIN,
		Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
			Rooms:  b.rooms,
			Except: b.exceptRooms,
		}),
		Rooms: rooms,
	})
}

func (b *BroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
	return b.publishRequest(&valkey.ValkeyRequest{
		Type: valkey.REMOTE_LEAVE,
		Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
			Rooms:  b.rooms,
			Except: b.exceptRooms,
		}),
		Rooms: rooms,
	})
}

func (b *BroadcastOperator) DisconnectSockets(close bool) error {
	return b.publishRequest(&valkey.ValkeyRequest{
		Type: valkey.REMOTE_DISCONNECT,
		Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
			Rooms:  b.rooms,
			Except: b.exceptRooms,
		}),
		Close: new(close),
	})
}

func (b *BroadcastOperator) ServerSideEmit(args ...any) error {
	if len(args) > 0 {
		if _, withAck := args[len(args)-1].(socket.Ack); withAck {
			return errAcknowledgementsNotSupported
		}
	}
	return b.publishRequest(&valkey.ValkeyRequest{
		Uid:  adapter.EMITTER_UID,
		Type: valkey.SERVER_SIDE_EMIT,
		Data: args,
	})
}

func (b *BroadcastOperator) publishRequest(request *valkey.ValkeyRequest) error {
	payload, err := json.Marshal(request)
	if err != nil {
		return err
	}
	return b.valkeyClient.Publish(b.valkeyClient.Context(), b.broadcastOptions.RequestChannel, payload)
}
