// Package emitter provides broadcast capabilities for Socket.IO via Valkey pub/sub.
package emitter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	valkey "github.com/zishang520/socket.io/adapters/valkey/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

var reservedEvents = types.NewSet(
	"connect",
	"connect_error",
	"disconnect",
	"disconnecting",
	"newListener",
	"removeListener",
)

// BroadcastOperator provides a fluent API for broadcasting events to Socket.IO clients via Valkey.
type BroadcastOperator struct {
	valkeyClient     *valkey.ValkeyClient
	broadcastOptions *BroadcastOptions
	rooms            *types.Set[socket.Room]
	exceptRooms      *types.Set[socket.Room]
	flags            *socket.BroadcastFlags
}

// MakeBroadcastOperator creates a new BroadcastOperator with empty room sets and default flags.
func MakeBroadcastOperator() *BroadcastOperator {
	return &BroadcastOperator{
		rooms:       types.NewSet[socket.Room](),
		exceptRooms: types.NewSet[socket.Room](),
		flags:       &socket.BroadcastFlags{},
	}
}

// NewBroadcastOperator creates and initializes a new BroadcastOperator.
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

// Construct initializes the BroadcastOperator.
func (b *BroadcastOperator) Construct(
	client *valkey.ValkeyClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) {
	b.valkeyClient = client

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
	flags := *b.flags
	flags.Compress = &compress
	return NewBroadcastOperator(b.valkeyClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

func (b *BroadcastOperator) Volatile() BroadcastOperatorInterface {
	flags := *b.flags
	flags.Volatile = true
	return NewBroadcastOperator(b.valkeyClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Emit broadcasts an event with the given name and arguments to all targeted clients.
func (b *BroadcastOperator) Emit(ev string, args ...any) error {
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

	msg, err := b.broadcastOptions.Parser.Encode(&Packet{
		Uid:    emitterUID,
		Packet: packet,
		Opts:   opts,
	})
	if err != nil {
		return err
	}

	channel := b.broadcastOptions.BroadcastChannel
	if b.rooms != nil && b.rooms.Len() == 1 {
		keys := b.rooms.Keys()
		if valkey.ShouldUseDynamicChannel(b.broadcastOptions.SubscriptionMode, keys[0]) {
			channel += string(keys[0]) + "#"
		}
	}

	emitterLog.Debug("publishing message to channel %s", channel)

	return b.valkeyClient.Publish(b.valkeyClient.Context, channel, msg)
}

// SocketsJoin makes all matching socket instances join the specified rooms.
func (b *BroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
	request, err := json.Marshal(&Request{
		Type: valkey.REMOTE_JOIN,
		Opts: &adapter.PacketOptions{
			Rooms:  b.rooms.Keys(),
			Except: b.exceptRooms.Keys(),
		},
		Rooms: rooms,
	})
	if err != nil {
		return err
	}
	return b.valkeyClient.Publish(b.valkeyClient.Context, b.broadcastOptions.RequestChannel, request)
}

// SocketsLeave makes all matching socket instances leave the specified rooms.
func (b *BroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
	request, err := json.Marshal(&Request{
		Type: valkey.REMOTE_LEAVE,
		Opts: &adapter.PacketOptions{
			Rooms:  b.rooms.Keys(),
			Except: b.exceptRooms.Keys(),
		},
		Rooms: rooms,
	})
	if err != nil {
		return err
	}
	return b.valkeyClient.Publish(b.valkeyClient.Context, b.broadcastOptions.RequestChannel, request)
}

// DisconnectSockets disconnects all matching socket instances.
func (b *BroadcastOperator) DisconnectSockets(state bool) error {
	request, err := json.Marshal(&Request{
		Type: valkey.REMOTE_DISCONNECT,
		Opts: &adapter.PacketOptions{
			Rooms:  b.rooms.Keys(),
			Except: b.exceptRooms.Keys(),
		},
		Close: state,
	})
	if err != nil {
		return err
	}
	return b.valkeyClient.Publish(b.valkeyClient.Context, b.broadcastOptions.RequestChannel, request)
}
