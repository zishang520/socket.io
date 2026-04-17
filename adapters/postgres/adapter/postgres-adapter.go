// Package adapter provides a PostgreSQL-based adapter implementation for Socket.IO clustering.
// It uses PostgreSQL LISTEN/NOTIFY for pub/sub communication between nodes, with an attachment
// table for payloads that exceed the 8000-byte NOTIFY limit.
package adapter

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/vmihailenco/msgpack/v5"
	"github.com/zishang520/socket.io/adapters/adapter/v3"
	"github.com/zishang520/socket.io/adapters/postgres/v3"
	"github.com/zishang520/socket.io/parsers/socket/v3/parser"
	"github.com/zishang520/socket.io/servers/socket/v3"
	"github.com/zishang520/socket.io/v3/pkg/log"
	"github.com/zishang520/socket.io/v3/pkg/types"
	"github.com/zishang520/socket.io/v3/pkg/utils"
)

// postgresLog is the logger for the PostgreSQL adapter.
var postgresLog = log.NewLog("socket.io-postgres")

// postgresAdapter implements the PostgresAdapter interface using PostgreSQL LISTEN/NOTIFY.
// It extends ClusterAdapterWithHeartbeat with PostgreSQL-specific functionality for
// message publishing and notification handling.
type postgresAdapter struct {
	adapter.ClusterAdapterWithHeartbeat

	postgresClient *postgres.PostgresClient
	opts           *PostgresAdapterOptions
	channel        string
	cleanupFunc    types.Callable // Cleanup callback for resource management
}

// MakePostgresAdapter creates a new uninitialized postgresAdapter.
// Call Construct() to complete initialization before use.
func MakePostgresAdapter() PostgresAdapter {
	a := &postgresAdapter{
		ClusterAdapterWithHeartbeat: adapter.MakeClusterAdapterWithHeartbeat(),
		opts:                        DefaultPostgresAdapterOptions(),
		cleanupFunc:                 nil,
	}

	a.Prototype(a)

	return a
}

// NewPostgresAdapter creates and initializes a new PostgreSQL adapter.
// This is the preferred way to create a PostgreSQL adapter instance.
func NewPostgresAdapter(nsp socket.Namespace, client *postgres.PostgresClient, opts any) PostgresAdapter {
	a := MakePostgresAdapter()

	a.SetPostgres(client)
	a.SetOpts(opts)
	a.Construct(nsp)

	return a
}

// SetPostgres sets the PostgreSQL client for the adapter.
func (a *postgresAdapter) SetPostgres(client *postgres.PostgresClient) {
	a.postgresClient = client
}

// SetOpts sets the configuration options for the adapter.
// Options are merged with the parent ClusterAdapterWithHeartbeat options.
func (a *postgresAdapter) SetOpts(opts any) {
	a.ClusterAdapterWithHeartbeat.SetOpts(opts)

	if options, ok := opts.(PostgresAdapterOptionsInterface); ok {
		a.opts.Assign(options)
	}
}

// SetChannel sets the PostgreSQL notification channel for this adapter.
func (a *postgresAdapter) SetChannel(channel string) {
	a.channel = channel
}

// Construct initializes the PostgreSQL adapter for the given namespace.
// This method must be called before using the adapter.
func (a *postgresAdapter) Construct(nsp socket.Namespace) {
	a.ClusterAdapterWithHeartbeat.Construct(nsp)
}

