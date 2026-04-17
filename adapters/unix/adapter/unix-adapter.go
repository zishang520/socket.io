// Package adapter provides a Unix Domain Socket-based adapter implementation for Socket.IO clustering.
// It uses Unix Domain Sockets for pub/sub communication between nodes on the same machine.
package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/unix/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// unixLog is the logger for the Unix Domain Socket adapter.
var unixLog = log.NewLog("socket.io-unix")

// unixAdapter implements the UnixAdapter interface using Unix Domain Sockets.
// It extends ClusterAdapterWithHeartbeat with Unix socket-specific functionality for
// message publishing and receiving.
type unixAdapter struct {
	adapter.ClusterAdapterWithHeartbeat

	unixClient  *unix.UnixClient
	opts        *UnixAdapterOptions
	channel     string
	cleanupFunc types.Callable // Cleanup callback for resource management
}

// MakeUnixAdapter creates a new uninitialized unixAdapter.
// Call Construct() to complete initialization before use.
func MakeUnixAdapter() UnixAdapter {
	a := &unixAdapter{
		ClusterAdapterWithHeartbeat: adapter.MakeClusterAdapterWithHeartbeat(),
		opts:                        DefaultUnixAdapterOptions(),
		cleanupFunc:                 nil,
	}

	a.Prototype(a)

	return a
}

// NewUnixAdapter creates and initializes a new Unix Domain Socket adapter.
// This is the preferred way to create a Unix adapter instance.
func NewUnixAdapter(nsp socket.Namespace, client *unix.UnixClient, opts any) UnixAdapter {
	a := MakeUnixAdapter()

	a.SetUnix(client)
	a.SetOpts(opts)
	a.Construct(nsp)

	return a
}

// SetUnix sets the Unix Domain Socket client for the adapter.
func (a *unixAdapter) SetUnix(client *unix.UnixClient) {
	a.unixClient = client
}

// SetOpts sets the configuration options for the adapter.
// Options are merged with the parent ClusterAdapterWithHeartbeat options.
func (a *unixAdapter) SetOpts(opts any) {
	a.ClusterAdapterWithHeartbeat.SetOpts(opts)

	if options, ok := opts.(UnixAdapterOptionsInterface); ok {
		a.opts.Assign(options)
	}
}

// SetChannel sets the channel prefix for this adapter.
func (a *unixAdapter) SetChannel(channel string) {
	a.channel = channel
}

// Construct initializes the Unix adapter for the given namespace.
// This method must be called before using the adapter.
func (a *unixAdapter) Construct(nsp socket.Namespace) {
	a.ClusterAdapterWithHeartbeat.Construct(nsp)
}

// hasBinary checks if a cluster message contains binary data.
// Only certain message types may carry binary payloads.
// This matches the Node.js adapter's binary detection types exactly:
// BROADCAST, BROADCAST_ACK, SERVER_SIDE_EMIT, SERVER_SIDE_EMIT_RESPONSE.
func hasBinary(message *adapter.ClusterResponse) bool {
	if message.Data == nil {
		return false
	}

	switch message.Type {
	case adapter.BROADCAST, adapter.BROADCAST_ACK,
		adapter.SERVER_SIDE_EMIT, adapter.SERVER_SIDE_EMIT_RESPONSE:
		return parser.HasBinary(message.Data)
	default:
		return false
	}
}

// DoPublish publishes a cluster message to other nodes via Unix Domain Socket.
// If the message contains binary data, msgpack encoding is used; otherwise JSON is used.
// The message is broadcast to all peer listeners found by scanning the socket directory.
// Returns an empty offset since Unix Domain Sockets do not support ordered offsets.
func (a *unixAdapter) DoPublish(message *adapter.ClusterMessage) (adapter.Offset, error) {
	unixLog.Debug("publishing message of type %d", message.Type)

	var payload []byte
	var err error

	// Binary data uses msgpack encoding; non-binary uses JSON
	if hasBinary(message) {
		payload, err = utils.MsgPack().Encode(message)
		if err != nil {
			return "", fmt.Errorf("failed to msgpack-encode message: %w", err)
		}
	} else {
		payload, err = json.Marshal(message)
		if err != nil {
			return "", fmt.Errorf("failed to encode message: %w", err)
		}
	}

	// Broadcast to all peer listener sockets
	if err := a.broadcast(payload); err != nil {
		return "", err
	}

	return "", nil
}

// DoPublishResponse publishes a response message to the cluster.
// This is used for request-response patterns between nodes.
func (a *unixAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *adapter.ClusterResponse) error {
	_, err := a.DoPublish(response)
	return err
}

// broadcast sends a message to all peer Unix Domain Socket listeners.
// It discovers peers by scanning the socket directory for matching listener paths.
func (a *unixAdapter) broadcast(payload []byte) error {
	socketPath := a.unixClient.SocketPath
	dir := filepath.Dir(socketPath)
	base := filepath.Base(socketPath)
	selfPath := a.unixClient.ListenerPath()

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

		// Skip self
		if peerPath == selfPath {
			continue
		}

		if err := a.unixClient.Send(peerPath, payload); err != nil {
			unixLog.Debug("failed to send to peer %s: %s", peerPath, err.Error())
			lastErr = err
		}
	}

	return lastErr
}

