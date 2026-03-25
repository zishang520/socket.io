// Package emitter provides broadcast capabilities for Socket.IO via cache pub/sub.
package emitter

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	cache "github.com/zishang520/socket.io/adapters/cache/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
)

// reservedEvents cannot be emitted directly by user code.
var reservedEvents = types.NewSet(
	"connect",
	"connect_error",
	"disconnect",
	"disconnecting",
	"newListener",
	"removeListener",
)

// BroadcastOperator provides a fluent API for targeting and emitting events via classic pub/sub.
type BroadcastOperator struct {
	cacheClient      cache.CacheClient
	broadcastOptions *BroadcastOptions
	rooms            *types.Set[socket.Room]
	exceptRooms      *types.Set[socket.Room]
	flags            *socket.BroadcastFlags
}

// MakeBroadcastOperator creates a BroadcastOperator with empty room sets.
func MakeBroadcastOperator() *BroadcastOperator {
	return &BroadcastOperator{
		rooms:       types.NewSet[socket.Room](),
		exceptRooms: types.NewSet[socket.Room](),
		flags:       &socket.BroadcastFlags{},
	}
}

// NewBroadcastOperator creates and initializes a BroadcastOperator.
func NewBroadcastOperator(
	client cache.CacheClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) *BroadcastOperator {
	b := MakeBroadcastOperator()
	b.Construct(client, broadcastOptions, rooms, exceptRooms, flags)
	return b
}

// Construct initializes the BroadcastOperator; nil parameters use safe defaults.
func (b *BroadcastOperator) Construct(
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

func (b *BroadcastOperator) To(room ...socket.Room) BroadcastOperatorInterface {
	rooms := types.NewSet(b.rooms.Keys()...)
	rooms.Add(room...)
	return NewBroadcastOperator(b.cacheClient, b.broadcastOptions, rooms, b.exceptRooms, b.flags)
}

func (b *BroadcastOperator) In(room ...socket.Room) BroadcastOperatorInterface { return b.To(room...) }

func (b *BroadcastOperator) Except(room ...socket.Room) BroadcastOperatorInterface {
	exceptRooms := types.NewSet(b.exceptRooms.Keys()...)
	exceptRooms.Add(room...)
	return NewBroadcastOperator(b.cacheClient, b.broadcastOptions, b.rooms, exceptRooms, b.flags)
}

func (b *BroadcastOperator) Compress(compress bool) BroadcastOperatorInterface {
	flags := *b.flags
	flags.Compress = &compress
	return NewBroadcastOperator(b.cacheClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

func (b *BroadcastOperator) Volatile() BroadcastOperatorInterface {
	flags := *b.flags
	flags.Volatile = true
	return NewBroadcastOperator(b.cacheClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Emit broadcasts an event to all targeted clients.
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
		if cache.ShouldUseDynamicChannel(b.broadcastOptions.SubscriptionMode, keys[0]) {
			channel += string(keys[0]) + "#"
		}
	}

	emitterLog.Debug("publishing message to channel %s", channel)
	return b.cacheClient.Publish(b.cacheClient.Context(), channel, msg)
}

// SocketsJoin sends a REMOTE_JOIN request to all servers.
func (b *BroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
	request, err := json.Marshal(&Request{
		Type: cache.REMOTE_JOIN,
		Opts: &adapter.PacketOptions{
			Rooms:  b.rooms.Keys(),
			Except: b.exceptRooms.Keys(),
		},
		Rooms: rooms,
	})
	if err != nil {
		return err
	}
	return b.cacheClient.Publish(b.cacheClient.Context(), b.broadcastOptions.RequestChannel, request)
}

// SocketsLeave sends a REMOTE_LEAVE request to all servers.
func (b *BroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
	request, err := json.Marshal(&Request{
		Type: cache.REMOTE_LEAVE,
		Opts: &adapter.PacketOptions{
			Rooms:  b.rooms.Keys(),
			Except: b.exceptRooms.Keys(),
		},
		Rooms: rooms,
	})
	if err != nil {
		return err
	}
	return b.cacheClient.Publish(b.cacheClient.Context(), b.broadcastOptions.RequestChannel, request)
}

// DisconnectSockets sends a REMOTE_DISCONNECT request to all servers.
func (b *BroadcastOperator) DisconnectSockets(state bool) error {
	request, err := json.Marshal(&Request{
		Type: cache.REMOTE_DISCONNECT,
		Opts: &adapter.PacketOptions{
			Rooms:  b.rooms.Keys(),
			Except: b.exceptRooms.Keys(),
		},
		Close: state,
	})
	if err != nil {
		return err
	}
	return b.cacheClient.Publish(b.cacheClient.Context(), b.broadcastOptions.RequestChannel, request)
}