// hasBinary checks if a cluster message contains binary data.
// Only certain message types may carry binary payloads.
// This matches the Node.js adapter's binary detection types exactly:
// BROADCAST, BROADCAST_ACK, SERVER_SIDE_EMIT, SERVER_SIDE_EMIT_RESPONSE.
func hasBinary(message *ClusterResponse) bool {
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

// DoPublish publishes a cluster message to other nodes via PostgreSQL pg_notify.
// If the message contains binary data, or the JSON payload exceeds the configured threshold,
// the full message is msgpack-encoded and stored in the attachment table. Only a reference
// header is sent via NOTIFY. This matches the Node.js adapter protocol exactly.
// Returns an empty offset since PostgreSQL NOTIFY does not support ordered offsets.
func (a *postgresAdapter) DoPublish(message *ClusterMessage) (adapter.Offset, error) {
	postgresLog.Debug("publishing message of type %d", message.Type)

	// Binary data always goes to attachment table (Node.js never sends binary via NOTIFY)
	if hasBinary(message) {
		return a.publishWithAttachment(message)
	}

	// Encode as JSON for NOTIFY
	payload, err := json.Marshal(message)
	if err != nil {
		return "", fmt.Errorf("failed to encode message: %w", err)
	}

	// If JSON payload exceeds threshold, use attachment table
	if len(payload) > a.opts.PayloadThreshold() {
		return a.publishWithAttachment(message)
	}

	err = a.postgresClient.Notify(a.postgresClient.Context, a.channel, string(payload))
	if err != nil {
		return "", err
	}

	return "", nil
}

// DoPublishResponse publishes a response message to the cluster.
// This is used for request-response patterns between nodes.
func (a *postgresAdapter) DoPublishResponse(requesterUid adapter.ServerId, response *ClusterResponse) error {
	_, err := a.DoPublish(response)
	return err
}

// publishWithAttachment msgpack-encodes the full ClusterMessage, stores it in the
// attachment table, and sends a lightweight NOTIFY header with the attachment ID.
// This matches the Node.js adapter protocol: attachments are always msgpack-encoded.
func (a *postgresAdapter) publishWithAttachment(message *ClusterMessage) (adapter.Offset, error) {
	// Msgpack-encode the entire ClusterMessage (matches Node.js: encode(message))
	payload, err := utils.MsgPack().Encode(message)
	if err != nil {
		return "", fmt.Errorf("failed to msgpack-encode message: %w", err)
	}

	id, err := a.postgresClient.InsertAttachment(
		a.postgresClient.Context,
		a.opts.TableName(),
		payload,
	)
	if err != nil {
		return "", fmt.Errorf("failed to insert attachment: %w", err)
	}

	// Send notification header with uid, type, and attachmentId (matches Node.js format)
	notification, err := json.Marshal(&NotificationMessage{
		Uid:          a.Uid(),
		Type:         message.Type,
		AttachmentId: strconv.FormatInt(id, 10),
	})
	if err != nil {
		return "", err
	}

	err = a.postgresClient.Notify(a.postgresClient.Context, a.channel, string(notification))
	return "", err
}

// OnNotification processes a raw notification payload received from PostgreSQL LISTEN/NOTIFY.
// It handles both direct JSON payloads and attachment references (msgpack-encoded in the DB).
func (a *postgresAdapter) OnNotification(payload string) {
	postgresLog.Debug("received notification on channel %s", a.channel)

	// Parse the JSON notification (either a full message or an attachment header)
	var notification NotificationMessage
	if err := json.Unmarshal([]byte(payload), &notification); err != nil {
		postgresLog.Debug("failed to parse notification: %s", err.Error())
		return
	}

	// Check if this is from ourselves
	if notification.Uid == a.Uid() {
		return
	}

	// If there's an attachment ID, fetch the full payload from the table and decode as msgpack
	if notification.AttachmentId != "" {
		attachmentId, err := strconv.ParseInt(notification.AttachmentId, 10, 64)
		if err != nil {
			postgresLog.Debug("invalid attachment ID: %s", notification.AttachmentId)
			return
		}

		attachmentPayload, err := a.postgresClient.GetAttachment(
			a.postgresClient.Context,
			a.opts.TableName(),
			attachmentId,
		)
		if err != nil {
			postgresLog.Debug("failed to fetch attachment %d: %s", attachmentId, err.Error())
			return
		}

		// Attachment payloads are msgpack-encoded (matches Node.js: decode(result.rows[0].payload))
		message, err := a.decodeMsgpack(attachmentPayload)
		if err != nil {
			postgresLog.Debug("failed to decode msgpack attachment: %s", err.Error())
			return
		}

		if message.Uid == a.Uid() {
			return
		}

		a.OnMessage(message, "")
		return
	}

	// Direct NOTIFY payload — decode as JSON
	message, err := a.decode([]byte(payload))
	if err != nil {
		postgresLog.Debug("failed to decode message: %s", err.Error())
		return
	}

	// The uid was already checked above, but verify again from the full message
	if message.Uid == a.Uid() {
		return
	}

	a.OnMessage(message, "")
}

// decode converts a JSON NOTIFY payload into a typed ClusterResponse.
// This handles non-binary messages sent directly via pg_notify.
func (a *postgresAdapter) decode(payload []byte) (*ClusterResponse, error) {
	// Parse the outer structure with Data as raw JSON
	var raw struct {
		Uid  string              `json:"uid"`
		Nsp  string              `json:"nsp"`
		Type adapter.MessageType `json:"type"`
		Data json.RawMessage     `json:"data,omitempty"`
	}

	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notification: %w", err)
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

// decodeMsgpack converts a msgpack-encoded attachment payload into a typed ClusterResponse.
// This handles binary/large messages stored in the attachment table.
func (a *postgresAdapter) decodeMsgpack(payload []byte) (*ClusterResponse, error) {
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
func (a *postgresAdapter) decodeData(messageType adapter.MessageType, rawData json.RawMessage) (any, error) {
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
func (a *postgresAdapter) decodeMsgpackData(messageType adapter.MessageType, rawData msgpack.RawMessage) (any, error) {
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
		return &BroadcastMessage{}
	case adapter.SOCKETS_JOIN, adapter.SOCKETS_LEAVE:
		return &SocketsJoinLeaveMessage{}
	case adapter.DISCONNECT_SOCKETS:
		return &DisconnectSocketsMessage{}
	case adapter.FETCH_SOCKETS:
		return &FetchSocketsMessage{}
	case adapter.FETCH_SOCKETS_RESPONSE:
		return &FetchSocketsResponse{}
	case adapter.SERVER_SIDE_EMIT:
		return &ServerSideEmitMessage{}
	case adapter.SERVER_SIDE_EMIT_RESPONSE:
		return &ServerSideEmitResponse{}
	case adapter.BROADCAST_CLIENT_COUNT:
		return &BroadcastClientCount{}
	case adapter.BROADCAST_ACK:
		return &BroadcastAck{}
	default:
		return nil
	}
}

// Cleanup registers a cleanup callback to be called when the adapter is closed.
func (a *postgresAdapter) Cleanup(cleanup func()) {
	a.cleanupFunc = cleanup
}

// Close releases resources and invokes the registered cleanup callback.
func (a *postgresAdapter) Close() {
	defer a.ClusterAdapterWithHeartbeat.Close()

	if a.cleanupFunc != nil {
		a.cleanupFunc()
	}
}
