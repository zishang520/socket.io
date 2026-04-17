// Package emitter provides broadcast capabilities for Socket.IO via Unix Domain Sockets.
package emitter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/unix/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
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
// Nil parameters are replaced with safe defaults.
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
// This method is called by NewBroadcastOperator and handles nil safety.
func (b *BroadcastOperator) Construct(
	client *unix.UnixClient,
	broadcastOptions *BroadcastOptions,
	rooms *types.Set[socket.Room],
	exceptRooms *types.Set[socket.Room],
	flags *socket.BroadcastFlags,
) {
	b.unixClient = client

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

// Compress sets the compress flag for the broadcast.
// When true, the message will be compressed before transmission.
func (b *BroadcastOperator) Compress(compress bool) BroadcastOperatorInterface {
	flags := *b.flags
	flags.Compress = &compress
	return NewBroadcastOperator(b.unixClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Volatile sets the volatile flag for the broadcast.
// When set, the event data may be lost if the client is not ready to receive.
func (b *BroadcastOperator) Volatile() BroadcastOperatorInterface {
	flags := *b.flags
	flags.Volatile = true
	return NewBroadcastOperator(b.unixClient, b.broadcastOptions, b.rooms, b.exceptRooms, &flags)
}

// Emit broadcasts an event with the given name and arguments to all targeted clients.
// Returns an error if the event name is reserved or if broadcasting fails.
//
// The message is sent as a ClusterMessage in JSON format via Unix Domain Socket.
// If the message contains binary data, msgpack encoding is used.
func (b *BroadcastOperator) Emit(ev string, args ...any) error {
	if reservedEvents.Has(ev) {
		return fmt.Errorf(`"%s" is a reserved event name`, ev)
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

	// Build ClusterMessage
	message := &adapter.ClusterMessage{
		Uid:  emitterUID,
		Nsp:  b.broadcastOptions.Nsp,
		Type: adapter.BROADCAST,
		Data: &adapter.BroadcastMessage{
			Packet: packet,
			Opts:   opts,
		},
	}

	return b.publish(message)
}

// publish sends a ClusterMessage via Unix Domain Socket, handling binary detection.
func (b *BroadcastOperator) publish(message *adapter.ClusterMessage) error {
	var payload []byte
	var err error

	// Check binary data — binary uses msgpack, non-binary uses JSON
	if b.messageHasBinary(message) {
		payload, err = utils.MsgPack().Encode(message)
		if err != nil {
			return fmt.Errorf("failed to msgpack-encode message: %w", err)
		}
	} else {
		payload, err = json.Marshal(message)
		if err != nil {
			return err
		}
	}

	emitterLog.Debug("publishing message to Unix socket peers")

	// Broadcast to all peer listener sockets
	return b.broadcast(payload)
}

// messageHasBinary checks if a ClusterMessage contains binary data.
func (b *BroadcastOperator) messageHasBinary(message *adapter.ClusterMessage) bool {
	if message.Data == nil {
		return false
	}
	switch message.Type {
	case adapter.BROADCAST, adapter.SERVER_SIDE_EMIT, adapter.SERVER_SIDE_EMIT_RESPONSE:
		return parser.HasBinary(message.Data)
	default:
		return false
	}
}

// broadcast sends a message to all peer Unix Domain Socket listeners.
// It discovers peers by scanning the socket directory for matching listener paths.
func (b *BroadcastOperator) broadcast(payload []byte) error {
	socketPath := b.broadcastOptions.SocketPath
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}

	dir := filepath.Dir(socketPath)
	base := filepath.Base(socketPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("failed to read socket directory %q: %w", dir, err)
	}

	var lastErr error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		// Match peer listener sockets: "{base}.{uid}"
		if !strings.HasPrefix(name, base+".") {
			continue
		}

		peerPath := filepath.Join(dir, name)

		if err := b.unixClient.Send(peerPath, payload); err != nil {
			emitterLog.Debug("failed to send to peer %s: %s", peerPath, err.Error())
			lastErr = err
		}
	}

	return lastErr
}

// SocketsJoin makes all matching socket instances join the specified rooms.
// This sends a SOCKETS_JOIN ClusterMessage to all Socket.IO servers.
func (b *BroadcastOperator) SocketsJoin(rooms ...socket.Room) error {
	message := &adapter.ClusterMessage{
		Uid:  emitterUID,
		Nsp:  b.broadcastOptions.Nsp,
		Type: adapter.SOCKETS_JOIN,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts: &adapter.PacketOptions{
				Rooms:  b.rooms.Keys(),
				Except: b.exceptRooms.Keys(),
			},
			Rooms: rooms,
		},
	}

	return b.publish(message)
}

// SocketsLeave makes all matching socket instances leave the specified rooms.
// This sends a SOCKETS_LEAVE ClusterMessage to all Socket.IO servers.
func (b *BroadcastOperator) SocketsLeave(rooms ...socket.Room) error {
	message := &adapter.ClusterMessage{
		Uid:  emitterUID,
		Nsp:  b.broadcastOptions.Nsp,
		Type: adapter.SOCKETS_LEAVE,
		Data: &adapter.SocketsJoinLeaveMessage{
			Opts: &adapter.PacketOptions{
				Rooms:  b.rooms.Keys(),
				Except: b.exceptRooms.Keys(),
			},
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
		Uid:  emitterUID,
		Nsp:  b.broadcastOptions.Nsp,
		Type: adapter.DISCONNECT_SOCKETS,
		Data: &adapter.DisconnectSocketsMessage{
			Opts: &adapter.PacketOptions{
				Rooms:  b.rooms.Keys(),
				Except: b.exceptRooms.Keys(),
			},
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
			return fmt.Errorf("acknowledgements are not supported when using emitter")
		}
	}

	message := &adapter.ClusterMessage{
		Uid:  emitterUID,
		Nsp:  b.broadcastOptions.Nsp,
		Type: adapter.SERVER_SIDE_EMIT,
		Data: &adapter.ServerSideEmitMessage{
			Packet: args,
		},
	}

	return b.publish(message)
}
