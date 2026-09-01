// Package emitter provides broadcast capabilities for Socket.IO via Unix Domain Sockets.
package emitter

import (
	"errors"
	"fmt"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/unix/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

var errAcknowledgementsNotSupported = errors.New("Acknowledgements are not supported") //nolint:staticcheck // Node.js API text

// BroadcastOperator provides a fluent API for broadcasting events to Socket.IO clients via Unix Domain Sockets.
// It supports room targeting, exclusions, and broadcast flags through method chaining.
type BroadcastOperator struct {
	unixClient       *unix.UnixClient        // Unix Domain Socket client for publishing messages
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
// Nil optional configuration values are replaced with safe defaults.
func NewBroadcastOperator(
	client *unix.UnixClient,
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
// This method is called by NewBroadcastOperator and normalizes optional values.
func (b *BroadcastOperator) Construct(
	client *unix.UnixClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) {
	b.unixClient = client

	if broadcastOptions == nil {
		broadcastOptions = &BroadcastOptions{Nsp: defaultNamespace}
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
	return NewBroadcastOperator(b.unixClient, b.broadcastOptions, rooms, b.exceptRooms, b.flags)
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
	return NewBroadcastOperator(b.unixClient, b.broadcastOptions, b.rooms, exceptRooms, b.flags)
}

// Compress sets the client-facing transport compression preference for the
// broadcast. It does not compress the Unix cluster frame.
func (b *BroadcastOperator) Compress(compress bool) BroadcastOperatorInterface {
	flags := new(*b.flags)
	flags.Compress = new(compress)
	return NewBroadcastOperator(b.unixClient, b.broadcastOptions, b.rooms, b.exceptRooms, flags)
}

// Volatile sets the volatile flag for the broadcast.
// When set, the event data may be lost if the client is not ready to receive.
func (b *BroadcastOperator) Volatile() BroadcastOperatorInterface {
	flags := new(*b.flags)
	flags.Volatile = true
	return NewBroadcastOperator(b.unixClient, b.broadcastOptions, b.rooms, b.exceptRooms, flags)
}

// Emit broadcasts an event with the given name and arguments to all targeted clients.
// Returns an error if the event name is reserved or if broadcasting fails.
//
// The shared cluster codec selects JSON or MessagePack based on the payload.
func (b *BroadcastOperator) Emit(ev string, args ...any) error {
	if socket.SOCKET_RESERVED_EVENTS.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
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

	// Build ClusterMessage
	message := &adapter.ClusterMessage{
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: packet,
			Opts:   opts,
		},
	}

	return b.publish(message)
}

// publish encodes and sends a ClusterMessage over the shared Unix transport.
func (b *BroadcastOperator) publish(message *adapter.ClusterMessage) error {
	message.Uid = adapter.EMITTER_UID
	message.Nsp = b.broadcastOptions.Nsp
	payload, err := adapter.EncodeClusterMessage(message)

	if err != nil {
		return err
	}

	return b.unixClient.Broadcast(payload)
}

// SocketsJoin makes all matching socket instances join the specified rooms.
// This sends a SOCKETS_JOIN ClusterMessage to all Socket.IO servers.
func (b *BroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
	message := &adapter.ClusterMessage{
		Type: adapter.SOCKETS_JOIN,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
				Rooms:  b.rooms,
				Except: b.exceptRooms,
			}),
			Rooms: rooms,
		},
	}

	return b.publish(message)
}

// SocketsLeave makes all matching socket instances leave the specified rooms.
// This sends a SOCKETS_LEAVE ClusterMessage to all Socket.IO servers.
func (b *BroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
	message := &adapter.ClusterMessage{
		Type: adapter.SOCKETS_LEAVE,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
				Rooms:  b.rooms,
				Except: b.exceptRooms,
			}),
			Rooms: rooms,
		},
	}

	return b.publish(message)
}

// DisconnectSockets disconnects all matching socket instances.
// If state is true, the underlying transport connection will be closed.
// This sends a DISCONNECT_SOCKETS ClusterMessage to all Socket.IO servers.
func (b *BroadcastOperator) DisconnectSockets(state bool) error {
	message := &adapter.ClusterMessage{
		Type: adapter.DISCONNECT_SOCKETS,
		Data: &adapter.DisconnectSocketsMessage{
			Opts: adapter.EncodeOptions(&socket.BroadcastOptions{
				Rooms:  b.rooms,
				Except: b.exceptRooms,
			}),
			Close: state,
		},
	}

	return b.publish(message)
}

// ServerSideEmit sends a message to all Socket.IO servers.
// The first argument should be the event name, followed by any data arguments.
// Note: Acknowledgements are not supported when using the emitter.
func (b *BroadcastOperator) ServerSideEmit(args ...any) error {
	if len(args) > 0 {
		if _, withAck := args[len(args)-1].(socket.Ack); withAck {
			return errAcknowledgementsNotSupported
		}
	}

	message := &adapter.ClusterMessage{
		Type: adapter.SERVER_SIDE_EMIT,
		Data: &adapter.ServerSideEmitMessage{
			Packet: args,
		},
	}

	return b.publish(message)
}