// OnRawMessage processes a raw message payload received from Unix Domain Socket.
// It handles both JSON and msgpack-encoded messages.
func (a *unixAdapter) OnRawMessage(payload []byte) {
	unixLog.Debug("received message on channel %s", a.channel)

	if len(payload) == 0 {
		return
	}

	var message *adapter.ClusterResponse
	var err error

	// Auto-detect encoding: JSON starts with '{', msgpack does not
	if payload[0] == '{' {
		message, err = a.decode(payload)
	} else {
		message, err = a.decodeMsgpack(payload)
	}

	if err != nil {
		unixLog.Debug("failed to decode message: %s", err.Error())
		return
	}

	// Check if this is from ourselves
	if message.Uid == a.Uid() {
		return
	}

	// Check namespace match
	if message.Nsp != "" && message.Nsp != a.Nsp().Name() {
		return
	}

	a.OnMessage(message, "")
}

// decode converts a JSON payload into a typed ClusterResponse.
// This handles non-binary messages.
func (a *unixAdapter) decode(payload []byte) (*adapter.ClusterResponse, error) {
	// Parse the outer structure with Data as raw JSON
	var raw struct {
		Uid  string              `json:"uid"`
		Nsp  string              `json:"nsp"`
		Type adapter.MessageType `json:"type"`
		Data json.RawMessage     `json:"data,omitempty"`
	}

	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}

	message := &adapter.ClusterMessage{
		Uid:  adapter.ServerId(raw.Uid),
		Nsp:  raw.Nsp,
		Type: raw.Type,
	}

	// Return early if no data
	if len(raw.Data) == 0 || string(raw.Data) == "null" {
		return message, nil
	}

	// Decode message data based on the message type
	data, err := a.decodeData(message.Type, raw.Data)
	if err != nil {
		return nil, err
	}
	message.Data = data

	return message, nil
}

// decodeMsgpack converts a msgpack-encoded payload into a typed ClusterResponse.
// This handles binary messages.
func (a *unixAdapter) decodeMsgpack(payload []byte) (*adapter.ClusterResponse, error) {
	// Two-pass decode: first get outer fields with Data as raw msgpack
	var raw struct {
		Uid  string              `msgpack:"uid,omitempty"`
		Nsp  string              `msgpack:"nsp,omitempty"`
		Type adapter.MessageType `msgpack:"type,omitempty"`
		Data msgpack.RawMessage  `msgpack:"data,omitempty"`
	}

	if err := utils.MsgPack().Decode(payload, &raw); err != nil {
		return nil, fmt.Errorf("failed to decode msgpack message: %w", err)
	}

	message := &adapter.ClusterMessage{
		Uid:  adapter.ServerId(raw.Uid),
		Nsp:  raw.Nsp,
		Type: raw.Type,
	}

	if len(raw.Data) == 0 {
		return message, nil
	}

	// Decode data based on message type using msgpack
	data, err := a.decodeMsgpackData(message.Type, raw.Data)
	if err != nil {
		return nil, err
	}
	message.Data = data

	return message, nil
}

// decodeData deserializes a JSON data payload based on the message type.
func (a *unixAdapter) decodeData(messageType adapter.MessageType, rawData json.RawMessage) (any, error) {
	target := allocateTarget(messageType)
	if target == nil {
		return nil, nil
	}

	if err := json.Unmarshal(rawData, target); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON data: %w", err)
	}

	return target, nil
}

// decodeMsgpackData deserializes a msgpack data payload based on the message type.
func (a *unixAdapter) decodeMsgpackData(messageType adapter.MessageType, rawData msgpack.RawMessage) (any, error) {
	target := allocateTarget(messageType)
	if target == nil {
		return nil, nil
	}

	if err := utils.MsgPack().Decode(rawData, target); err != nil {
		return nil, fmt.Errorf("failed to decode MessagePack data: %w", err)
	}

	return target, nil
}

// allocateTarget returns a pointer to the appropriate struct for the given message type.
func allocateTarget(messageType adapter.MessageType) any {
	switch messageType {
	case adapter.INITIAL_HEARTBEAT, adapter.HEARTBEAT, adapter.ADAPTER_CLOSE:
		return nil
	case adapter.BROADCAST:
		return &adapter.BroadcastMessage{}
	case adapter.SOCKETS_JOIN, adapter.SOCKETS_LEAVE:
		return &adapter.SocketsJoinLeaveMessage{}
	case adapter.DISCONNECT_SOCKETS:
		return &adapter.DisconnectSocketsMessage{}
	case adapter.FETCH_SOCKETS:
		return &adapter.FetchSocketsMessage{}
	case adapter.FETCH_SOCKETS_RESPONSE:
		return &adapter.FetchSocketsResponse{}
	case adapter.SERVER_SIDE_EMIT:
		return &adapter.ServerSideEmitMessage{}
	case adapter.SERVER_SIDE_EMIT_RESPONSE:
		return &adapter.ServerSideEmitResponse{}
	case adapter.BROADCAST_CLIENT_COUNT:
		return &adapter.BroadcastClientCount{}
	case adapter.BROADCAST_ACK:
		return &adapter.BroadcastAck{}
	default:
		return nil
	}
}

// Cleanup registers a cleanup callback to be called when the adapter is closed.
func (a *unixAdapter) Cleanup(cleanup func()) {
	a.cleanupFunc = cleanup
}

// Close releases resources and invokes the registered cleanup callback.
func (a *unixAdapter) Close() {
	defer a.ClusterAdapterWithHeartbeat.Close()

	if a.cleanupFunc != nil {
		a.cleanupFunc()
	}

	// Remove the listener socket file
	if listenerPath := a.unixClient.ListenerPath(); listenerPath != "" {
		_ = os.Remove(listenerPath)
	}
}
